package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/JordanCoin/docmap/internal/docset"
	"github.com/JordanCoin/docmap/parser"
	"github.com/JordanCoin/docmap/render"
	"github.com/JordanCoin/docmap/stale"
)

const (
	hookBudget        = 1500 * time.Millisecond
	preReadMinLines   = 200
	postEditMaxLines  = 8
	preReadMaxLines   = 60
	sessionStartLimit = 5
)

// runHook runs `docmap hook <name> [--agent=codex]`, reading the agent's hook
// JSON from stdin and printing context for the model. Exit code is always 0 on
// a handled hook so a docmap problem never blocks the agent.
func runHook(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: docmap hook session-start|post-edit|pre-read [--agent=codex]")
		return 1
	}
	name := args[0]
	codex := os.Getenv("CODEX") == "1"
	for _, a := range args[1:] {
		if a == "--agent=codex" {
			codex = true
		}
	}
	input, _ := io.ReadAll(os.Stdin)
	root := hookRoot()

	var fn func() string
	var event string
	switch name {
	case "session-start":
		event, fn = "SessionStart", func() string { return hookSessionStart(root) }
	case "post-edit":
		event, fn = "PostToolUse", func() string { return hookPostEdit(root, input) }
	case "pre-read":
		event, fn = "PreToolUse", func() string { return hookPreRead(root, input) }
	default:
		fmt.Fprintf(os.Stderr, "unknown hook: %s\nAvailable: session-start, post-edit, pre-read\n", name)
		return 1
	}
	emitHook(os.Stdout, event, fn(), codex)
	return 0
}

// emitHook writes the hook result in the shape each agent reads.
// Claude Code shows plain stdout to the model for SessionStart and
// PostToolUse, but not for PreToolUse, which needs the JSON envelope.
// Codex reads one JSON envelope per hook and rejects empty stdout on
// PostToolUse, so post-edit always emits an envelope.
func emitHook(w io.Writer, event, text string, codex bool) {
	text = strings.TrimRight(text, "\n")
	if codex || event == "PreToolUse" {
		if text == "" {
			if codex && event == "PostToolUse" {
				_ = json.NewEncoder(w).Encode(map[string]any{"continue": true})
			}
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     event,
				"additionalContext": text,
			},
		})
		return
	}
	if text != "" {
		fmt.Fprintln(w, text)
	}
}

func hookRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return cwd
	}
	return strings.TrimSpace(string(out))
}

// withBudget runs fn and returns its result, or "" when it overruns d.
// ponytail: the goroutine keeps running after a timeout; the process exits
// right after, so nothing leaks.
func withBudget(d time.Duration, fn func() string) (string, bool) {
	ch := make(chan string, 1)
	go func() { ch <- fn() }()
	select {
	case s := <-ch:
		return s, true
	case <-time.After(d):
		return "", false
	}
}

