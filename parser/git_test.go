package parser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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

func TestParseHunkLinesStripsANSI(t *testing.T) {
	diff := "\x1b[1m@@ -1 +2 @@\x1b[0m\n+new\n"
	got := parseHunkLines(diff)
	if !got[2] {
		t.Fatalf("ANSI-colored hunk header should still parse, got %v", got)
	}
}

func TestChangedLinesViaSymlink(t *testing.T) {
	repo, guide := initGitDocRepo(t)
	write(t, guide, "# Guide\n\n## Setup\n\nhello\n")
	gitCommit(t, repo, "second")

	parent := t.TempDir()
	link := filepath.Join(parent, "work")
	if err := os.Symlink(repo, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	linked := filepath.Join(link, "docs", "guide.md")
	got, err := ChangedLines(linked, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if !got[4] && !got[5] {
		t.Fatalf("symlink path should still see the diff, got %v", got)
	}
}

func TestChangedLinesFromOtherCwd(t *testing.T) {
	repo, guide := initGitDocRepo(t)
	write(t, guide, "# Guide\n\n## Setup\n\nhello\n")
	gitCommit(t, repo, "second")

	// The temp repo is not the test process cwd. Before the fix, git diff
	// ran in cwd and `--since` always reported no changes.
	got, err := ChangedLines(guide, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if !got[4] && !got[5] {
		t.Fatalf("expected setup/body lines changed from outside the repo, got %v", got)
	}
}

func TestChangedLinesUnchanged(t *testing.T) {
	_, guide := initGitDocRepo(t)
	got, err := ChangedLines(guide, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unchanged file should be empty, got %v", got)
	}
}

func TestChangedLinesNewFile(t *testing.T) {
	repo, _ := initGitDocRepo(t)
	newbie := filepath.Join(repo, "docs", "new.md")
	write(t, newbie, "# New\n\n## Body\n\ntext\n")

	got, err := ChangedLines(newbie, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !got[1] || !got[3] {
		t.Fatalf("new file should mark all lines, got %v", got)
	}
}

func TestChangedLinesMissingRef(t *testing.T) {
	_, guide := initGitDocRepo(t)
	got, err := ChangedLines(guide, "this-ref-does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("missing ref should be empty, got %v", got)
	}
}

func TestChangedLinesNotARepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	write(t, path, "# Notes\n")
	got, err := ChangedLines(path, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("non-repo should be empty, got %v", got)
	}
}

func initGitDocRepo(t *testing.T) (repo, guide string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo = t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "docmap@example.com")
	runGit(t, repo, "config", "user.name", "docmap")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	guide = filepath.Join(repo, "docs", "guide.md")
	write(t, guide, "# Guide\n\n## Intro\n\nstart\n")
	runGit(t, repo, "add", ".")
	gitCommit(t, repo, "initial")
	return repo, guide
}

func gitCommit(t *testing.T, repo, msg string) {
	t.Helper()
	runGit(t, repo, "commit", "-q", "-am", msg)
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
