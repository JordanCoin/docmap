package docset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, dir string, all bool) (string, int) {
	t.Helper()
	docs, skipped := LoadDirSkipped(dir, all)
	var out []string
	for _, d := range docs {
		out = append(out, filepath.ToSlash(d.Filename))
	}
	return strings.Join(out, ","), skipped
}

// #32: the directory walk must visit the .txt files single-file mode parses.
func TestLoadDir_IncludesText(t *testing.T) {
	t.Setenv("DOCMAP_NO_CACHE", "1")
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.md"), "# Doc A\n\nSome text.\n")
	write(t, filepath.Join(dir, "e.txt"), "# Plain text doc\n\nA text file with a heading.\n")
	write(t, filepath.Join(dir, "sub", "notes.txt"), "no headings here\n")

	got, skipped := names(t, dir, false)
	if got != "a.md,e.txt,sub/notes.txt" || skipped != 0 {
		t.Fatalf("docs = %q skipped = %d", got, skipped)
	}
	if got := KindBreakdown(LoadDir(dir, false)); got != "1 md, 2 txt" {
		t.Fatalf("breakdown = %q", got)
	}
	// LoadDocs (the --stale / --mentions set) sees them too.
	if n := len(LoadDocs(dir, false)); n != 3 {
		t.Fatalf("LoadDocs = %d docs, want 3", n)
	}
}

func TestLoadDir_TextGuards(t *testing.T) {
	t.Setenv("DOCMAP_NO_CACHE", "1")
	dir := t.TempDir()
	write(t, filepath.Join(dir, "keep.txt"), "a real note\n")
	write(t, filepath.Join(dir, "requirements-dev.txt"), "pytest==8.0\n")
	write(t, filepath.Join(dir, "CMakeLists.txt"), "project(x)\n")
	write(t, filepath.Join(dir, "dump.txt"), strings.Repeat("log line\n", maxWalkTextSize/9+10))
	write(t, filepath.Join(dir, "blob.txt"), "abc\x00\x01\x02def")

	got, skipped := names(t, dir, false)
	if got != "keep.txt" {
		t.Fatalf("docs = %q, want keep.txt only", got)
	}
	if skipped != 3 {
		t.Fatalf("skipped = %d, want 3 (binary is dropped silently)", skipped)
	}

	got, skipped = names(t, dir, true)
	if got != "CMakeLists.txt,dump.txt,keep.txt,requirements-dev.txt" || skipped != 0 {
		t.Fatalf("--all docs = %q skipped = %d", got, skipped)
	}
}
