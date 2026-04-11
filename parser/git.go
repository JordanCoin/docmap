package parser

import (
	"bufio"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ChangedLines returns the set of line numbers in `file` that have been
// modified since the given git ref. It shells out to `git diff --unified=0`
// and parses the hunk headers, which in unified-0 mode give exact line
// ranges in the new (working) file.
//
// If the file isn't tracked by git, the ref doesn't exist, or git isn't
// available, ChangedLines returns an empty set and a nil error — callers
// should treat "nothing changed" as the fall-through behavior.
func ChangedLines(file, ref string) (map[int]bool, error) {
	cmd := exec.Command("git", "diff", "--unified=0", ref, "--", file)
	out, err := cmd.Output()
	if err != nil {
		return map[int]bool{}, nil
	}
	return parseHunkLines(string(out)), nil
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
