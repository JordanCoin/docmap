package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type hookSpec struct {
	Event, Matcher, Command string
}

func docmapHookSpecs(codex bool) []hookSpec {
	suffix := ""
	edit := "Edit|Write"
	if codex {
		suffix = " --agent=codex"
		edit = "apply_patch|Edit|Write"
	}
	return []hookSpec{
		{"SessionStart", "", "docmap hook session-start" + suffix},
		{"PreToolUse", "Read", "docmap hook pre-read" + suffix},
		{"PostToolUse", edit, "docmap hook post-edit" + suffix},
	}
}

func detectAgents() (claude, codex bool) {
	home, _ := os.UserHomeDir()
	for _, p := range []string{".claude", ".claude.json"} {
		if _, err := os.Stat(filepath.Join(home, p)); err == nil {
			claude = true
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); err == nil {
		codex = true
	}
	if _, err := exec.LookPath("claude"); err == nil {
		claude = true
	}
	if _, err := exec.LookPath("codex"); err == nil {
		codex = true
	}
	return
}

// runSetup writes docmap hook entries for the detected (or named) agents.
// It merges into existing settings and never removes other hooks.
func runSetup(args []string) int {
	agent, global, check := "", false, false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--global":
			global = true
		case a == "--check":
			check = true
		case strings.HasPrefix(a, "--agent="):
			agent = strings.TrimPrefix(a, "--agent=")
		case a == "--agent" && i+1 < len(args):
			agent = args[i+1]
			i++
		default:
			fmt.Fprintf(os.Stderr, "unknown option: %s\nusage: docmap setup [--agent claude|codex|both] [--global] [--check]\n", a)
			return 1
		}
	}
	claude, codex := detectAgents()
	switch agent {
	case "claude":
		claude, codex = true, false
	case "codex":
		claude, codex = false, true
	case "both":
		claude, codex = true, true
	case "":
	default:
		fmt.Fprintf(os.Stderr, "unknown agent: %s (claude, codex, both)\n", agent)
		return 1
	}
	if !claude && !codex {
		fmt.Println("No agent detected (no ~/.claude or ~/.codex). Use --agent claude|codex.")
		return 1
	}
	root := hookRoot()
	home, _ := os.UserHomeDir()
	var targets []struct {
		name, path string
		codex      bool
	}
	if claude {
		p := filepath.Join(root, ".claude", "settings.local.json")
		if global {
			p = filepath.Join(home, ".claude", "settings.json")
		}
		targets = append(targets, struct {
			name, path string
			codex      bool
		}{"claude", p, false})
	}
	if codex {
		p := filepath.Join(root, ".codex", "hooks.json")
		if global {
			p = filepath.Join(home, ".codex", "hooks.json")
		}
		targets = append(targets, struct {
			name, path string
			codex      bool
		}{"codex", p, true})
	}
	exit, wrote := 0, false
	for _, t := range targets {
		specs := docmapHookSpecs(t.codex)
		if check {
			missing := missingHooks(t.path, specs)
			if len(missing) == 0 {
				fmt.Printf("%s: 3/3 docmap hooks present (%s)\n", t.name, t.path)
			} else {
				fmt.Printf("%s: %d/3 docmap hooks missing (%s): run docmap setup --agent %s\n", t.name, len(missing), t.path, t.name)
				exit = 1
			}
			continue
		}
		added, err := ensureHooks(t.path, specs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", t.name, err)
			return 1
		}
		if added > 0 {
			wrote = true
		}
		fmt.Printf("%s: %d hooks added, %d already present (%s)\n", t.name, added, len(specs)-added, t.path)
	}
	if wrote {
		fmt.Println("Next: start a new session")
	}
	return exit
}

func readHooks(path string) (root map[string]any, hooks map[string]any, err error) {
	root = map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	hooks, _ = root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		root["hooks"] = hooks
	}
	return root, hooks, nil
}

func hasHook(hooks map[string]any, spec hookSpec) bool {
	entries, _ := hooks[spec.Event].([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		list, _ := entry["hooks"].([]any)
		for _, h := range list {
			hook, _ := h.(map[string]any)
			cmd, _ := hook["command"].(string)
			if strings.Contains(cmd, spec.Command) {
				return true
			}
		}
	}
	return false
}

func missingHooks(path string, specs []hookSpec) []hookSpec {
	_, hooks, err := readHooks(path)
	if err != nil {
		return specs
	}
	var out []hookSpec
	for _, s := range specs {
		if !hasHook(hooks, s) {
			out = append(out, s)
		}
	}
	return out
}

// ensureHooks appends each missing spec under its event and rewrites the
// file only when something was added.
func ensureHooks(path string, specs []hookSpec) (added int, err error) {
	root, hooks, err := readHooks(path)
	if err != nil {
		return 0, err
	}
	for _, s := range specs {
		if hasHook(hooks, s) {
			continue
		}
		entry := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": s.Command}}}
		if s.Matcher != "" {
			entry["matcher"] = s.Matcher
		}
		entries, _ := hooks[s.Event].([]any)
		hooks[s.Event] = append(entries, entry)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return 0, err
	}
	return added, os.WriteFile(path, append(data, '\n'), 0o644)
}
