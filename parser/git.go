package parser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrNotRepo is returned when the path is not inside a git work tree.
	ErrNotRepo = errors.New("not a git repository")
	// ErrBadRef is returned when --since names a ref git cannot resolve.
	ErrBadRef = errors.New("unknown git ref")
)

// ChangedLines returns the set of line numbers in `file` that have been
// modified since the given git ref. It shells out to `git diff --unified=0`
// and parses the hunk headers, which in unified-0 mode give exact line
// ranges in the new (working) file.
//
// Git is always run from the file's repository root, so the caller does not
// have to be in the work tree (or even in the same directory as the file).
// Paths are canonicalized with EvalSymlinks so macOS /var vs /private/var
// (and any other intermediate symlink) still maps onto git's pathspec.
//
// New files (untracked, or added after `ref`) are treated as fully changed.
// Renames compare against the pre-rename blob so only real edits light up.
// Binary diffs (no text hunks) light up every current line.
// Missing git, a path outside any work tree, or an unknown ref return a
// typed error (ErrNotRepo / ErrBadRef) with an empty set.
func ChangedLines(file, ref string) (map[int]bool, error) {
	abs := canonicalPath(file)
	root := gitRoot(filepath.Dir(abs))
	if root == "" {
		return map[int]bool{}, fmt.Errorf("%w", ErrNotRepo)
	}
	batch, err := ChangedLinesBatch(root, ref, []string{abs})
	if err != nil {
		return map[int]bool{}, err
	}
	if lines, ok := batch[abs]; ok {
		return lines, nil
	}
	return map[int]bool{}, nil
}

// ChangedLinesBatch returns changed line sets for each path in files since
// ref, keyed by the exact strings passed in. It runs one repo-level
// `git diff --unified=0 -M ref` (plus one ls-tree for paths absent from the
// diff) instead of spawning git once per file.
//
// Behavior matches ChangedLines: renames light up only real edits, binaries
// and new/untracked files mark every current line, and ErrNotRepo / ErrBadRef
// are returned when the root or ref is invalid.
func ChangedLinesBatch(root, ref string, files []string) (map[string]map[int]bool, error) {
	result := make(map[string]map[int]bool, len(files))
	absRoot := canonicalPath(root)
	gitroot := gitRoot(absRoot)
	if gitroot == "" {
		if fi, err := os.Stat(absRoot); err == nil && !fi.IsDir() {
			gitroot = gitRoot(filepath.Dir(absRoot))
		}
	}
	if gitroot == "" {
		return nil, fmt.Errorf("%w", ErrNotRepo)
	}
	if !gitRevExists(gitroot, ref) {
		return nil, fmt.Errorf("%w %q", ErrBadRef, ref)
	}
	if len(files) == 0 {
		return result, nil
	}

	cmdOut, err := runRepoDiff(gitroot, ref)
	if err != nil && !isDiffExit(err) && len(cmdOut) == 0 {
		return nil, err
	}

	byRel := make(map[string]map[int]bool)
	for rel, section := range splitDiffSections(string(cmdOut)) {
		abs := filepath.Join(gitroot, filepath.FromSlash(rel))
		byRel[rel] = linesFromDiff(section, abs)
	}

	var atRef map[string]bool
	for _, file := range files {
		abs := canonicalPath(file)
		rel, ok := repoRelPath(gitroot, abs)
		if !ok {
			result[file] = map[int]bool{}
			continue
		}
		if lines, ok := byRel[rel]; ok {
			result[file] = lines
			continue
		}
		if atRef == nil {
			atRef = filesAtRef(gitroot, ref)
		}
		if atRef[rel] {
			result[file] = map[int]bool{}
			continue
		}
		result[file] = allFileLines(abs)
	}
	return result, nil
}

func isDiffExit(err error) bool {
	ee, ok := err.(*exec.ExitError)
	return ok && ee.ExitCode() == 1
}

func runRepoDiff(root, ref string) ([]byte, error) {
	cmd := gitCommand(root, "diff", "--no-color", "--no-ext-diff", "--unified=0", "-M", ref)
	return cmd.Output()
}

