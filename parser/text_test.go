package parser

import (
	"strings"
	"testing"
)

func titles(doc *Document) []string {
	var out []string
	for _, s := range doc.GetAllSections() {
		out = append(out, s.Title)
	}
	return out
}

func TestParseText_MarkdownHeadingsStayMarkdown(t *testing.T) {
	doc := ParseText("# Plain text doc\n\nA text file with a heading.\n")
	if got := titles(doc); len(got) != 1 || got[0] != "Plain text doc" {
		t.Fatalf("titles = %v", got)
	}
}

func TestParseText_InfersHeadings(t *testing.T) {
	src := "Title Of Thing\n==============\n\nintro text\n\n1 Introduction\n\nbody one\n\n" +
		"1.1 Background\n\nbody bg\n\nRESULTS AND DISCUSSION\n\nfinal body.\n"
	doc := ParseText(src)
	want := []string{"Title Of Thing", "1 Introduction", "1.1 Background", "RESULTS AND DISCUSSION"}
	if got := titles(doc); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("titles = %v, want %v", got, want)
	}
	bg := doc.GetSection("1.1 Background")
	if bg == nil || bg.Level != 2 || bg.Parent == nil || bg.Parent.Title != "1 Introduction" {
		t.Fatalf("1.1 should nest under 1: %+v", bg)
	}
	if bg.Content != "body bg" || bg.LineStart != 10 || bg.LineEnd != 12 {
		t.Fatalf("background = %q lines %d-%d", bg.Content, bg.LineStart, bg.LineEnd)
	}
	if intro := doc.GetSection("Title Of Thing"); intro.Content != "intro text" {
		t.Fatalf("underline must not leak into content: %q", intro.Content)
	}
	if doc.TotalTokens == 0 {
		t.Fatal("tokens should be counted")
	}
}

func TestParseText_PreambleKeepsLeadingText(t *testing.T) {
	doc := ParseText("some opening words\n\nSECTION ONE\n\nbody\n")
	got := titles(doc)
	if len(got) != 2 || got[0] != "(preamble)" || doc.Sections[0].Content != "some opening words" {
		t.Fatalf("titles = %v", got)
	}
}

func TestParseText_FormFeedPages(t *testing.T) {
	// pdftotext layout: a form feed opens each following page and ends the file.
	doc := ParseText("page one words\nmore\n\fpage two words\n\f")
	if got := titles(doc); strings.Join(got, "|") != "Page 1|Page 2" {
		t.Fatalf("titles = %v", got)
	}
	p2 := doc.Sections[1]
	if p2.Content != "page two words" || p2.LineStart != 3 {
		t.Fatalf("page 2 = %q at line %d", p2.Content, p2.LineStart)
	}
}

func TestParseText_NoStructureIsOneSearchableSection(t *testing.T) {
	doc := ParseText("\njust a note about quantification.\nSecond line, with a comma,\nand a third.\n")
	if len(doc.Sections) != 1 || doc.Sections[0].Title != "(full text)" {
		t.Fatalf("titles = %v", titles(doc))
	}
	s := doc.Sections[0]
	if !strings.Contains(s.Content, "quantification") || s.LineStart != 2 || s.LineEnd != 4 {
		t.Fatalf("section = %q lines %d-%d", s.Content, s.LineStart, s.LineEnd)
	}
	if got := doc.SourceSpan(s.LineStart, s.LineEnd); !strings.HasPrefix(got, "just a note") {
		t.Fatalf("SourceSpan = %q", got)
	}
}

func TestParseText_ProseIsNotAHeading(t *testing.T) {
	// Indented text must not become a code block, and short sentences or
	// shouted lines inside a paragraph must not become headings.
	doc := ParseText("    indented line\nWARNING THIS IS INLINE\nmore prose.\n\n3 apples were eaten.\n\nEnd.\n")
	if got := titles(doc); len(got) != 1 || got[0] != "(full text)" {
		t.Fatalf("titles = %v", got)
	}
	if len(doc.Nodes) != 0 {
		t.Fatalf("plain text should have no markdown nodes, got %d", len(doc.Nodes))
	}
}

func TestParseText_Empty(t *testing.T) {
	if doc := ParseText("  \n\n"); len(doc.Sections) != 0 || doc.TotalTokens != 0 {
		t.Fatalf("empty text: %+v", doc)
	}
}
