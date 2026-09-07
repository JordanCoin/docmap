package parser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitDiffSections(t *testing.T) {
	diff := `diff --git a/docs/a.md b/docs/a.md
--- a/docs/a.md
+++ b/docs/a.md
@@ -1 +1 @@
-old
+new
diff --git a/docs/old.md b/docs/renamed.md
similarity index 80%
rename from docs/old.md
rename to docs/renamed.md
--- a/docs/old.md
+++ b/docs/renamed.md
@@ -2 +2 @@
-x
+y
diff --git a/docs/x.pdf b/docs/x.pdf
Binary files a/docs/x.pdf and b/docs/x.pdf differ
`
	got := splitDiffSections(diff)
	if _, ok := got["docs/a.md"]; !ok {
		t.Fatalf("missing docs/a.md, got keys %v", keysOf(got))
	}
	if _, ok := got["docs/renamed.md"]; !ok {
		t.Fatalf("missing renamed path, got keys %v", keysOf(got))
	}
	if !isBinaryDiff(got["docs/x.pdf"]) {
		t.Fatal("expected binary section for docs/x.pdf")
	}
	if strings.Contains(got["docs/a.md"], "renamed.md") {
		t.Fatal("section for a.md leaked rename content")
	}
}

func TestChangedLinesBatch(t *testing.T) {
	repo, guide := initGitDocRepo(t)
	write(t, guide, "# Guide\n\n## Setup\n\nhello\n")
	newbie := filepath.Join(repo, "docs", "new.md")
	write(t, newbie, "# New\n\n## Body\n\ntext\n")
	gitCommit(t, repo, "second")

	other := filepath.Join(repo, "docs", "other.md")
	write(t, other, "# Other\n\nedited\n")

	batch, err := ChangedLinesBatch(repo, "HEAD~1", []string{guide, newbie, other})
	if err != nil {
		t.Fatal(err)
	}
	if !batch[guide][4] && !batch[guide][5] {
		t.Fatalf("guide should show setup edits, got %v", batch[guide])
	}
	if !batch[newbie][1] {
		t.Fatalf("committed-after-ref file should be fully changed, got %v", batch[newbie])
	}
	if !batch[other][1] {
		t.Fatalf("untracked should be fully changed, got %v", batch[other])
	}

	unchanged, err := ChangedLinesBatch(repo, "HEAD", []string{guide})
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged[guide]) != 0 {
		t.Fatalf("HEAD should be empty for committed guide, got %v", unchanged[guide])
	}
}

