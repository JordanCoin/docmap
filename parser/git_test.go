package parser

import "testing"

func TestParseHunkLines(t *testing.T) {
	diff := `diff --git a/foo.md b/foo.md
index 1234..5678 100644
--- a/foo.md
+++ b/foo.md
@@ -1,3 +1,5 @@
 existing
+new line
+another new
@@ -10 +12 @@
-old
+new
@@ -20,0 +25,3 @@
+inserted
+inserted
+inserted
`
	got := parseHunkLines(diff)

	// First hunk: lines 1-5 in new file (but with count 5 from +1,5)
	for _, line := range []int{1, 2, 3, 4, 5} {
		if !got[line] {
			t.Errorf("expected line %d to be marked changed", line)
		}
	}
	// Second hunk: line 12
	if !got[12] {
		t.Error("expected line 12 to be marked changed")
	}
	// Third hunk: lines 25, 26, 27
	for _, line := range []int{25, 26, 27} {
		if !got[line] {
			t.Errorf("expected line %d to be marked changed", line)
		}
	}

	// Line 8 shouldn't be marked.
	if got[8] {
		t.Error("line 8 should not be marked changed")
	}
}

func TestParseHunkLinesEmpty(t *testing.T) {
	got := parseHunkLines("")
	if len(got) != 0 {
		t.Errorf("empty diff should produce empty map, got %d entries", len(got))
	}
}

func TestParseHunkLinesPureDeletion(t *testing.T) {
	diff := `@@ -5,3 +5,0 @@
-removed
-removed
-removed
`
	got := parseHunkLines(diff)
	// Pure deletion: anchor line should be recorded so the containing
	// section still shows up.
	if !got[5] {
		t.Error("expected anchor line 5 to be marked for pure deletion hunk")
	}
}