func hookSessionStart(root string) string {
	docs := docset.LoadDocs(root, false)
	if len(docs) == 0 {
		return ""
	}
	md, pdf, sections, tokens := 0, 0, 0, 0
	for _, d := range docs {
		switch strings.ToLower(filepath.Ext(d.Filename)) {
		case ".md":
			md++
		case ".pdf":
			pdf++
		}
		sections += len(d.GetAllSections())
		tokens += d.TotalTokens
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📚 Docs: %d md, %d pdf · %d sections · ~%s tokens\n", md, pdf, sections, briefTokens(tokens))
	if recent := recentDocs(root, docs, sessionStartLimit); len(recent) > 0 {
		fmt.Fprintf(&b, "recent docs: %s\n", strings.Join(recent, ", "))
	}
	staleLine, ok := withBudget(hookBudget, func() string {
		n := len(stale.Check(docs, stale.Options{Root: root}))
		if n == 0 {
			return "stale: none"
		}
		return fmt.Sprintf("stale: %d (docmap . --stale)", n)
	})
	if !ok {
		staleLine = "stale: not checked in budget, run docmap . --stale"
	}
	b.WriteString(staleLine)
	return b.String()
}

// recentDocs filters git's recent files down to loaded docs, falling back to
// mtime order when git has nothing.
func recentDocs(root string, docs []*parser.Document, limit int) []string {
	isDoc := make(map[string]bool, len(docs))
	for _, d := range docs {
		isDoc[filepath.ToSlash(d.Filename)] = true
	}
	var out []string
	for _, f := range parser.RecentFiles(root, 400) {
		if isDoc[f] {
			out = append(out, f)
			if len(out) == limit {
				return out
			}
		}
	}
	if len(out) == 0 {
		return docset.RecentByMtime(docs, root, limit)
	}
	return out
}

func hookPostEdit(root string, input []byte) string {
	paths := hookFilePaths(input)
	if len(paths) == 0 {
		return ""
	}
	out, _ := withBudget(hookBudget, func() string {
		docs := docset.LoadDocs(root, false)
		var b strings.Builder
		for _, p := range paths {
			rel := relToRoot(root, p)
			lines := collectMentions(docs, []string{rel})
			if len(lines) == 0 {
				continue
			}
			fmt.Fprintf(&b, "📚 Docs mentioning %s (%d sections):\n", rel, len(lines))
			shown := lines
			if len(shown) > postEditMaxLines {
				shown = shown[:postEditMaxLines]
			}
			for _, l := range shown {
				fmt.Fprintf(&b, "  %s\n", l)
			}
			if more := len(lines) - len(shown); more > 0 {
				fmt.Fprintf(&b, "  +%d more: docmap . --mentions %s\n", more, rel)
			}
		}
		return b.String()
	})
	return out
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func hookPreRead(root string, input []byte) string {
	paths := hookFilePaths(input)
	if len(paths) == 0 {
		return ""
	}
	p := paths[0]
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext != ".md" && ext != ".pdf" {
		return ""
	}
	if ext == ".md" {
		data, err := os.ReadFile(p)
		if err != nil || strings.Count(string(data), "\n") <= preReadMinLines {
			return ""
		}
	}
	if sessionSeen(hookSessionID(input), p) {
		return ""
	}
	doc, err := parser.ParseFile(p)
	if err != nil {
		return ""
	}
	doc.Filename = filepath.Base(p)
	tree := ansiRe.ReplaceAllString(captureStdout(func() { render.Tree(doc) }), "")
	lines := strings.Split(strings.TrimRight(tree, "\n"), "\n")
	if len(lines) > preReadMaxLines {
		lines = append(lines[:preReadMaxLines], fmt.Sprintf("... %d more lines: docmap %s", len(lines)-preReadMaxLines, relToRoot(root, p)))
	}
	lines = append(lines, fmt.Sprintf("Read only the section you need: docmap %s --expand \"<title>\"", relToRoot(root, p)))
	return strings.Join(lines, "\n")
}

// sessionSeen reports whether this session already got the tree for path,
// and records it. No session id means every read is first.
func sessionSeen(session, path string) bool {
	if session == "" {
		return false
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return false
	}
	sum := sha1.Sum([]byte(session + "\x00" + path))
	dir := filepath.Join(cache, "docmap", "sessions")
	marker := filepath.Join(dir, hex.EncodeToString(sum[:]))
	if _, err := os.Stat(marker); err == nil {
		return true
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(marker, nil, 0o644)
	return false
}

func captureStdout(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		return ""
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(r); done <- b }()
	fn()
	_ = w.Close()
	os.Stdout = orig
	out := <-done
	_ = r.Close()
	return string(out)
}

func relToRoot(root, p string) string {
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(p)
}

var applyPatchRe = regexp.MustCompile(`(?m)^\*\*\* (?:Update|Add|Delete) File: (.+)$`)

// hookFilePaths extracts edited or read paths from a Claude Code or Codex
// hook payload: tool_input.file_path / path / notebook_path, a top-level
// file_path, or the files named in a Codex apply_patch command.
func hookFilePaths(input []byte) []string {
	var data map[string]any
	if err := json.Unmarshal(input, &data); err != nil {
		return nil
	}
	if p, ok := data["file_path"].(string); ok && p != "" {
		return []string{p}
	}
	ti, _ := data["tool_input"].(map[string]any)
	for _, key := range []string{"file_path", "path", "notebook_path"} {
		if p, ok := ti[key].(string); ok && strings.TrimSpace(p) != "" {
			return []string{strings.TrimSpace(p)}
		}
	}
	if cmd, ok := ti["command"].(string); ok {
		var out []string
		seen := map[string]bool{}
		for _, m := range applyPatchRe.FindAllStringSubmatch(cmd, -1) {
			p := strings.TrimSpace(m[1])
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
		return out
	}
	return nil
}

func hookSessionID(input []byte) string {
	var data struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(input, &data)
	return data.SessionID
}