func TestChangedLinesBatchBadRef(t *testing.T) {
	repo, guide := initGitDocRepo(t)
	_, err := ChangedLinesBatch(repo, "nope", []string{guide})
	if !errors.Is(err, ErrBadRef) {
		t.Fatalf("expected ErrBadRef, got %v", err)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

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
	if !errors.Is(err, ErrBadRef) {
		t.Fatalf("expected ErrBadRef, got %v", err)
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
	if !errors.Is(err, ErrNotRepo) {
		t.Fatalf("expected ErrNotRepo, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("non-repo should be empty, got %v", got)
	}
}

func TestChangedPathsDeleted(t *testing.T) {
	repo, guide := initGitDocRepo(t)
	if err := os.Remove(guide); err != nil {
		t.Fatal(err)
	}
	changes, err := ChangedPaths(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range changes {
		if c.Path == "docs/guide.md" && c.Status == "D" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected deleted docs/guide.md, got %+v", changes)
	}
}

func TestChangedPathsUntracked(t *testing.T) {
	repo, _ := initGitDocRepo(t)
	write(t, filepath.Join(repo, "docs", "new.md"), "# New\n")
	changes, err := ChangedPaths(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range changes {
		if c.Path == "docs/new.md" && c.Status == "A" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected untracked docs/new.md as A, got %+v", changes)
	}
}

func TestEnsureRef(t *testing.T) {
	repo, _ := initGitDocRepo(t)
	if err := EnsureRef(repo, "HEAD"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRef(repo, "nope"); !errors.Is(err, ErrBadRef) {
		t.Fatalf("expected ErrBadRef, got %v", err)
	}
}

func TestChangedPathsRename(t *testing.T) {
	repo, guide := initGitDocRepo(t)
	dest := filepath.Join(repo, "docs", "renamed.md")
	runGit(t, repo, "mv", guide, dest)
	write(t, dest, "# Guide\n\n## Intro\n\nstart\nedited\n")
	runGit(t, repo, "add", "-A")

	changes, err := ChangedPaths(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range changes {
		if c.Status == "R" && c.Path == "docs/renamed.md" && c.OldPath == "docs/guide.md" {
			found = true
		}
		if c.Status == "D" && c.Path == "docs/guide.md" {
			t.Fatalf("rename should not appear as delete, got %+v", changes)
		}
	}
	if !found {
		t.Fatalf("expected rename R docs/guide.md -> docs/renamed.md, got %+v", changes)
	}

	got, err := ChangedLines(dest, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("rename with edit should report changed lines")
	}
	if len(got) > 3 {
		t.Fatalf("rename should not mark whole file when only a line was added, got %v", got)
	}
}

func TestChangedPathsIgnoresNonDocs(t *testing.T) {
	repo, _ := initGitDocRepo(t)
	write(t, filepath.Join(repo, "main.go"), "package main\n")
	runGit(t, repo, "add", "main.go")
	gitCommit(t, repo, "code")
	if err := os.Remove(filepath.Join(repo, "main.go")); err != nil {
		t.Fatal(err)
	}
	changes, err := ChangedPaths(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if strings.HasSuffix(c.Path, ".go") {
			t.Fatalf("non-doc delete leaked: %+v", changes)
		}
	}

	all, err := ChangedPathsAll(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	foundGo := false
	for _, c := range all {
		if c.Path == "main.go" && c.Status == "D" {
			foundGo = true
			break
		}
	}
	if !foundGo {
		t.Fatalf("ChangedPathsAll should include deleted .go: %+v", all)
	}
}

func TestChangedPathsAllIncludesNonDocs(t *testing.T) {
	repo, _ := initGitDocRepo(t)
	write(t, filepath.Join(repo, "main.go"), "package main\n")
	runGit(t, repo, "add", "main.go")
	gitCommit(t, repo, "code")
	if err := os.Remove(filepath.Join(repo, "main.go")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, "helper.go"), "package main\n")

	all, err := ChangedPathsAll(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	var deletedGo, untrackedGo bool
	for _, c := range all {
		if c.Path == "main.go" && c.Status == "D" {
			deletedGo = true
		}
		if c.Path == "helper.go" && c.Status == "A" {
			untrackedGo = true
		}
	}
	if !deletedGo {
		t.Fatalf("ChangedPathsAll missing deleted .go: %+v", all)
	}
	if !untrackedGo {
		t.Fatalf("ChangedPathsAll missing untracked .go: %+v", all)
	}

	docsOnly, err := ChangedPaths(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range docsOnly {
		if strings.HasSuffix(c.Path, ".go") {
			t.Fatalf("ChangedPaths should still ignore .go, got %+v", docsOnly)
		}
	}
}

func TestChangedLinesBinary(t *testing.T) {
	repo, _ := initGitDocRepo(t)
	pdf := filepath.Join(repo, "docs", "x.pdf")
	if err := os.WriteFile(pdf, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "docs/x.pdf")
	gitCommit(t, repo, "pdf")
	if err := os.WriteFile(pdf, []byte{0x00, 0x01, 0x03, 0x04}, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ChangedLines(pdf, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("binary change should mark lines")
	}
}

func TestParseHunkLinesBinaryBanner(t *testing.T) {
	diff := "diff --git a/x.pdf b/x.pdf\nBinary files a/x.pdf and b/x.pdf differ\n"
	if !isBinaryDiff(diff) {
		t.Fatal("expected binary detection")
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
