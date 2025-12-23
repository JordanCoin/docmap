package parser

import (
	"testing"
)

func TestParse(t *testing.T) {
	content := `# Title

Some intro text.

## Section One

Content for section one with **bold term** and ` + "`code`" + `.

### Subsection

More content here.

## Section Two

Another section with a [link](other.md).
`

	doc := Parse(content)

	if len(doc.Sections) != 1 {
		t.Errorf("expected 1 root section, got %d", len(doc.Sections))
	}

	if doc.Sections[0].Title != "Title" {
		t.Errorf("expected title 'Title', got '%s'", doc.Sections[0].Title)
	}

	if len(doc.Sections[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(doc.Sections[0].Children))
	}

	if doc.TotalTokens == 0 {
		t.Error("expected non-zero token count")
	}

	if len(doc.References) != 1 {
		t.Errorf("expected 1 reference, got %d", len(doc.References))
	}

	if doc.References[0].Target != "other.md" {
		t.Errorf("expected reference to 'other.md', got '%s'", doc.References[0].Target)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"test", 1},
		{"hello world", 2},
		{"this is a longer string with more tokens", 10},
	}

	for _, tc := range tests {
		got := estimateTokens(tc.input)
		if got != tc.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestExtractKeyTerms(t *testing.T) {
	content := `This has **bold text** and ` + "`inline code`" + ` and __underline bold__.`

	terms := extractKeyTerms(content)

	if len(terms) < 2 {
		t.Errorf("expected at least 2 key terms, got %d", len(terms))
	}

	// Check that bold and code are extracted
	foundBold := false
	foundCode := false
	for _, term := range terms {
		if term == "bold text" {
			foundBold = true
		}
		if term == "inline code" {
			foundCode = true
		}
	}

	if !foundBold {
		t.Error("expected to find 'bold text' in key terms")
	}
	if !foundCode {
		t.Error("expected to find 'inline code' in key terms")
	}
}

func TestGetSection(t *testing.T) {
	content := `# Main

## Installation

Install instructions.

## Usage

Usage info.
`

	doc := Parse(content)

	section := doc.GetSection("install")
	if section == nil {
		t.Error("expected to find 'install' section")
	}
	if section != nil && section.Title != "Installation" {
		t.Errorf("expected title 'Installation', got '%s'", section.Title)
	}

	notFound := doc.GetSection("nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent section")
	}
}

func TestGetAllSections(t *testing.T) {
	content := `# One

## Two

### Three

## Four
`

	doc := Parse(content)
	all := doc.GetAllSections()

	if len(all) != 4 {
		t.Errorf("expected 4 sections, got %d", len(all))
	}
}

func TestBuildTree(t *testing.T) {
	content := `# Root

## Child1

### Grandchild

## Child2
`

	doc := Parse(content)

	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 root, got %d", len(doc.Sections))
	}

	root := doc.Sections[0]
	if len(root.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(root.Children))
	}

	if len(root.Children[0].Children) != 1 {
		t.Errorf("expected 1 grandchild, got %d", len(root.Children[0].Children))
	}
}

func TestReferences(t *testing.T) {
	content := `# Docs

See [architecture](docs/ARCHITECTURE.md) and [api](api.md#section).
Also check [external](https://example.com).
`

	doc := Parse(content)

	// Should only find .md references, not external links
	if len(doc.References) != 2 {
		t.Errorf("expected 2 references, got %d", len(doc.References))
	}

	// Check that anchor is stripped from target
	for _, ref := range doc.References {
		if ref.Target == "api.md" {
			return // Found the stripped version
		}
	}
	t.Error("expected api.md reference with anchor stripped")
}
