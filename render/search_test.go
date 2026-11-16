package render

import (
	"strings"
	"testing"

	"github.com/JordanCoin/docmap/parser"
)

func searchDocs() []*parser.Document {
	a := parser.Parse("# Guide\n\nUse github for hosting.\n\n## Hub files\n\nA hub is imported by many.\nEvery hub matters.\n\n## Other\n\nThe hub again.\n")
	a.Filename = "a.md"
	b := parser.Parse("# Notes\n\n```bash\necho hi\n```\n\n## More\n\n```go\nfmt.Println()\n```\n")
	b.Filename = "b.md"
	c := parser.Parse("# Empty\n\nprose only\n")
	c.Filename = "c.md"
	return []*parser.Document{a, b, c}
}

func TestSearchResultsShowLinesAndRank(t *testing.T) {
	docs := searchDocs()
	found := RankSearchResults(FindSearchResults(docs, "hub"))
	var order []string
	for _, r := range found {
		order = append(order, r.Section.Title)
	}
	// Title match first, then whole-word matches, then "github".
	if got := strings.Join(order, "|"); got != "Hub files|Other|Guide" {
		t.Fatalf("rank order = %q", got)
	}
	if found[0].Count != 3 || !found[0].TitleMatch || !found[0].WholeWord {
		t.Fatalf("top result = %+v", found[0])
	}
	if found[2].WholeWord {
		t.Fatal(`"github" is not a whole-word match for "hub"`)
	}

	out := captureTree(t, func() { SearchResults(docs, "hub") })
	for _, want := range []string{
		"3 sections match 'hub' · 5 lines",
		"a.md > Guide > Hub files (",
		":7  A hub is imported by many.",
		":8  Every hub matters.",
		":3  Use github for hosting.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("search output missing %q:\n%s", want, out)
		}
	}
	// Document order is still what FindSearchResults returns (--compact, --json).
	if first := FindSearchResults(docs, "hub")[0].Section.Title; first != "Guide" {
		t.Fatalf("FindSearchResults should keep document order, got %q first", first)
	}
}

func TestSearchResultsCapsLongLists(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Top\n\n")
	for i := 0; i < maxSearchSections+7; i++ {
		b.WriteString("## Part\n\nneedle here\n\n")
	}
	doc := parser.Parse(b.String())
	doc.Filename = "big.md"
	out := captureTree(t, func() { SearchResults([]*parser.Document{doc}, "needle") })
	if !strings.Contains(out, "… 7 more sections") || strings.Count(out, "needle here") != maxSearchSections {
		t.Fatalf("expected %d shown and 7 hidden:\n%s", maxSearchSections, out)
	}
}

func TestTypeFilterDocs(t *testing.T) {
	docs := searchDocs()
	out := captureTree(t, func() { TypeFilterDocs(docs, "code", "", "") })
	if !strings.Contains(out, "2 code blocks in 1 of 3 files") {
		t.Fatalf("missing total:\n%s", out)
	}
	if !strings.Contains(out, "echo hi") || !strings.Contains(out, "fmt.Println()") {
		t.Fatalf("code rows should preview their first line:\n%s", out)
	}
	if strings.Contains(out, "a.md") || strings.Contains(out, "c.md") {
		t.Fatalf("documents without the construct must be skipped:\n%s", out)
	}
	out = captureTree(t, func() { TypeFilterDocs(docs, "code", "go", "") })
	if strings.Contains(out, "echo hi") || !strings.Contains(out, "1 code block") {
		t.Fatalf("--lang go:\n%s", out)
	}
	out = captureTree(t, func() { TypeFilterDocs(docs, "math", "", "") })
	if !strings.Contains(out, "No math blocks found in 3 files.") {
		t.Fatalf("empty result:\n%s", out)
	}
}

func TestResolveRefTarget(t *testing.T) {
	cases := []struct{ from, target, want string }{
		{"docs/a.md", "../README.md", "README.md"},
		{"docs/a.md", "b.md", "docs/b.md"},
		{"docs/a.md", "./sub/c.md", "docs/sub/c.md"},
		{"README.md", "docs/a.md", "docs/a.md"},
		{"README.md", "../outside.md", "../outside.md"},
		{"docs/a.md", "https://example.com/x.md", "https://example.com/x.md"},
		{"docs/a.md", "/abs/x.md", "/abs/x.md"},
	}
	for _, c := range cases {
		if got := resolveRefTarget(c.from, c.target); got != c.want {
			t.Errorf("resolveRefTarget(%q, %q) = %q, want %q", c.from, c.target, got, c.want)
		}
	}
}

func TestSectionNotFoundSuggests(t *testing.T) {
	doc := parser.Parse("# Tool\n\n## Installing\n\nx\n\n## Usage notes\n\ny\n")
	doc.Filename = "r.md"
	out := captureTree(t, func() { ExpandSection(doc, "install guide") })
	if !strings.Contains(out, "Section 'install guide' not found") || !strings.Contains(out, "closest:\n  Installing") {
		t.Fatalf("suggestion output:\n%s", out)
	}
	out = captureTree(t, func() { ExpandSection(doc, "zzz") })
	if !strings.Contains(out, "sections here:\n  Installing\n  Usage notes") {
		t.Fatalf("outline fallback:\n%s", out)
	}
}
