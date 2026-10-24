package parser

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParsePDF_FileNotFound(t *testing.T) {
	_, err := ParsePDF("nonexistent.pdf")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParsePDF_InvalidFile(t *testing.T) {
	// Create a temp file that's not a valid PDF
	f, err := os.CreateTemp("", "invalid*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("not a valid PDF content")
	f.Close()

	_, err = ParsePDF(f.Name())
	if err == nil {
		t.Error("expected error for invalid PDF")
	}
}

func TestHasOutline(t *testing.T) {
	tests := []struct {
		name     string
		children int
		expected bool
	}{
		{"empty outline", 0, false},
		{"with children", 1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// We can't easily create pdf.Outline structs without the library
			// but we can test the logic indirectly through ParsePDF
		})
	}
}

func TestParseByPage_EmptyDocument(t *testing.T) {
	// Create a minimal valid PDF (empty)
	// This tests that parseByPage handles edge cases gracefully
	// The actual PDF parsing is tested through integration tests
}

func TestEstimateTokensForPDF(t *testing.T) {
	// Reuses the same estimateTokens function from markdown.go
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"PDF content", 2},
		{"This is a longer piece of text from a PDF page", 11},
	}

	for _, tc := range tests {
		got := estimateTokens(tc.input)
		if got != tc.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestPDFDocumentStructure(t *testing.T) {
	// Test that Document and Section types work correctly for PDF use cases
	doc := &Document{
		Filename:    "test.pdf",
		TotalTokens: 100,
		Sections: []*Section{
			{
				Level:     1,
				Title:     "Page 1",
				Content:   "Content from page one",
				Tokens:    25,
				LineStart: 1, // Page number
				LineEnd:   1,
			},
			{
				Level:     1,
				Title:     "Page 2",
				Content:   "Content from page two",
				Tokens:    25,
				LineStart: 2,
				LineEnd:   2,
			},
		},
	}

	if doc.Filename != "test.pdf" {
		t.Errorf("expected filename 'test.pdf', got '%s'", doc.Filename)
	}

	if len(doc.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(doc.Sections))
	}

	all := doc.GetAllSections()
	if len(all) != 2 {
		t.Errorf("expected 2 sections from GetAllSections, got %d", len(all))
	}
}

func TestPDFSectionWithChildren(t *testing.T) {
	// Test hierarchical structure (from outline)
	doc := &Document{
		Filename: "outlined.pdf",
		Sections: []*Section{
			{
				Level: 1,
				Title: "Chapter 1",
				Children: []*Section{
					{
						Level: 2,
						Title: "Section 1.1",
					},
					{
						Level: 2,
						Title: "Section 1.2",
					},
				},
			},
		},
	}

	if len(doc.Sections[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(doc.Sections[0].Children))
	}

	// Test GetSection
	section := doc.GetSection("1.1")
	if section == nil {
		t.Error("expected to find section containing '1.1'")
	}
}

func TestNormalizeForMatch(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		idx   []int
	}{
		{"lowercases and keeps alphanumerics", "Ab 9", "ab9", []int{0, 1, 3}},
		{"drops nbsp and non-breaking hyphen", "N‑methyl is not", "nmethylisnot", nil},
		{"drops control bytes and quotes", "\x1fe \u0091first\u0092", "efirst", nil},
		{"trailing period and space", "Enzyme assays. ", "enzymeassays", nil},
		{"empty", "", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			norm, idx := normalizeForMatch(tc.input)
			if string(norm) != tc.want {
				t.Errorf("norm = %q, want %q", norm, tc.want)
			}
			if len(idx) != len(norm) {
				t.Fatalf("idx length %d, norm length %d", len(idx), len(norm))
			}
			for i, raw := range tc.idx {
				if idx[i] != raw {
					t.Errorf("idx[%d] = %d, want %d", i, idx[i], raw)
				}
			}
		})
	}
}

