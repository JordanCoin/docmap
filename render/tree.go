package render

import (
	"fmt"
	"strings"

	"github.com/JordanCoin/docmap/parser"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	cyan   = "\033[36m"
	yellow = "\033[33m"
	green  = "\033[32m"
	blue   = "\033[34m"
)

// Tree renders the full document map
func Tree(doc *parser.Document) {
	// Header box
	printHeader(doc)

	// Render sections
	for i, section := range doc.Sections {
		isLast := i == len(doc.Sections)-1
		renderSection(section, "", isLast, false)
	}

	fmt.Println()
}

func printHeader(doc *parser.Document) {
	// Count sections
	sectionCount := len(doc.GetAllSections())
	info := fmt.Sprintf("Sections: %d | ~%s tokens", sectionCount, formatTokens(doc.TotalTokens))

	// Calculate width based on content
	innerWidth := 60
	if len(info)+4 > innerWidth {
		innerWidth = len(info) + 4
	}

	// Title in top border (like codemap)
	titleLine := fmt.Sprintf(" %s ", doc.Filename)
	padding := innerWidth - len(titleLine)
	leftPad := padding / 2
	rightPad := padding - leftPad
	fmt.Printf("╭%s%s%s╮\n", strings.Repeat("─", leftPad), titleLine, strings.Repeat("─", rightPad))

	// Info line
	fmt.Printf("│ %-*s │\n", innerWidth-2, centerText(info, innerWidth-2))

	// Bottom border
	fmt.Printf("╰%s╯\n", strings.Repeat("─", innerWidth))
	fmt.Println()
}

func centerText(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	padding := (width - len(s)) / 2
	return strings.Repeat(" ", padding) + s
}

func formatTokens(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

func renderSection(s *parser.Section, prefix string, isLast bool, isFiltered bool) {
	// Choose connector
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	// Token count
	tokenStr := dim + fmt.Sprintf("(%s)", formatTokens(s.Tokens)) + reset

	// Title with color based on level
	var titleColor string
	switch s.Level {
	case 1:
		titleColor = bold + cyan
	case 2:
		titleColor = bold + blue
	case 3:
		titleColor = yellow
	default:
		titleColor = ""
	}

	// Print section
	fmt.Printf("%s%s%s%s%s %s\n", prefix, dim, connector, reset, titleColor+s.Title+reset, tokenStr)

	// Key terms on next line if present
	if len(s.KeyTerms) > 0 && (s.Level <= 2 || isFiltered) {
		termPrefix := prefix
		if isLast {
			termPrefix += "    "
		} else {
			termPrefix += "│   "
		}
		terms := strings.Join(s.KeyTerms, ", ")
		if len(terms) > 55 {
			terms = terms[:52] + "..."
		}
		fmt.Printf("%s%s└─ %s%s\n", termPrefix, dim, terms, reset)
	}

	// Render children
	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	for i, child := range s.Children {
		childIsLast := i == len(s.Children)-1
		renderSection(child, childPrefix, childIsLast, isFiltered)
	}
}

// FilteredTree shows only sections matching the filter
func FilteredTree(doc *parser.Document, filter string) {
	section := doc.GetSection(filter)
	if section == nil {
		fmt.Printf("Section '%s' not found\n", filter)
		return
	}

	// Print mini header
	fmt.Printf("%s╭── %s%s%s (%s tokens)%s\n", dim, reset, bold+cyan, section.Title, formatTokens(section.Tokens), reset)

	// Print children
	for i, child := range section.Children {
		isLast := i == len(section.Children)-1
		renderSection(child, "", isLast, true)
	}

	fmt.Println()
}

// ExpandSection shows full content of a section
func ExpandSection(doc *parser.Document, name string) {
	section := doc.GetSection(name)
	if section == nil {
		fmt.Printf("Section '%s' not found\n", name)
		return
	}

	fmt.Printf("%s%s%s\n", bold+cyan, section.Title, reset)
	fmt.Println(dim + strings.Repeat("─", 50) + reset)
	fmt.Println()

	// Print content (limited)
	content := section.Content
	lines := strings.Split(content, "\n")
	maxLines := 50
	if len(lines) > maxLines {
		for _, line := range lines[:maxLines] {
			fmt.Println(line)
		}
		fmt.Printf("\n%s... (%d more lines)%s\n", dim, len(lines)-maxLines, reset)
	} else {
		fmt.Println(content)
	}
}

// MultiTree renders multiple documents as a combined tree
func MultiTree(docs []*parser.Document, dirName string) {
	// Calculate totals
	totalTokens := 0
	totalSections := 0
	for _, doc := range docs {
		totalTokens += doc.TotalTokens
		totalSections += len(doc.GetAllSections())
	}

	// Header
	info := fmt.Sprintf("%d files | %d sections | ~%s tokens", len(docs), totalSections, formatTokens(totalTokens))
	innerWidth := 60
	if len(info)+4 > innerWidth {
		innerWidth = len(info) + 4
	}

	// Title in top border (like codemap)
	titleLine := fmt.Sprintf(" %s/ ", dirName)
	padding := innerWidth - len(titleLine)
	leftPad := padding / 2
	rightPad := padding - leftPad
	fmt.Printf("╭%s%s%s╮\n", strings.Repeat("─", leftPad), titleLine, strings.Repeat("─", rightPad))

	// Info line
	fmt.Printf("│ %-*s │\n", innerWidth-2, centerText(info, innerWidth-2))

	// Bottom border
	fmt.Printf("╰%s╯\n", strings.Repeat("─", innerWidth))
	fmt.Println()

	// Render each document
	for i, doc := range docs {
		isLast := i == len(docs)-1
		renderDocSummary(doc, isLast)
	}

	fmt.Println()
}

func renderDocSummary(doc *parser.Document, isLast bool) {
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	tokenStr := dim + fmt.Sprintf("(%s)", formatTokens(doc.TotalTokens)) + reset

	// Filename in bold cyan
	fmt.Printf("%s%s%s%s %s\n", dim, connector, reset, bold+green+doc.Filename+reset, tokenStr)

	// Show top-level sections (h1/h2 only)
	childPrefix := "    "
	if !isLast {
		childPrefix = "│   "
	}

	topSections := getTopSections(doc.Sections, 2) // max depth 2
	for j, section := range topSections {
		sectionIsLast := j == len(topSections)-1
		sectionConnector := "├─ "
		if sectionIsLast {
			sectionConnector = "└─ "
		}

		// Truncate long titles
		title := section.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		fmt.Printf("%s%s%s%s\n", childPrefix, dim, sectionConnector, title+reset)
	}
}

func getTopSections(sections []*parser.Section, maxDepth int) []*parser.Section {
	var result []*parser.Section
	for _, s := range sections {
		if s.Level <= maxDepth {
			result = append(result, s)
		}
	}
	// Limit to first 5
	if len(result) > 5 {
		result = result[:5]
	}
	return result
}
