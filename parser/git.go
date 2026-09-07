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
//
// New files (untracked, or added after `ref`) are treated as fully changed.
// If the file isn't in a git work tree, the ref doesn't exist, or git isn't
// available, ChangedLines returns an empty set and a nil error — callers
// should treat "nothing changed" as the fall-through behavior.
func ChangedLines(file, ref string) (map[int]bool, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return map[int]bool{}, nil
	}
	root := gitRoot(filepath.Dir(abs))
	if root == "" {
		return map[int]bool{}, nil
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return map[int]bool{}, nil
	}
	rel = filepath.ToSlash(rel)

	if !gitRevExists(root, ref) {
		return map[int]bool{}, nil
	}

	cmd := exec.Command("git", "-C", root, "diff", "--unified=0", ref, "--", rel)
	out, err := cmd.Output()
	if err != nil {
		// `git diff --exit-code` (or diff.exitCode) returns 1 when there
		// are differences; stdout is still the patch. Other non-zero codes
		// are real failures.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return parseHunkLines(string(out)), nil
		}
		return map[int]bool{}, nil
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

func gitRoot(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitRevExists(root, ref string) bool {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return cmd.Run() == nil
}

func fileExistedAt(root, ref, rel string) bool {
	cmd := exec.Command("git", "-C", root, "cat-file", "-e", ref+":"+rel)
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
		line := scanner.Text()
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
			// so the containing section still shows up.
			changed[start] = true
			continue
		}
		for i := start; i < start+count; i++ {
			changed[i] = true
		}
	}
	return changed
}
