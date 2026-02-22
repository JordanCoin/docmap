package parser

import (
	"testing"
)

func TestParseHTMLContentBasic(t *testing.T) {
	html := `<html><body>
		<h1>Getting Started</h1>
		<p>Welcome to the docs.</p>
		<h2>Installation</h2>
		<p>Run npm install.</p>
		<h2>Configuration</h2>
		<p>Edit the config file.</p>
	</body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 root section, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Title != "Getting Started" {
		t.Errorf("expected 'Getting Started', got '%s'", doc.Sections[0].Title)
	}
	if len(doc.Sections[0].Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(doc.Sections[0].Children))
	}
	if doc.Sections[0].Children[0].Title != "Installation" {
		t.Errorf("expected 'Installation', got '%s'", doc.Sections[0].Children[0].Title)
	}
	if doc.Sections[0].Children[1].Title != "Configuration" {
		t.Errorf("expected 'Configuration', got '%s'", doc.Sections[0].Children[1].Title)
	}
}

func TestParseHTMLContentSkipsNav(t *testing.T) {
	html := `<html><body>
		<nav><h5>Sidebar Title</h5><a href="/">Home</a></nav>
		<h1>Main Content</h1>
		<p>Real content here.</p>
	</body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section (nav skipped), got %d", len(doc.Sections))
	}
	if doc.Sections[0].Title != "Main Content" {
		t.Errorf("expected 'Main Content', got '%s'", doc.Sections[0].Title)
	}
}

func TestParseHTMLContentSkipsSidebar(t *testing.T) {
	html := `<html><body>
		<div class="sidebar"><h5>Nav Section</h5></div>
		<h1>Page Title</h1>
		<p>Body text.</p>
		<footer><h6>Footer Heading</h6></footer>
	</body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Title != "Page Title" {
		t.Errorf("expected 'Page Title', got '%s'", doc.Sections[0].Title)
	}
}

func TestParseHTMLContentNoHeadings(t *testing.T) {
	html := `<html><body><p>Just a paragraph.</p><p>No headings here.</p></body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc != nil {
		t.Errorf("expected nil for no headings, got %+v", doc)
	}
}

func TestParseHTMLContentEmpty(t *testing.T) {
	doc, err := parseHTMLContent("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc != nil {
		t.Errorf("expected nil for empty HTML, got %+v", doc)
	}
}

func TestParseHTMLContentThreeLevels(t *testing.T) {
	html := `<html><body>
		<h1>Chapter 1</h1>
		<p>Chapter intro.</p>
		<h2>Section 1.1</h2>
		<p>Section content.</p>
		<h3>Subsection 1.1.1</h3>
		<p>Detail content.</p>
	</body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 root, got %d", len(doc.Sections))
	}
	root := doc.Sections[0]
	if root.Title != "Chapter 1" {
		t.Errorf("expected 'Chapter 1', got '%s'", root.Title)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child of root, got %d", len(root.Children))
	}
	child := root.Children[0]
	if child.Title != "Section 1.1" {
		t.Errorf("expected 'Section 1.1', got '%s'", child.Title)
	}
	if len(child.Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(child.Children))
	}
	if child.Children[0].Title != "Subsection 1.1.1" {
		t.Errorf("expected 'Subsection 1.1.1', got '%s'", child.Children[0].Title)
	}
}

func TestParseHTMLContentExtractsBodyText(t *testing.T) {
	html := `<html><body>
		<h1>Title</h1>
		<p>First paragraph.</p>
		<p>Second paragraph.</p>
	</body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	content := doc.Sections[0].Content
	if content == "" {
		t.Error("expected non-empty content")
	}
	if doc.Sections[0].Tokens == 0 {
		t.Error("expected non-zero tokens")
	}
}

func TestParseHTMLContentPrefersMainElement(t *testing.T) {
	html := `<html><body>
		<header><h1>Site Header</h1></header>
		<main>
			<h1>Page Title</h1>
			<p>Content.</p>
		</main>
	</body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(doc.Sections))
	}
	// Should pick up "Page Title" from <main>, not "Site Header" from <header>
	if doc.Sections[0].Title != "Page Title" {
		t.Errorf("expected 'Page Title', got '%s'", doc.Sections[0].Title)
	}
}

func TestParseHTMLContentSkipsScriptStyle(t *testing.T) {
	html := `<html><body>
		<script>var x = "heading h1";</script>
		<style>.h1 { color: red; }</style>
		<h1>Real Title</h1>
		<p>Content here.</p>
	</body></html>`

	doc, err := parseHTMLContent(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Title != "Real Title" {
		t.Errorf("expected 'Real Title', got '%s'", doc.Sections[0].Title)
	}
}
