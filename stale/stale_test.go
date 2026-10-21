package stale

import (
	"os"
	"os/exec"
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
	write(t, filepath.Join(root, "README.md"), "# Docs\n\n## Setup\n\nSee `docs/missing.md` and `./present.md`. Swift users add `Package.swift`.\n")
	write(t, filepath.Join(root, "present.md"), "# Present\n")
	write(t, filepath.Join(root, "docs", "other.md"), "# Other\n")
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
		if strings.Contains(f.Reason, "Package.swift") {
			t.Fatalf("a bare name that never existed here is unverifiable, not stale: %+v", findings)
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
	write(t, filepath.Join(root, ".env.example"), "APP_KNOWN_KEY=1\n")
	write(t, filepath.Join(root, "cfg.md"), "# Config\n\n## Env\n\nSet `$APP_MISSING_API_KEY` and `$APP_KNOWN_KEY`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "cfg.md")))
	doc.Filename = "cfg.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	foundMissing, foundKnown := false, false
	for _, f := range findings {
		if f.Kind == "missing_env" && strings.Contains(f.Reason, "APP_MISSING_API_KEY") {
			foundMissing = true
		}
		if strings.Contains(f.Reason, "KNOWN_KEY") {
			foundKnown = true
		}
	}
	if !foundMissing {
		t.Fatalf("expected APP_MISSING_API_KEY, got %+v", findings)
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
	write(t, filepath.Join(root, "really", "missing", "present.go"), "package missing\n")
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
	write(t, filepath.Join(root, "ci.md"), "# CI\n\n## Env\n\nUses `$GITHUB_TOKEN` and `$RUNNER_OS` and `$CI` and `$NODE_ENV` and `$INPUT_TOKEN` and `$ACTIONS_RUNTIME_TOKEN` and `$PATH` and `$HOME` and `$USER` and `$TERM` and `$SHELL` and `$APP_MISSING_CI_KEY`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "ci.md")))
	doc.Filename = "ci.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	for _, f := range findings {
		for _, skip := range []string{"GITHUB_TOKEN", "RUNNER_OS", "NODE_ENV", "INPUT_TOKEN", "ACTIONS_RUNTIME_TOKEN", "`PATH`", "`HOME`", "`USER`", "`TERM`", "`SHELL`"} {
			if strings.Contains(f.Reason, skip) {
				t.Fatalf("%s should be allowlisted: %+v", skip, findings)
			}
		}
		if f.Kind == "missing_env" && strings.Contains(f.Reason, "`CI`") {
			t.Fatalf("CI keys should be allowlisted: %+v", findings)
		}
	}
	found := false
	for _, f := range findings {
		if f.Kind == "missing_env" && strings.Contains(f.Reason, "APP_MISSING_CI_KEY") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected APP_MISSING_CI_KEY, got %+v", findings)
	}
}

func TestSkipOptionalStackCargoInGoRepo(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n")
	write(t, filepath.Join(root, "main.go"), "package main\n")
	write(t, filepath.Join(root, "guide.md"), "# Guide\n\n## Setup\n\nOptional `Cargo.toml` for Rust users. Real path `docs/missing.md`.\n")
	write(t, filepath.Join(root, "docs", "index.md"), "# Index\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "guide.md")))
	doc.Filename = "guide.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	for _, f := range findings {
		if f.Kind == "missing_path" && strings.Contains(f.Reason, "Cargo.toml") {
			t.Fatalf("Cargo.toml should be skipped in Go-only repo: %+v", findings)
		}
	}
	found := false
	for _, f := range findings {
		if f.Kind == "missing_path" && strings.Contains(f.Reason, "docs/missing.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected docs/missing.md finding, got %+v", findings)
	}
}

func TestGoModStillFlaggedInGoRepo(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")
	write(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	write(t, filepath.Join(root, "guide.md"), "# Guide\n\n## Setup\n\nSee `go.mod`.\n")
	git(t, root, "init", "-q")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")
	git(t, root, "rm", "-q", "go.mod")
	git(t, root, "commit", "-q", "-m", "drop go.mod")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "guide.md")))
	doc.Filename = "guide.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	found := false
	for _, f := range findings {
		if f.Kind == "missing_path" && strings.Contains(f.Reason, "go.mod") && strings.Contains(f.Reason, "removed in ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected go.mod flagged with its deleting commit, got %+v", findings)
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPathClaimsNormalizedAndUnanchoredSilent(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "cmd", "hooks.go"), "package cmd\n")
	write(t, filepath.Join(root, "scanner", "a.go"), "package scanner\n")
	write(t, filepath.Join(root, "g.md"), "# G\n\n## Refs\n\nSee `cmd/hooks.go:1482`, `cmd/hooks.go:L10-L20`, `file=\"x/y\"`, `_test.go`, `.md`, `/Fonts/`, `memory/YYYY-MM-DD.md`, `scanner/queries/`, `scanner/missing.go`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "g.md")))
	doc.Filename = "g.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	// scanner/queries/ is a directory claim with no git history here:
	// unverifiable, silent. scanner/missing.go is a file whose parent exists.
	if len(findings) != 1 || !strings.Contains(findings[0].Reason, "`scanner/missing.go`") {
		t.Fatalf("want only scanner/missing.go (file, parent exists), got %+v", findings)
	}
}

func TestEnvKeyReferencedInCodeIsNotStale(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".env.example"), "OTHER=1\n")
	write(t, filepath.Join(root, "src", "app.ts"), "const d = process.env.OPENCLAW_STATE_DIR;\n")
	write(t, filepath.Join(root, "env.md"), "# Env\n\n## Keys\n\nSet `$OPENCLAW_STATE_DIR` and `$NEVER_USED_KEY`.\n")
	git(t, root, "init", "-q")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "env.md")))
	doc.Filename = "env.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	var keys []string
	for _, f := range findings {
		if f.Kind == "missing_env" {
			keys = append(keys, f.Reason)
		}
	}
	if len(keys) != 1 || !strings.Contains(keys[0], "NEVER_USED_KEY") {
		t.Fatalf("want only NEVER_USED_KEY, got %v", keys)
	}
}

