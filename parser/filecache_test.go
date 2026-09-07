package parser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseFileCacheHit(t *testing.T) {
	dir := t.TempDir()
	cacheDirForTest = filepath.Join(dir, ".docmap", "cache")
	t.Cleanup(func() { cacheDirForTest = "" })

	path := filepath.Join(dir, "doc.md")
	content := "# Hello\n\nWorld with `code`.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	doc1, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc1.Source != content {
		t.Fatalf("source mismatch")
	}
	if len(doc1.Sections) == 0 {
		t.Fatal("expected sections")
	}
	if len(doc1.Nodes) == 0 {
		t.Fatal("expected nodes")
	}

	entries, err := os.ReadDir(cacheDirForTest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 cache file, got %d", len(entries))
	}

	doc2, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc2.Source != doc1.Source {
		t.Fatal("cached source mismatch")
	}
	if len(doc2.Sections) != len(doc1.Sections) {
		t.Fatalf("sections: got %d want %d", len(doc2.Sections), len(doc1.Sections))
	}
	if doc2.Sections[0].Title != doc1.Sections[0].Title {
		t.Fatalf("title: got %q want %q", doc2.Sections[0].Title, doc1.Sections[0].Title)
	}
	if doc2.TotalTokens != doc1.TotalTokens {
		t.Fatalf("tokens: got %d want %d", doc2.TotalTokens, doc1.TotalTokens)
	}
	if len(doc2.Nodes) != len(doc1.Nodes) {
		t.Fatalf("nodes: got %d want %d", len(doc2.Nodes), len(doc1.Nodes))
	}
}

func TestParseFileCacheDisabled(t *testing.T) {
	dir := t.TempDir()
	cacheDirForTest = filepath.Join(dir, ".docmap", "cache")
	t.Cleanup(func() {
		cacheDirForTest = ""
		os.Unsetenv("DOCMAP_NO_CACHE")
	})
	os.Setenv("DOCMAP_NO_CACHE", "1")

	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# A\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cacheDirForTest); !os.IsNotExist(err) {
		t.Fatalf("cache dir should not exist when DOCMAP_NO_CACHE=1, err=%v", err)
	}
}

func TestParseFileCacheInvalidatesOnChange(t *testing.T) {
	dir := t.TempDir()
	cacheDirForTest = filepath.Join(dir, ".docmap", "cache")
	t.Cleanup(func() { cacheDirForTest = "" })

	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# V1\n\none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc1, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond) // ensure mtime advances on coarse FS
	if err := os.WriteFile(path, []byte("# V2\n\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc2, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc2.Sections[0].Title == doc1.Sections[0].Title && doc2.Source == doc1.Source {
		t.Fatal("expected cache miss after content change")
	}
	if doc2.Sections[0].Title != "V2" {
		t.Fatalf("got title %q want V2", doc2.Sections[0].Title)
	}
}

func TestParseFileYAMLCache(t *testing.T) {
	dir := t.TempDir()
	cacheDirForTest = filepath.Join(dir, ".docmap", "cache")
	t.Cleanup(func() { cacheDirForTest = "" })

	path := filepath.Join(dir, "cfg.yaml")
	body := "name: docmap\nversion: 1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	doc1, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc2, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc2.Sections) == 0 || doc2.Sections[0].Title != doc1.Sections[0].Title {
		t.Fatalf("yaml cache: %#v vs %#v", doc2.Sections, doc1.Sections)
	}
	if doc2.Sections[0].Children != nil {
		for _, c := range doc2.Sections[0].Children {
			if c.Parent != doc2.Sections[0] {
				t.Fatal("parent not rewired after yaml cache load")
			}
		}
	}
}
