package stale

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestSkipSchemeAndAPIPaths(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "paths.md"), "# Paths\n\n## Refs\n\nSee `booklink://x`, `%APPDATA%/foo`, `$HOME/bar`, `/v1/users`, `/api/orders`, and `really/missing/file.go`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "paths.md")))
	doc.Filename = "paths.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	for _, f := range findings {
		for _, bad := range []string{"booklink", "%APPDATA%", "$HOME", "/v1/", "/api/"} {
			if strings.Contains(f.Reason, bad) {
				t.Fatalf("should skip %s, got %+v", bad, findings)
			}
		}
	}
	if !hasKind(findings, "missing_path") {
		t.Fatalf("expected real missing path, got %+v", findings)
	}
}

func TestSkipCIEnvKeys(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".env.example"), "APP_KEY=1\n")
	write(t, filepath.Join(root, "ci.md"), "# CI\n\n## Env\n\nUses `$GITHUB_TOKEN` and `$RUNNER_OS` and `$CI` and `$MISSING_CI_KEY`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "ci.md")))
	doc.Filename = "ci.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	for _, f := range findings {
		if strings.Contains(f.Reason, "GITHUB_TOKEN") || strings.Contains(f.Reason, "RUNNER_OS") ||
			(f.Kind == "missing_env" && strings.Contains(f.Reason, "`CI`")) {
			t.Fatalf("CI keys should be allowlisted: %+v", findings)
		}
	}
	found := false
	for _, f := range findings {
		if f.Kind == "missing_env" && strings.Contains(f.Reason, "MISSING_CI_KEY") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected MISSING_CI_KEY, got %+v", findings)
	}
}

func TestLeafOnlyNoParentDupes(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "nest.md"), "# Parent\n\n## Child\n\nMissing `only/in/child.go`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "nest.md")))
	doc.Filename = "nest.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	n := 0
	for _, f := range findings {
		if f.Kind == "missing_path" && strings.Contains(f.Reason, "only/in/child.go") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected one finding for child path, got %d: %+v", n, findings)
	}
	for _, f := range findings {
		if f.Section == "Parent" && strings.Contains(f.Reason, "only/in/child.go") {
			t.Fatalf("parent should not claim child path: %+v", findings)
		}
	}
}

func TestPathStatMemoized(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.md"), "# A\n\n## One\n\nSee `shared/missing.go`.\n\n## Two\n\nAgain `shared/missing.go`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "a.md")))
	doc.Filename = "a.md"
	opt := Options{Root: root}
	opt = normalize(opt)
	ctx := &checkCtx{
		opt:       opt,
		repoRoot:  "",
		statCache: map[string]bool{},
		cfg:       configIndex{keys: map[string]bool{}, values: map[string]map[string]bool{}},
		helpCache: &sync.Map{},
	}
	_ = checkPaths("a.md", "A > One", "See `shared/missing.go`.", ctx, root)
	nAfterFirst := len(ctx.statCache)
	_ = checkPaths("a.md", "A > Two", "Again `shared/missing.go`.", ctx, root)
	if len(ctx.statCache) != nAfterFirst {
		t.Fatalf("stat cache grew on repeat path: before %d after %d keys=%v", nAfterFirst, len(ctx.statCache), ctx.statCache)
	}
}