// splitDiffSections splits a multi-file unified diff into per-path bodies
// keyed by the new (b/) path, slash-separated and relative to the repo root.
func splitDiffSections(diff string) map[string]string {
	sections := map[string]string{}
	if strings.TrimSpace(diff) == "" {
		return sections
	}

	var curPath string
	var buf strings.Builder
	flush := func() {
		if curPath != "" {
			sections[curPath] = buf.String()
		}
		buf.Reset()
		curPath = ""
	}

	scanner := bufio.NewScanner(strings.NewReader(diff))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			curPath = pathFromDiffGitLine(line)
			buf.WriteString(line)
			buf.WriteByte('\n')
		case strings.HasPrefix(line, "rename to "):
			curPath = filepath.ToSlash(strings.TrimPrefix(line, "rename to "))
			buf.WriteString(line)
			buf.WriteByte('\n')
		case strings.HasPrefix(line, "+++ b/"):
			p := trimDiffPath(strings.TrimPrefix(line, "+++ b/"))
			if p != "/dev/null" && p != "" {
				curPath = filepath.ToSlash(p)
			}
			buf.WriteString(line)
			buf.WriteByte('\n')
		default:
			if curPath != "" || buf.Len() > 0 {
				buf.WriteString(line)
				buf.WriteByte('\n')
			}
		}
	}
	flush()
	return sections
}

func pathFromDiffGitLine(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return filepath.ToSlash(rest[i+3:])
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return filepath.ToSlash(strings.TrimPrefix(fields[len(fields)-1], "b/"))
}

func trimDiffPath(p string) string {
	if i := strings.IndexByte(p, '\t'); i >= 0 {
		return p[:i]
	}
	return p
}

func filesAtRef(root, ref string) map[string]bool {
	cmd := gitCommand(root, "ls-tree", "-r", "--name-only", "--full-tree", ref)
	out, err := cmd.Output()
	if err != nil {
		return map[string]bool{}
	}
	paths := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths[filepath.ToSlash(line)] = true
	}
	return paths
}

func linesFromDiff(diff, abs string) map[int]bool {
	if isBinaryDiff(diff) {
		return allFileLines(abs)
	}
	changed := parseHunkLines(diff)
	if len(changed) == 0 && strings.Contains(diff, "diff --git") {
		// Textless change (mode-only / binary without the usual banner).
		return allFileLines(abs)
	}
	return changed
}

func isBinaryDiff(diff string) bool {
	return strings.Contains(diff, "Binary files ") ||
		strings.Contains(diff, "GIT binary patch")
}

// GitRoot returns the work tree root that contains dir, or "" if dir is
// not inside a git repository.
func GitRoot(dir string) string {
	return gitRoot(dir)
}

// EnsureRef reports whether dir is in a git work tree and ref names a commit.
func EnsureRef(dir, ref string) error {
	abs := canonicalPath(dir)
	root := gitRoot(abs)
	if root == "" {
		if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
			root = gitRoot(filepath.Dir(abs))
		}
	}
	if root == "" {
		return fmt.Errorf("%w: %s", ErrNotRepo, dir)
	}
	if !gitRevExists(root, ref) {
		return fmt.Errorf("%w %q", ErrBadRef, ref)
	}
	return nil
}

// PathChange is one path git reports as different from ref.
type PathChange struct {
	Path    string // relative to the directory passed to ChangedPaths
	OldPath string // set for renames (R) / copies (C)
	Status  string // A, M, D, R, C (untracked files are A)
}

// ChangedPaths lists documentation files that differ from ref under dir,
// including deletions, renames, and untracked files. Paths are slash-
// separated and relative to dir. Non-doc paths are omitted.
func ChangedPaths(dir, ref string) ([]PathChange, error) {
	all, err := ChangedPathsAll(dir, ref)
	if err != nil {
		return nil, err
	}
	var out []PathChange
	for _, c := range all {
		if isDocPath(c.Path) || (c.OldPath != "" && isDocPath(c.OldPath)) {
			out = append(out, c)
		}
	}
	return out, nil
}

// ChangedPathsAll lists every path that differs from ref under dir,
// including non-doc files (e.g. .go), deletions, renames, and untracked
// files. Paths are slash-separated and relative to dir.
func ChangedPathsAll(dir, ref string) ([]PathChange, error) {
	if err := EnsureRef(dir, ref); err != nil {
		return nil, err
	}
	abs := canonicalPath(dir)
	if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
		abs = filepath.Dir(abs)
	}
	root := gitRoot(abs)
	spec, ok := repoRelPath(root, abs)
	if !ok || spec == "" {
		spec = "."
	}
	cmd := gitCommand(root, "diff", "--no-color", "--name-status", "-M", "-z", ref, "--", spec)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
			if len(out) == 0 {
				return nil, err
			}
		}
	}
	changes := parseNameStatus(string(out), root, abs)

	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Path] = true
		if c.OldPath != "" {
			seen[c.OldPath] = true
		}
	}

	ls := gitCommand(abs, "ls-files", "-z", "--others", "--exclude-standard", "--", ".")
	lsOut, lsErr := ls.Output()
	if lsErr == nil {
		for _, rel := range strings.Split(string(lsOut), "\x00") {
			if rel == "" {
				continue
			}
			rel = filepath.ToSlash(rel)
			if seen[rel] {
				continue
			}
			changes = append(changes, PathChange{Path: rel, Status: "A"})
			seen[rel] = true
		}
	}
	return changes, nil
}

