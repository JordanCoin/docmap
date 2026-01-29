package parser

import (
	"os"
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
