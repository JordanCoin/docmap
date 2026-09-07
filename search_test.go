package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchRootsTermsCompactAndJSON(t *testing.T) {
	root := t.TempDir()
	a, b := root+"/a", root+"/b"
	if err := os.MkdirAll(a, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a+"/one.md", []byte("# Alpha\nhello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b+"/two.md", []byte("# Beta\nhello"), 0644); err != nil {
		t.Fatal(err)
	}
	docs, err := parsePath(a)
	if err != nil {
		t.Fatal(err)
	}
	other, err := parsePath(b)
	if err != nil {
		t.Fatal(err)
	}
	docs = append(docs, other...)
	termsPath := root + "/terms.txt"
	if err := os.WriteFile(termsPath, []byte("# ignored\n\nhello\n beta \n"), 0644); err != nil {
		t.Fatal(err)
	}
	terms, err := readTerms("", termsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 2 || terms[0] != "hello" || terms[1] != "beta" {
		t.Fatalf("terms = %#v", terms)
	}
	out := captureOutput(func() { outputSearch(docs, terms, false, true) })
	if !strings.Contains(out, "## hello\none.md > Alpha\ntwo.md > Beta") || !strings.Contains(out, "## beta\ntwo.md > Beta") {
		t.Fatalf("compact output = %q", out)
	}
	jsonOut := captureOutput(func() { outputSearch(docs, []string{"missing"}, true, false) })
	var got []struct {
		Term    string `json:"term"`
		File    string `json:"file"`
		Section string `json:"section"`
		Tokens  int    `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("no-hit JSON = %#v", got)
	}
}

func TestSearchCLI(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "docmap")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	if err := os.MkdirAll(a, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(a, "one.md"), []byte("# Alpha\nhello"), 0644)
	os.WriteFile(filepath.Join(b, "two.md"), []byte("# Beta\nhello"), 0644)
	run := func(args ...string) string {
		cmd := exec.Command(bin, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	out := run(a, b, "--search", "hello", "--compact")
	if out != "one.md > Alpha\ntwo.md > Beta\n" {
		t.Fatalf("compact = %q", out)
	}
	out = run(a, filepath.Join(b, "two.md"), "--search", "hello", "--compact")
	if out != "one.md > Alpha\ntwo.md > Beta\n" {
		t.Fatalf("standalone merge = %q", out)
	}
	terms := filepath.Join(root, "terms.txt")
	os.WriteFile(terms, []byte("# ignore\n\nbeta\nhello\n"), 0644)
	out = run(a, b, "--terms-file", terms, "--compact")
	if !strings.Contains(out, "## beta\n") || !strings.Contains(out, "## hello\n") {
		t.Fatalf("grouped = %q", out)
	}
	jsonText := run(a, b, "--search", "hello", "--json")
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonText), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["term"] != "hello" || rows[0]["file"] == nil || rows[0]["section"] == nil {
		t.Fatalf("json = %#v", rows)
	}
	if _, ok := rows[0]["tokens"].(float64); !ok {
		t.Fatalf("tokens missing/nonnumeric: %#v", rows[0])
	}
	if got := run(a, "--search", "missing", "--json"); strings.TrimSpace(got) != "[]" {
		t.Fatalf("no-hit json = %q", got)
	}
	manifest := `{"root":"stdin","files":[{"path":"x.md","content":"# Stdin\nhello"}]}`
	cmd := exec.Command(bin, "--stdin", "--search", "hello", "--json")
	cmd.Stdin = strings.NewReader(manifest)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(outBytes), `"file":"x.md"`) {
		t.Fatalf("stdin json = %s", outBytes)
	}
}

func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	r.Close()
	return buf.String()
}
