package parser

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ParsePDF parses a PDF file into a Document structure
func ParsePDF(filepath string) (*Document, error) {
	f, r, err := pdf.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	doc := &Document{}

	// Try to extract outline (bookmarks) first
	outline := r.Outline()
	if hasOutline(outline) {
		doc.Sections = parseOutline(outline)
		// Add page content as token estimates
		addPageTokens(r, doc)
	} else {
		// Fall back to page-based structure
		doc.Sections = parseByPage(r)
	}

	// Calculate total tokens
	for _, s := range doc.GetAllSections() {
		doc.TotalTokens += s.Tokens
	}

	return doc, nil
}

// hasOutline checks if the PDF has a meaningful outline structure
func hasOutline(outline pdf.Outline) bool {
	return len(outline.Child) > 0
}

// parseOutline extracts document structure from PDF bookmarks
func parseOutline(outline pdf.Outline) []*Section {
	var sections []*Section

	for _, item := range outline.Child {
		section := outlineItemToSection(item, 1)
		sections = append(sections, section)
	}

	return sections
}

// outlineItemToSection converts a PDF outline item to a Section
func outlineItemToSection(item pdf.Outline, level int) *Section {
	section := &Section{
		Level: level,
		Title: strings.TrimSpace(item.Title),
	}

	// Process children recursively
	for _, child := range item.Child {
		childSection := outlineItemToSection(child, level+1)
		childSection.Parent = section
		section.Children = append(section.Children, childSection)
	}

	return section
}

// addPageTokens adds token estimates to outlined documents by reading all pages
func addPageTokens(r *pdf.Reader, doc *Document) {
	numPages := r.NumPage()
	totalText := strings.Builder{}

	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err == nil {
			totalText.WriteString(text)
		}
	}

	// Distribute tokens across top-level sections proportionally
	allText := totalText.String()
	totalTokens := estimateTokens(allText)

	if len(doc.Sections) > 0 && totalTokens > 0 {
		tokensPerSection := totalTokens / len(doc.Sections)
		for _, section := range doc.Sections {
			distributeTokens(section, tokensPerSection)
		}
	}
}

// distributeTokens assigns token estimates to a section and its children
func distributeTokens(section *Section, tokens int) {
	if len(section.Children) == 0 {
		section.Tokens = tokens
		return
	}

	// Give parent a portion, distribute rest to children
	childCount := len(section.Children)
	perChild := tokens / (childCount + 1)
	section.Tokens = perChild

	for _, child := range section.Children {
		distributeTokens(child, perChild)
	}

	// Recalculate cumulative tokens
	calculateCumulativeTokens(section)
}

// parseByPage creates a page-based structure for PDFs without outlines
func parseByPage(r *pdf.Reader) []*Section {
	numPages := r.NumPage()
	if numPages == 0 {
		return nil
	}

	var sections []*Section
	var hasContent bool

	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		hasContent = true
		section := &Section{
			Level:     1,
			Title:     fmt.Sprintf("Page %d", i),
			Content:   text,
			Tokens:    estimateTokens(text),
			LineStart: i, // Page number
			LineEnd:   i,
		}
		sections = append(sections, section)
	}

	// If no text content was extracted, return a warning section
	if !hasContent && numPages > 0 {
		return []*Section{{
			Level:     1,
			Title:     fmt.Sprintf("(%d pages - no extractable text)", numPages),
			Content:   "",
			Tokens:    0,
			LineStart: 1,
			LineEnd:   numPages,
		}}
	}

	return sections
}