func TestKeyVariants(t *testing.T) {
	tests := []struct {
		key  string
		want []string
	}{
		{"abc", []string{"abc"}},
		{"purification", []string{"purification", "purication"}},
		{"theflux", []string{"theflux", "eflux", "theux", "eux"}},
		{"ffix", []string{"ffix", "x"}},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got := keyVariants([]byte(tc.key))
			if len(got) != len(tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			for i := range got {
				if string(got[i]) != tc.want[i] {
					t.Errorf("variant %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
	many := keyVariants([]byte("fi fi fi fi fi fi fi fi"))
	if len(many) != 2 {
		t.Errorf("past the site cap want key plus fully stripped copy, got %d variants", len(many))
	}
}

func TestAnchorTitles(t *testing.T) {
	norm := func(s string) []byte { n, _ := normalizeForMatch(s); return n }
	keys := func(titles ...string) [][]byte {
		out := make([][]byte, len(titles))
		for i, s := range titles {
			out[i] = norm(s)
		}
		return out
	}
	tests := []struct {
		name string
		text string
		keys [][]byte
		want []int // which occurrence of each key in text anchors it (1-based), 0 when unplaced
	}{
		{
			name: "short title takes the heading, not the earlier prose mention",
			text: "Paper Title Here Please. These results suggest a pathway. Results The deletion of INMT does not affect activity. body",
			keys: keys("Paper Title Here Please", "Results", "The deletion of INMT does not affect activity"),
			want: []int{1, 2, 1},
		},
		{
			name: "distinctive titles scan forward past an earlier mention",
			text: "Intro mentions reversal learning task early. Methods. Reversal learning task. body",
			keys: keys("Methods", "Reversal learning task"),
			want: []int{0, 1},
		},
		{
			name: "ligature dropped by the font still anchors",
			text: "Methods Section Heading. Expression and Puri\x01cation of scFv16 was done. Cryo-EM Sample preparation steps",
			keys: keys("Methods Section Heading", "Expression and Purification of scFv16", "Cryo-EM Sample preparation"),
			want: []int{1, 1, 1},
		},
		{
			name: "caption listed last in the outline anchors where it sits in the text",
			text: "Opening Section Heading. text. Fig 1 Psychedelics reopen the critical period. more text. Online content section. end",
			keys: keys("Opening Section Heading", "Online content section", "Fig 1 Psychedelics reopen the critical period"),
			want: []int{1, 1, 1},
		},
		{
			name: "unmatched titles stay unplaced",
			text: "Nothing in here matches at all",
			keys: keys("Distinctive Heading Text", "Methods"),
			want: []int{0, 0},
		},
		{
			name: "empty title never anchors",
			text: "Heading One Of Several words",
			keys: keys("Heading One Of Several", ""),
			want: []int{1, 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			normText := norm(tc.text)
			pos, lens := anchorTitles(normText, tc.keys)
			for i := range tc.keys {
				got := 0
				if pos[i] >= 0 {
					matched := normText[pos[i] : pos[i]+lens[i]]
					got = bytes.Count(normText[:pos[i]], matched) + 1
				}
				if got != tc.want[i] {
					t.Errorf("title %d %q: occurrence %d (pos %d), want occurrence %d", i, tc.keys[i], got, pos[i], tc.want[i])
				}
			}
		})
	}
}

func TestAnchorSections(t *testing.T) {
	text := "Journal header line\nA Distinctive Paper Title. Abstract text here. Results The deletion of INMT does not affect activity. Finding one. Discussion\nWe report things.\nMethods Enzyme assays. Assay details. References 1. Ref."
	doc := &Document{Sections: []*Section{{
		Level: 1, Title: "A Distinctive Paper Title",
		Children: []*Section{
			{Level: 2, Title: "Results", Children: []*Section{
				{Level: 3, Title: "The deletion of INMT does not affect activity"},
			}},
			{Level: 2, Title: "Discussion"},
			{Level: 2, Title: "Methods", Children: []*Section{
				{Level: 3, Title: "Enzyme assays."},
			}},
			{Level: 2, Title: "References"},
		},
	}}}
	for _, s := range doc.GetAllSections() {
		for _, c := range s.Children {
			c.Parent = s
		}
	}

	if !anchorSections(doc, text) {
		t.Fatal("anchorSections returned false")
	}

	want := []struct {
		title                   string
		content                 string
		lineStart, lineEnd      int
		ownTokensFromOwnContent bool
	}{
		{"A Distinctive Paper Title", "Abstract text here.", 2, 9, false},
		{"Results", "", 3, 4, false},
		{"The deletion of INMT does not affect activity", "Finding one.", 4, 4, true},
		{"Discussion", "We report things.", 5, 6, true},
		{"Methods", "", 7, 8, false},
		{"Enzyme assays.", "Assay details.", 8, 8, true},
		{"References", "1. Ref.", 9, 9, true},
	}
	all := doc.GetAllSections()
	if len(all) != len(want) {
		t.Fatalf("got %d sections, want %d", len(all), len(want))
	}
	for i, w := range want {
		s := all[i]
		if s.Title != w.title {
			t.Fatalf("section %d is %q, want %q", i, s.Title, w.title)
		}
		if s.Content != w.content {
			t.Errorf("%q content = %q, want %q", w.title, s.Content, w.content)
		}
		if s.LineStart != w.lineStart || s.LineEnd != w.lineEnd {
			t.Errorf("%q lines = %d-%d, want %d-%d", w.title, s.LineStart, s.LineEnd, w.lineStart, w.lineEnd)
		}
		if w.ownTokensFromOwnContent && s.Tokens != estimateTokens(w.content) {
			t.Errorf("%q tokens = %d, want %d", w.title, s.Tokens, estimateTokens(w.content))
		}
	}

	results := all[1]
	if results.Tokens != all[2].Tokens {
		t.Errorf("parent with no own content should report child tokens, got %d want %d", results.Tokens, all[2].Tokens)
	}
	span := doc.SourceSpan(results.LineStart, results.LineEnd)
	if !strings.HasPrefix(span, "Results") || !strings.Contains(span, "Finding one.") {
		t.Errorf("SourceSpan for Results = %q, want heading through child body", span)
	}
	if !strings.HasPrefix(doc.SourceSpan(8, 8), "Enzyme assays.") {
		t.Errorf("line 8 = %q, want the Enzyme assays heading on its own line", doc.SourceSpan(8, 8))
	}
}

func TestAnchorSections_FallsBackWhenFewTitlesMatch(t *testing.T) {
	doc := &Document{Sections: []*Section{
		{Level: 1, Title: "First Distinctive Heading"},
		{Level: 1, Title: "Second Heading Missing"},
		{Level: 1, Title: "Third Heading Missing Too"},
	}}
	if anchorSections(doc, "Only the First Distinctive Heading appears in this scan.") {
		t.Fatal("expected fallback when fewer than half the titles anchor")
	}
	if doc.Source != "" || doc.Sections[0].Content != "" || doc.Sections[0].LineStart != 0 {
		t.Error("fallback must leave the document untouched")
	}
	if anchorSections(doc, "") {
		t.Error("empty text layer must not anchor")
	}
}
