package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/JordanCoin/docmap/parser"
)

func TestParseDirectory(t *testing.T) {
	// Test with current directory (should find README.md at minimum)
	docs := parseDirectory(".")

	if len(docs) == 0 {
		t.Error("expected to find at least one markdown file")
	}

	// Check that README.md was found
	found := false
	for _, doc := range docs {
		if doc.Filename == "README.md" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find README.md")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func docNames(docs []*parser.Document) []string {
	names := make([]string, 0, len(docs))
	for _, d := range docs {
		names = append(names, filepath.ToSlash(d.Filename))
	}
	sort.Strings(names)
	return names
}

// #4: dependency directories are skipped unless --all.
func TestParseDirectorySkipsDependencyDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "README.md"), "# Project\n")
	writeFile(t, filepath.Join(dir, "docs", "guide.md"), "# Guide\n")
	writeFile(t, filepath.Join(dir, "node_modules", "dep", "README.md"), "# Dep\n")
	writeFile(t, filepath.Join(dir, "vendor", "lib", "CHANGELOG.md"), "# Vendor\n")
	writeFile(t, filepath.Join(dir, "sub", "node_modules", "x", "README.md"), "# Nested dep\n")

	got := docNames(parseDirectoryOpts(dir, false))
	want := []string{"README.md", "docs/guide.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("default walk: got %v, want %v", got, want)
	}

	all := docNames(parseDirectoryOpts(dir, true))
	if len(all) != 5 {
		t.Errorf("--all walk: got %d docs %v, want 5", len(all), all)
	}
}

// #4: inside a git work tree, .gitignore'd files are not documents.
func TestParseDirectoryHonorsGitignore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init failed: %v %s", err, out)
	}
	writeFile(t, filepath.Join(dir, ".gitignore"), "generated/\n*.tmp.md\n")
	writeFile(t, filepath.Join(dir, "README.md"), "# Project\n")
	writeFile(t, filepath.Join(dir, "generated", "api.md"), "# Generated\n")
	writeFile(t, filepath.Join(dir, "notes.tmp.md"), "# Scratch\n")
	writeFile(t, filepath.Join(dir, "docs", "untracked.md"), "# Untracked but not ignored\n")

	got := docNames(parseDirectoryOpts(dir, false))
	want := []string{"README.md", "docs/untracked.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("gitignore walk: got %v, want %v", got, want)
	}

	all := docNames(parseDirectoryOpts(dir, true))
	if len(all) != 4 {
		t.Errorf("--all walk: got %d docs %v, want 4", len(all), all)
	}
}

func TestOutputMentionsFindsPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "guide.md"), "# Guide\n\nSee `parser/git.go` when diffs break.\n")
	writeFile(t, filepath.Join(dir, "other.md"), "# Other\n\nNo git path here.\n")
	docs := parseDirectoryOpts(dir, true)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	outputMentions(docs, []string{"parser/git.go"})
	w.Close()
	os.Stdout = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "guide.md > Guide") {
		t.Fatalf("expected mention hit, got %q", out)
	}
	if strings.Contains(out, "other.md") {
		t.Fatalf("other.md should not match, got %q", out)
	}
}

func TestOutputBriefCounts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n## One\n\ntext\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n## Two\n\ntext\n")
	docs := parseDirectoryOpts(dir, true)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	outputBrief(docs, dir, 0)
	w.Close()
	os.Stdout = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "2 files") || !strings.Contains(out, "2 md") {
		t.Fatalf("brief counts, got %q", out)
	}
}

func TestConvertSectionsSince(t *testing.T) {
	doc := parser.Parse("# Keep\n\nintro\n\n## Changed\n\nedit me\n\n## Other\n\nstill\n")
	sec := doc.GetSection("Changed")
	if sec == nil {
		t.Fatal("missing Changed")
	}
	changed := map[int]bool{}
	for i := sec.LineStart; i <= sec.LineEnd; i++ {
		changed[i] = true
	}
	got := convertSectionsSince(doc.Sections, changed)
	if len(got) == 0 {
		t.Fatal("expected parent section in since JSON")
	}
	blob, _ := json.Marshal(got)
	if !strings.Contains(string(blob), "Changed") {
		t.Fatalf("expected Changed section, got %s", blob)
	}
	if strings.Contains(string(blob), "Other") {
		t.Fatalf("unchanged Other should be omitted, got %s", blob)
	}
}