// IsDocPath reports whether path looks like a documentation file docmap walks.
func IsDocPath(rel string) bool {
	return isDocPath(rel)
}

func isDocPath(rel string) bool {
	lower := strings.ToLower(rel)
	return strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".pdf") ||
		strings.HasSuffix(lower, ".yaml") ||
		strings.HasSuffix(lower, ".yml")
}

func parseNameStatus(raw, root, abs string) []PathChange {
	parts := strings.Split(raw, "\x00")
	var out []PathChange
	for i := 0; i < len(parts); {
		status := strings.TrimSpace(parts[i])
		if status == "" {
			i++
			continue
		}
		code := status[:1]
		if code == "R" || code == "C" {
			if i+2 >= len(parts) {
				break
			}
			oldRel, _ := relUnder(root, abs, parts[i+1])
			newRel, okNew := relUnder(root, abs, parts[i+2])
			i += 3
			if !okNew {
				continue
			}
			out = append(out, PathChange{Path: newRel, OldPath: oldRel, Status: code})
			continue
		}
		if i+1 >= len(parts) {
			break
		}
		rel, ok := relUnder(root, abs, parts[i+1])
		i += 2
		if !ok {
			continue
		}
		out = append(out, PathChange{Path: rel, Status: code})
	}
	return out
}

func relUnder(root, abs, path string) (string, bool) {
	full := canonicalPath(filepath.Join(root, filepath.FromSlash(path)))
	rel, err := filepath.Rel(abs, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// RecentFiles returns up to `limit` unique paths changed in recent commits
// under dir, newest first. Paths are relative to dir. Returns nil when git
// is unavailable or dir is not a work tree.
func RecentFiles(dir string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	abs := canonicalPath(dir)
	root := gitRoot(abs)
	if root == "" {
		return nil
	}
	cmd := gitCommand(root, "log", "-n", "80", "--name-only", "--pretty=format:", "--diff-filter=ACMR")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		full := canonicalPath(filepath.Join(root, filepath.FromSlash(line)))
		rel, err := filepath.Rel(abs, full)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		rel = filepath.ToSlash(rel)
		if seen[rel] {
			continue
		}
		seen[rel] = true
		files = append(files, rel)
		if len(files) >= limit {
			break
		}
	}
	return files
}

func gitCommand(root string, args ...string) *exec.Cmd {
	all := append([]string{"-C", root, "-c", "color.ui=never"}, args...)
	return exec.Command("git", all...)
}

func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if eval, err := filepath.EvalSymlinks(abs); err == nil {
		return eval
	}
	return abs
}

func gitRoot(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(string(out))
	if eval, err := filepath.EvalSymlinks(root); err == nil {
		return eval
	}
	return root
}

func repoRelPath(root, abs string) (string, bool) {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func gitRevExists(root, ref string) bool {
	cmd := gitCommand(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return cmd.Run() == nil
}

func allFileLines(path string) map[int]bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return map[int]bool{}
	}
	n := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		n++
	}
	changed := make(map[int]bool, n)
	for i := 1; i <= n; i++ {
		changed[i] = true
	}
	return changed
}

// hunkHeaderRe matches the `+start,count` part of a unified diff hunk
// header. Count is optional — a missing count means 1.
var hunkHeaderRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func parseHunkLines(diff string) map[int]bool {
	changed := map[int]bool{}
	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := stripANSI(scanner.Text())
		m := hunkHeaderRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		count := 1
		if m[2] != "" {
			c, err := strconv.Atoi(m[2])
			if err != nil {
				continue
			}
			count = c
		}
		if count == 0 {
			// Pure deletion — record the anchor line as "changed nearby"
			// so the containing section still shows up. +0,0 (deleted
			// from the start of the file) has no current line; skip it.
			if start > 0 {
				changed[start] = true
			}
			continue
		}
		for i := start; i < start+count; i++ {
			changed[i] = true
		}
	}
	return changed
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}
