package main

import (
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
