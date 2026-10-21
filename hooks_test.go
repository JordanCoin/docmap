package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookFilePaths(t *testing.T) {
	cases := map[string][]string{
		`{"tool_input":{"file_path":"/a/b.go"},"session_id":"s"}`: {"/a/b.go"},
		`{"tool_input":{"path":"c.md"}}`:                          {"c.md"},
		`{"tool_input":{"notebook_path":"n.ipynb"}}`:              {"n.ipynb"},
		`{"file_path":"top.go"}`:                                  {"top.go"},
		`{"tool_input":{"command":"*** Begin Patch\n*** Update File: x/y.go\n*** Add File: z.md\n*** End Patch"}}`: {"x/y.go", "z.md"},
		`{"tool_input":{"command":"ls"}}`: nil,
		`not json`:                        nil,
	}
	for in, want := range cases {
		got := hookFilePaths([]byte(in))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: got %v want %v", in, got, want)
		}
	}
}

func TestEmitHookShapes(t *testing.T) {
	var b strings.Builder
	emitHook(&b, "PostToolUse", "hello\n", false)
	if b.String() != "hello\n" {
		t.Errorf("claude post-edit plain: %q", b.String())
	}
	b.Reset()
	emitHook(&b, "PostToolUse", "", false)
	if b.String() != "" {
		t.Errorf("claude empty must be silent: %q", b.String())
	}
	b.Reset()
	emitHook(&b, "PostToolUse", "", true)
	if strings.TrimSpace(b.String()) != `{"continue":true}` {
		t.Errorf("codex empty post-edit must no-op: %q", b.String())
	}
	b.Reset()
	emitHook(&b, "PreToolUse", "tree", false)
	var env map[string]map[string]string
	if err := json.Unmarshal([]byte(b.String()), &env); err != nil || env["hookSpecificOutput"]["additionalContext"] != "tree" || env["hookSpecificOutput"]["hookEventName"] != "PreToolUse" {
		t.Errorf("pre-read envelope: %q", b.String())
	}
}

func TestHookPostEditCap(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 11; i++ {
		os.WriteFile(filepath.Join(root, fmt.Sprintf("d%02d.md", i)), []byte("# Title\n\nSee `pkg/thing.go` here.\n"), 0o644)
	}
	out := hookPostEdit(root, []byte(`{"tool_input":{"file_path":"`+filepath.Join(root, "pkg/thing.go")+`"}}`))
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "📚 Docs mentioning pkg/thing.go (11 sections):" {
		t.Fatalf("header: %q", lines[0])
	}
	if len(lines) != 1+postEditMaxLines+1 || lines[len(lines)-1] != "  +3 more: docmap . --mentions pkg/thing.go" {
		t.Fatalf("cap: %d lines, last %q", len(lines), lines[len(lines)-1])
	}
	if hookPostEdit(root, []byte(`{"tool_input":{"file_path":"nobody/mentions.go"}}`)) != "" {
		t.Fatal("no mentions must print nothing")
	}
}

func TestHookPreReadThreshold(t *testing.T) {
	root := t.TempDir()
	small := filepath.Join(root, "small.md")
	big := filepath.Join(root, "big.md")
	os.WriteFile(small, []byte(strings.Repeat("# H\n\ntext\n", 20)), 0o644)
	os.WriteFile(big, []byte(strings.Repeat("# H\n\ntext\n", 100)), 0o644)
	if out := hookPreRead(root, []byte(`{"tool_input":{"file_path":"`+small+`"}}`)); out != "" {
		t.Fatalf("small file must be silent: %q", out)
	}
	out := hookPreRead(root, []byte(`{"tool_input":{"file_path":"`+big+`"}}`))
	if !strings.HasSuffix(out, `Read only the section you need: docmap big.md --expand "<title>"`) {
		t.Fatalf("missing footer: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("ANSI leaked")
	}
	if n := len(strings.Split(out, "\n")); n > preReadMaxLines+2 {
		t.Fatalf("not capped: %d lines", n)
	}
	if out := hookPreRead(root, []byte(`{"tool_input":{"file_path":"`+filepath.Join(root, "x.go")+`"}}`)); out != "" {
		t.Fatal("non-doc must be silent")
	}
}

func TestEnsureHooksIdempotentAndNonDestructive(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.local.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"permissions":{"allow":["Bash"]},"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"codemap hook post-edit"}]}]}}`), 0o644)
	specs := docmapHookSpecs(false)
	added, err := ensureHooks(path, specs)
	if err != nil || added != 3 {
		t.Fatalf("first run: added=%d err=%v", added, err)
	}
	first, _ := os.ReadFile(path)
	added, err = ensureHooks(path, specs)
	if err != nil || added != 0 {
		t.Fatalf("second run: added=%d err=%v", added, err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("second run changed the file")
	}
	if !strings.Contains(string(first), "codemap hook post-edit") || !strings.Contains(string(first), `"Bash"`) {
		t.Fatal("foreign hook or permissions dropped")
	}
	if len(missingHooks(path, specs)) != 0 {
		t.Fatal("check must find all present")
	}
}
