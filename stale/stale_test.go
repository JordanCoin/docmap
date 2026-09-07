package stale

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JordanCoin/docmap/parser"
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

func TestMissingPath(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "README.md"), "# Docs\n\n## Setup\n\nSee `docs/missing.md` and `./present.md`.\n")
	write(t, filepath.Join(root, "present.md"), "# Present\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "README.md")))
	doc.Filename = "README.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	if !hasKind(findings, "missing_path") {
		t.Fatalf("expected missing_path, got %+v", findings)
	}
	for _, f := range findings {
		if strings.Contains(f.Reason, "present.md") {
			t.Fatalf("present.md should exist: %+v", findings)
		}
	}
}

func TestMissingBinary(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "guide.md"), "# Guide\n\n## Tools\n\nRun `git status`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "guide.md")))
	doc.Filename = "guide.md"
	findings := Check([]*parser.Document{doc}, Options{
		Root: root,
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	if !hasKind(findings, "missing_binary") {
		t.Fatalf("expected missing_binary, got %+v", findings)
	}
}

func TestStaleDate(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "status.md"), "# Status\n\n## Last checked\n\nVerified on 2020-01-15.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "status.md")))
	doc.Filename = "status.md"
	findings := Check([]*parser.Document{doc}, Options{
		Root: root,
		Days: 90,
		Now:  time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	})
	if !hasKind(findings, "stale_date") {
		t.Fatalf("expected stale_date, got %+v", findings)
	}
}

func TestMissingEnv(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".env.example"), "KNOWN_KEY=1\n")
	write(t, filepath.Join(root, "cfg.md"), "# Config\n\n## Env\n\nSet `$MISSING_API_KEY` and `$KNOWN_KEY`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "cfg.md")))
	doc.Filename = "cfg.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	foundMissing, foundKnown := false, false
	for _, f := range findings {
		if f.Kind == "missing_env" && strings.Contains(f.Reason, "MISSING_API_KEY") {
			foundMissing = true
		}
		if strings.Contains(f.Reason, "KNOWN_KEY") {
			foundKnown = true
		}
	}
	if !foundMissing {
		t.Fatalf("expected MISSING_API_KEY, got %+v", findings)
	}
	if foundKnown {
		t.Fatalf("KNOWN_KEY should not be flagged: %+v", findings)
	}
}

func TestRemoteURL(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "links.md"), "# Links\n\n## Refs\n\nSee https://example.com/gone.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "links.md")))
	doc.Filename = "links.md"
	findings := Check([]*parser.Document{doc}, Options{
		Root:   root,
		Remote: true,
		HTTPHead: func(url string) (int, error) {
			return 404, nil
		},
	})
	if !hasKind(findings, "remote_url") {
		t.Fatalf("expected remote_url, got %+v", findings)
	}
}

func TestRemoteConfigMismatch(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "docker-compose.yml"), "services:\n  firestore:\n    environment:\n      - FIRESTORE_PORT=8380\n")
	write(t, filepath.Join(root, "notes.md"), "# Notes\n\n## Emulator\n\n`FIRESTORE_PORT=8080`\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "notes.md")))
	doc.Filename = "notes.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root, Remote: true})
	if !hasKind(findings, "remote_config") {
		t.Fatalf("expected remote_config, got %+v", findings)
	}
}

func TestAllowDomains(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "links.md"), "# Links\n\n## Refs\n\nhttps://blocked.example/x and https://ok.example/y\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "links.md")))
	doc.Filename = "links.md"
	hit := map[string]bool{}
	Check([]*parser.Document{doc}, Options{
		Root:         root,
		Remote:       true,
		AllowDomains: []string{"ok.example"},
		HTTPHead: func(url string) (int, error) {
			hit[url] = true
			return 200, nil
		},
	})
	for u := range hit {
		if strings.Contains(u, "blocked") {
			t.Fatalf("blocked domain was fetched: %s", u)
		}
	}
	if !hit["https://ok.example/y"] {
		t.Fatalf("allowed domain not fetched: %v", hit)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasKind(fs []Finding, kind string) bool {
	for _, f := range fs {
		if f.Kind == kind {
			return true
		}
	}
	return false
}
