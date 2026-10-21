package render

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/JordanCoin/docmap/parser"
)

func captureTree(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestNoColorBlanksEscapes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	applyColorPolicy()
	doc := parser.Parse("# Top\n\nbody\n\n## Big\n\n" + strings.Repeat("word ", 200) + "\n")
	doc.Filename = "x.md"
	out := captureTree(t, func() { Tree(doc) })
	if strings.Contains(out, "\033[") {
		t.Fatalf("escape in output: %q", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), `expand a section: docmap x.md --expand "Big"`) {
		t.Fatalf("footer missing or wrong: %q", out)
	}
}

func TestMultiTreeFooter(t *testing.T) {
	doc := parser.Parse("# A\n\nbody\n")
	doc.Filename = "a.md"
	out := captureTree(t, func() { MultiTree([]*parser.Document{doc}, "docs") })
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "find content: docmap docs --search <term>") {
		t.Fatalf("footer missing: %q", out)
	}
}
