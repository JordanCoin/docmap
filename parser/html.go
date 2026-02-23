package parser

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// htmlSection is an intermediate representation of a section extracted from HTML.
type htmlSection struct {
	level   int
	title   string
	content strings.Builder
}

// parseHTMLFromURL fetches a URL and tries to extract sections from semantic HTML headings.
// Returns nil, nil if the HTML has no usable heading structure (e.g. JS-only SPA).
func parseHTMLFromURL(url string) (*Document, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseHTMLContent(string(body))
}

// parseHTMLContent extracts document sections from HTML content using semantic heading tags.
// Returns nil, nil if no usable heading structure is found.
func parseHTMLContent(htmlContent string) (*Document, error) {
	node, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Find the best content root — prefer <main>, <article>, or element with role="main"
	contentRoot := findContentRoot(node)
	if contentRoot == nil {
		contentRoot = findBodyElement(node)
	}
	if contentRoot == nil {
		return nil, nil
	}

	// Walk the DOM and extract headings + body text in document order
	var sections []htmlSection
	var current *htmlSection

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		// Skip nav, header, footer, sidebar elements
		if shouldSkipElement(n) {
			return
		}

		if n.Type == html.ElementNode {
			level := headingLevel(n)
			if level > 0 {
				title := extractText(n)
				title = strings.TrimSpace(title)
				if title == "" {
					// Skip empty headings
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						walk(c)
					}
					return
				}

				// Finalize previous section
				if current != nil {
					sections = append(sections, *current)
				}

				current = &htmlSection{level: level, title: title}
				return // Don't recurse into heading children (already extracted text)
			}
		}

		if n.Type == html.TextNode && current != nil {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				current.content.WriteString(text)
				current.content.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(contentRoot)

	// Finalize last section
	if current != nil {
		sections = append(sections, *current)
	}

	// No headings found — HTML is probably a JS SPA shell
	if len(sections) == 0 {
		return nil, nil
	}

	// Convert to Section structs
	var allSections []*Section
	for _, hs := range sections {
		content := strings.TrimSpace(hs.content.String())
		s := &Section{
			Level:   hs.level,
			Title:   hs.title,
			Content: content,
			Tokens:  estimateTokens(content),
		}
		allSections = append(allSections, s)
	}

	doc := &Document{
		Sections: buildTree(allSections),
	}
	for _, s := range allSections {
		doc.TotalTokens += s.Tokens
	}

	return doc, nil
}

// headingLevel returns the heading level (1-6) for h1-h6 elements, or 0 for non-headings.
func headingLevel(n *html.Node) int {
	if n.Type != html.ElementNode {
		return 0
	}
	switch n.Data {
	case "h1":
		return 1
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	case "h5":
		return 5
	case "h6":
		return 6
	}
	return 0
}

// shouldSkipElement returns true for elements that typically contain navigation/chrome, not content.
func shouldSkipElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}

	// Skip nav, header, footer elements
	switch n.Data {
	case "nav", "footer", "noscript", "script", "style", "svg", "iframe":
		return true
	}

	for _, attr := range n.Attr {
		val := strings.ToLower(attr.Val)

		// Skip elements with sidebar/nav roles or IDs
		if attr.Key == "role" && (val == "navigation" || val == "banner" || val == "contentinfo") {
			return true
		}
		if attr.Key == "id" && (val == "sidebar-title" || strings.Contains(val, "sidebar") || strings.Contains(val, "nav")) {
			return true
		}
		// Skip elements with common nav/sidebar classes
		if attr.Key == "class" {
			if strings.Contains(val, "sidebar") || strings.Contains(val, "nav-") ||
				strings.Contains(val, "navigation") || strings.Contains(val, "toc") {
				return true
			}
		}
		// Skip hidden elements
		if attr.Key == "hidden" || (attr.Key == "aria-hidden" && val == "true") {
			return true
		}
	}

	return false
}

// findContentRoot looks for a <main>, <article>, or element with role="main".
func findContentRoot(n *html.Node) *html.Node {
	if n.Type == html.ElementNode {
		if n.Data == "main" || n.Data == "article" {
			return n
		}
		for _, attr := range n.Attr {
			if attr.Key == "role" && attr.Val == "main" {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findContentRoot(c); found != nil {
			return found
		}
	}
	return nil
}

// findBodyElement returns the <body> element, or nil.
func findBodyElement(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findBodyElement(c); found != nil {
			return found
		}
	}
	return nil
}

// extractText recursively extracts all text content from a node.
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(extractText(c))
	}
	return b.String()
}
