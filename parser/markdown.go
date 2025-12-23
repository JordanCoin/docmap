package parser

import (
	"regexp"
	"strings"
)

// Document represents a parsed markdown document
type Document struct {
	Filename    string
	TotalTokens int
	Sections    []*Section
	References  []Reference // Links to other .md files
}

// Reference represents a link to another markdown file
type Reference struct {
	Text   string // Link text
	Target string // Target file path
	Line   int    // Line number where reference appears
}

// Section represents a heading and its content
type Section struct {
	Level       int       // 1 = #, 2 = ##, etc.
	Title       string
	Content     string    // raw content (excluding children)
	Tokens      int       // estimated tokens for this section
	KeyTerms    []string  // extracted key concepts
	Children    []*Section
	Parent      *Section
	LineStart   int
	LineEnd     int
}

// Token estimation: ~4 chars per token (rough approximation)
func estimateTokens(s string) int {
	return len(s) / 4
}

// Parse parses markdown content into a Document structure
func Parse(content string) *Document {
	lines := strings.Split(content, "\n")
	doc := &Document{}

	headingRe := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	// Match markdown links to .md files: [text](path.md) or [text](path.md#anchor)
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+\.md(?:#[^)]*)?)\)`)

	var allSections []*Section
	var currentSection *Section
	var contentBuilder strings.Builder

	for i, line := range lines {
		// Extract markdown links to .md files
		for _, match := range linkRe.FindAllStringSubmatch(line, -1) {
			target := match[2]
			// Remove anchor if present
			if idx := strings.Index(target, "#"); idx != -1 {
				target = target[:idx]
			}
			doc.References = append(doc.References, Reference{
				Text:   match[1],
				Target: target,
				Line:   i + 1,
			})
		}

		if matches := headingRe.FindStringSubmatch(line); matches != nil {
			// Save previous section's content
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(contentBuilder.String())
				currentSection.Tokens = estimateTokens(currentSection.Content)
				currentSection.KeyTerms = extractKeyTerms(currentSection.Content)
				currentSection.LineEnd = i - 1
			}

			level := len(matches[1])
			title := strings.TrimSpace(matches[2])

			section := &Section{
				Level:     level,
				Title:     title,
				LineStart: i + 1,
			}

			allSections = append(allSections, section)
			currentSection = section
			contentBuilder.Reset()
		} else if currentSection != nil {
			contentBuilder.WriteString(line)
			contentBuilder.WriteString("\n")
		}
	}

	// Finalize last section
	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(contentBuilder.String())
		currentSection.Tokens = estimateTokens(currentSection.Content)
		currentSection.KeyTerms = extractKeyTerms(currentSection.Content)
		currentSection.LineEnd = len(lines)
	}

	// Build tree structure
	doc.Sections = buildTree(allSections)

	// Calculate total tokens
	for _, s := range allSections {
		doc.TotalTokens += s.Tokens
	}

	return doc
}

// buildTree organizes flat sections into a tree based on heading levels
func buildTree(sections []*Section) []*Section {
	if len(sections) == 0 {
		return nil
	}

	var roots []*Section
	var stack []*Section

	for _, section := range sections {
		// Pop stack until we find a parent with lower level
		for len(stack) > 0 && stack[len(stack)-1].Level >= section.Level {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			roots = append(roots, section)
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, section)
			section.Parent = parent
		}

		stack = append(stack, section)
	}

	// Calculate cumulative tokens (including children)
	for _, root := range roots {
		calculateCumulativeTokens(root)
	}

	return roots
}

func calculateCumulativeTokens(s *Section) int {
	total := s.Tokens
	for _, child := range s.Children {
		total += calculateCumulativeTokens(child)
	}
	s.Tokens = total
	return total
}

// extractKeyTerms pulls out bold text, code, and quoted terms
func extractKeyTerms(content string) []string {
	var terms []string
	seen := make(map[string]bool)

	// Bold text: **term** or __term__
	boldRe := regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	for _, match := range boldRe.FindAllStringSubmatch(content, -1) {
		term := match[1]
		if term == "" {
			term = match[2]
		}
		term = strings.TrimSpace(term)
		if term != "" && !seen[term] && len(term) < 50 {
			terms = append(terms, term)
			seen[term] = true
		}
	}

	// Inline code: `term`
	codeRe := regexp.MustCompile("`([^`]+)`")
	for _, match := range codeRe.FindAllStringSubmatch(content, 10) {
		term := strings.TrimSpace(match[1])
		if term != "" && !seen[term] && len(term) < 40 {
			terms = append(terms, term)
			seen[term] = true
		}
	}

	// Limit terms
	if len(terms) > 5 {
		terms = terms[:5]
	}

	return terms
}

// GetSection finds a section by name (case-insensitive partial match)
func (d *Document) GetSection(name string) *Section {
	name = strings.ToLower(name)
	return findSection(d.Sections, name)
}

func findSection(sections []*Section, name string) *Section {
	for _, s := range sections {
		if strings.Contains(strings.ToLower(s.Title), name) {
			return s
		}
		if found := findSection(s.Children, name); found != nil {
			return found
		}
	}
	return nil
}

// GetAllSections returns a flat list of all sections
func (d *Document) GetAllSections() []*Section {
	var all []*Section
	var collect func([]*Section)
	collect = func(sections []*Section) {
		for _, s := range sections {
			all = append(all, s)
			collect(s.Children)
		}
	}
	collect(d.Sections)
	return all
}
