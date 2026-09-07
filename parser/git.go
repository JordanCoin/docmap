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
// Missing git, a path outside any work tree, or an unknown ref return a
// typed error (ErrNotRepo / ErrBadRef) with an empty set.
func ChangedLines(file, ref string) (map[int]bool, error) {
	abs := canonicalPath(file)
	root := gitRoot(filepath.Dir(abs))
	if root == "" {
		return map[int]bool{}, fmt.Errorf("%w", ErrNotRepo)
	}
	rel, ok := repoRelPath(root, abs)
	if !ok {
		return map[int]bool{}, fmt.Errorf("%w", ErrNotRepo)
	}

	if !gitRevExists(root, ref) {
		return map[int]bool{}, fmt.Errorf("%w %q", ErrBadRef, ref)
	}

	cmd := gitCommand(root, "diff", "--no-color", "--no-ext-diff", "--unified=0", ref, "--", rel)
	out, err := cmd.Output()
	if err != nil {
		// `git diff --exit-code` (or diff.exitCode) returns 1 when there
		// are differences; stdout is still the patch. Other non-zero codes
		// are real failures — still try the new-file fallback below.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return parseHunkLines(string(out)), nil
		}
		if fileExistedAt(root, ref, rel) {
			return map[int]bool{}, nil
		}
		return allFileLines(abs), nil
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return parseHunkLines(string(out)), nil
	}

	// Empty patch: either unchanged, or the file did not exist at ref
	// (new / untracked). The latter should light up the whole file.
	if fileExistedAt(root, ref, rel) {
		return map[int]bool{}, nil
	}
	return allFileLines(abs), nil
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
	Path   string // relative to the directory passed to ChangedPaths
	Status string // A, M, D (untracked files are A)
}

// ChangedPaths lists files that differ from ref under dir, including
// deletions and untracked files. Paths are slash-separated and relative to dir.
func ChangedPaths(dir, ref string) ([]PathChange, error) {
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
	cmd := gitCommand(root, "diff", "--no-color", "--name-status", "--no-renames", "-z", ref, "--", spec)
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
			if !isDocPath(rel) {
				continue
			}
			changes = append(changes, PathChange{Path: rel, Status: "A"})
			seen[rel] = true
		}
	}
	return changes, nil
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
	for i := 0; i+1 < len(parts); i += 2 {
		status := strings.TrimSpace(parts[i])
		path := parts[i+1]
		if status == "" || path == "" {
			continue
		}
		full := canonicalPath(filepath.Join(root, filepath.FromSlash(path)))
		rel, err := filepath.Rel(abs, full)
		if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		rel = filepath.ToSlash(rel)
		code := status[:1]
		out = append(out, PathChange{Path: rel, Status: code})
	}
	return out
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

func fileExistedAt(root, ref, rel string) bool {
	cmd := gitCommand(root, "cat-file", "-e", ref+":"+rel)
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
