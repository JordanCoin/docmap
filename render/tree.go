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

// RefsTree renders document references (links to other .md files)
func RefsTree(docs []*parser.Document, dirName string) {
	// Build reference graph
	type RefInfo struct {
		From string
		To   string
		Text string
		Line int
	}

	var allRefs []RefInfo
	fileRefs := make(map[string][]string)  // file -> files it references
	fileRefBy := make(map[string][]string) // file -> files that reference it

	for _, doc := range docs {
		for _, ref := range doc.References {
			allRefs = append(allRefs, RefInfo{
				From: doc.Filename,
				To:   ref.Target,
				Text: ref.Text,
				Line: ref.Line,
			})
			fileRefs[doc.Filename] = append(fileRefs[doc.Filename], ref.Target)
			fileRefBy[ref.Target] = append(fileRefBy[ref.Target], doc.Filename)
		}
	}

	if len(allRefs) == 0 {
		fmt.Println("No markdown cross-references found")
		return
	}

	// Header
	innerWidth := 60
	titleLine := fmt.Sprintf(" %s/ ", dirName)
	padding := innerWidth - len(titleLine)
	leftPad := padding / 2
	rightPad := padding - leftPad
	fmt.Printf("╭%s%s%s╮\n", strings.Repeat("─", leftPad), titleLine, strings.Repeat("─", rightPad))

	info := fmt.Sprintf("References: %d links between docs", len(allRefs))
	fmt.Printf("│ %-*s │\n", innerWidth-2, centerText(info, innerWidth-2))
	fmt.Printf("╰%s╯\n", strings.Repeat("─", innerWidth))
	fmt.Println()

	// Find hubs (most referenced files)
	type hub struct {
		file  string
		count int
	}
	var hubs []hub
	for file, refs := range fileRefBy {
		if len(refs) >= 2 {
			hubs = append(hubs, hub{file, len(refs)})
		}
	}

	if len(hubs) > 0 {
		// Sort by count descending
		for i := 0; i < len(hubs); i++ {
			for j := i + 1; j < len(hubs); j++ {
				if hubs[j].count > hubs[i].count {
					hubs[i], hubs[j] = hubs[j], hubs[i]
				}
			}
		}

		fmt.Printf("%sHUBS:%s ", bold, reset)
		var hubStrs []string
		for _, h := range hubs {
			if len(hubStrs) >= 5 {
				break
			}
			hubStrs = append(hubStrs, fmt.Sprintf("%s%s%s (%d←)", green, h.file, reset, h.count))
		}
		fmt.Println(strings.Join(hubStrs, ", "))
		fmt.Println()
	}

	// Show reference flow by file
	fmt.Printf("%sReference Flow:%s\n", bold+cyan, reset)
	fmt.Println()

	// Group by source file
	printed := make(map[string]bool)
	for _, doc := range docs {
		if len(doc.References) == 0 {
			continue
		}
		if printed[doc.Filename] {
			continue
		}
		printed[doc.Filename] = true

		// Dedupe targets
		seen := make(map[string]bool)
		var targets []string
		for _, ref := range doc.References {
			if !seen[ref.Target] {
				targets = append(targets, ref.Target)
				seen[ref.Target] = true
			}
		}

		fmt.Printf("  %s%s%s\n", bold, doc.Filename, reset)
		for i, target := range targets {
			connector := "├──▶ "
			if i == len(targets)-1 {
				connector = "└──▶ "
			}
			fmt.Printf("  %s%s%s%s\n", dim, connector, reset, target)
		}
		fmt.Println()
	}
}
