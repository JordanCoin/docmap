package parser

import (
	"strings"
	"testing"
)

func TestContainsText(t *testing.T) {
	text := "public trust in the research are under-\nmined [41]. As a community,\nwe must do   better. The key-\nvalue store"
	cases := []struct {
		q    string
		want bool
	}{
		{"undermined", true},           // end-of-line hyphenation
		{"under-", true},               // literal text still matches
		{"a community, we must", true}, // phrase across a line wrap
		{"do better", true},            // collapsed spaces
		{"key-value", true},            // hyphen kept when the query has it
		{"keyvalue", true},             // and seen through when it does not
		{"public  trust", true},        // query whitespace is normalized too
		{"mined", true},
		{"overmined", false},
		{"", false},
	}
	for _, c := range cases {
		if got := ContainsText(text, c.q); got != c.want {
			t.Errorf("ContainsText(%q) = %v, want %v", c.q, got, c.want)
		}
	}
	// "- " that is punctuation, not hyphenation, must not join words.
	if ContainsText("costs fell -\nprices rose", "fellprices") {
		t.Error("a spaced dash is not a hyphen")
	}
}

func TestSectionLineHits(t *testing.T) {
	src := "# Guide\n\nintro mentions widgets once.\n\n## Setup\n\nInstall the widget\nthen configure the wid-\nget cache. Widgets everywhere.\n\n## Other\n\nnothing here\n"
	doc := Parse(src)
	guide, setup := doc.GetSection("Guide"), doc.GetSection("Setup")

	// Own lines only: the parent must not report its subsection's lines.
	hits, n := SectionLineHits(doc, guide, "widget")
	if n != 1 || len(hits) != 1 || hits[0].Line != 3 {
		t.Fatalf("guide hits = %+v (n=%d)", hits, n)
	}
	hits, n = SectionLineHits(doc, setup, "widget")
	if n != 3 || hits[0].Line != 7 || hits[1].Line != 8 || hits[2].Line != 9 {
		// line 8 holds "wid-" + "get": a hyphenated match starts there
		t.Fatalf("setup hits = %+v (n=%d)", hits, n)
	}
	// A phrase across a wrap is reported once, where it starts.
	hits, n = SectionLineHits(doc, setup, "the widget then configure")
	if n != 1 || hits[0].Line != 7 {
		t.Fatalf("wrapped phrase hits = %+v (n=%d)", hits, n)
	}
	if _, n := SectionLineHits(doc, doc.GetSection("Other"), "widget"); n != 0 {
		t.Fatalf("Other should have no hits, got %d", n)
	}
}

func TestSnippetCentersOnMatch(t *testing.T) {
	line := strings.Repeat("a ", 100) + "NEEDLE" + strings.Repeat(" b", 100)
	got := snippet(line, strings.Index(line, "NEEDLE"))
	if !strings.Contains(got, "NEEDLE") || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("snippet = %q", got)
	}
	if n := len([]rune(got)); n > snippetWidth+2 {
		t.Fatalf("snippet is %d runes", n)
	}
}

func TestFindSectionsRanking(t *testing.T) {
	doc := Parse("# Tool and Smudgeplot overview\n\n## Methods\n\n### Smudgeplot\n\nbody\n\n### Smudgeplot extras\n\n## Results\n\n### Setup\n\n## Appendix\n\n### Setup\n")
	// Exact title beats the earlier title that merely contains the word.
	found := doc.FindSections("smudgeplot")
	if len(found) != 3 || found[0].Title != "Smudgeplot" || found[1].Title != "Smudgeplot extras" ||
		found[2].Title != "Tool and Smudgeplot overview" {
		t.Fatalf("ranking = %v", sectionTitles(found))
	}
	if got := doc.GetSection("smudgeplot"); got != found[0] {
		t.Fatalf("GetSection = %q", got.Title)
	}
	// Breadcrumbs pick between identically titled sections.
	if got := doc.FindSections("Appendix > Setup"); len(got) != 1 || got[0].Parent.Title != "Appendix" {
		t.Fatalf("breadcrumb = %v", sectionTitles(got))
	}
	if got := doc.FindSections("Nope > Setup"); len(got) != 0 {
		t.Fatalf("wrong ancestor should not match: %v", sectionTitles(got))
	}
	if got := doc.FindSections("missing"); got != nil {
		t.Fatalf("missing = %v", sectionTitles(got))
	}
	if got := doc.SuggestSections("calibration method", 5); len(got) != 1 || got[0] != "Methods" {
		t.Fatalf("suggestions = %v", got)
	}
	if got := doc.SuggestSections("zzz", 5); len(got) != 0 {
		t.Fatalf("nothing shares a word with zzz: %v", got)
	}
}

func sectionTitles(ss []*Section) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.Title)
	}
	return out
}