func TestBinaryMatchIsCaseSensitive(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "p.md"), "# P\n\n## Prose\n\nIf a hook prints `Next codemap:` run it. Build with `next build`.\n")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "p.md")))
	doc.Filename = "p.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root, LookPath: func(string) (string, error) { return "", os.ErrNotExist }})
	var bins []string
	for _, f := range findings {
		if f.Kind == "missing_binary" {
			bins = append(bins, f.Reason)
		}
	}
	if len(bins) != 1 || !strings.Contains(bins[0], "`next`") {
		t.Fatalf("want only `next`, got %v", bins)
	}
}

func TestOptionalStackWithEvidenceStillFlags(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Cargo.toml"), "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n")
	write(t, filepath.Join(root, "guide.md"), "# Guide\n\n## Crates\n\nSee `crates/missing/Cargo.toml`.\n")
	write(t, filepath.Join(root, "crates", "missing", "src", "lib.rs"), "")
	doc := parser.Parse(mustRead(t, filepath.Join(root, "guide.md")))
	doc.Filename = "guide.md"
	findings := Check([]*parser.Document{doc}, Options{Root: root})
	found := false
	for _, f := range findings {
		if f.Kind == "missing_path" && strings.Contains(f.Reason, "crates/missing/Cargo.toml") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing_path for nested Cargo.toml when Rust evidence exists, got %+v", findings)
	}
}

func TestLeafOnlyNoParentDupes(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "nest.md"), "# Parent\n\n## Child\n\nMissing `only/in/child.go`.\n")
	write(t, filepath.Join(root, "only", "in", "other.go"), "package in\n")
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
