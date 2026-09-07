package parser

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
// If the file isn't in a git work tree, the ref doesn't exist, or git isn't
// available, ChangedLines returns an empty set and a nil error — callers
// should treat "nothing changed" as the fall-through behavior.
func ChangedLines(file, ref string) (map[int]bool, error) {
	abs := canonicalPath(file)
	root := gitRoot(filepath.Dir(abs))
	if root == "" {
		return map[int]bool{}, nil
	}
	rel, ok := repoRelPath(root, abs)
	if !ok {
		return map[int]bool{}, nil
	}

	if !gitRevExists(root, ref) {
		return map[int]bool{}, nil
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
