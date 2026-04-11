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
	// Main info line.
	sectionCount := len(doc.GetAllSections())
	mainInfo := fmt.Sprintf("Sections: %d │ ~%s tokens", sectionCount, formatTokens(doc.TotalTokens))

	// Build content summary lines from the typed AST.
	summaryLines := buildSummaryLines(doc.Summary())

	// Join all info lines for width calculation.
	allLines := append([]string{mainInfo}, summaryLines...)

	// Truncate long filenames for display.
	displayName := doc.Filename
	maxNameLen := 50
	if len(displayName) > maxNameLen {
		displayName = "..." + displayName[len(displayName)-maxNameLen+3:]
	}

	// Inner width = max of title, main info, and every summary line.
	innerWidth := 60
	titleLine := fmt.Sprintf(" %s ", displayName)
	if len(titleLine) > innerWidth {
		innerWidth = len(titleLine) + 4
	}
	for _, line := range allLines {
		if len(line)+4 > innerWidth {
			innerWidth = len(line) + 4
		}
	}

	// Top border with centered title.
	padding := innerWidth - len(titleLine)
	leftPad := padding / 2
	rightPad := padding - leftPad
	fmt.Printf("╭%s%s%s╮\n", strings.Repeat("─", leftPad), titleLine, strings.Repeat("─", rightPad))

	// Info lines: main line, then one line per summary group.
	for _, line := range allLines {
		fmt.Printf("│ %-*s │\n", innerWidth-2, centerText(line, innerWidth-2))
	}

	// Bottom border.
	fmt.Printf("╰%s╯\n", strings.Repeat("─", innerWidth))
	fmt.Println()
}

// buildSummaryLines turns a ContentSummary into at most three display rows:
// block constructs, tasks/Obsidian, and GFM-extras references.
// Each row is a " · "-joined list of populated counters so zero-count items
// never appear.
func buildSummaryLines(s parser.ContentSummary) []string {
	var lines []string

	var blocks []string
	if s.Callouts > 0 {
		blocks = append(blocks, fmt.Sprintf("%d callout%s", s.Callouts, pluralS(s.Callouts)))
	}
	if s.Tables > 0 {
		blocks = append(blocks, fmt.Sprintf("%d table%s", s.Tables, pluralS(s.Tables)))
	}
	if s.CodeBlocks > 0 {
		blocks = append(blocks, fmt.Sprintf("%d code block%s", s.CodeBlocks, pluralS(s.CodeBlocks)))
	}
	if s.MathBlocks > 0 {
		blocks = append(blocks, fmt.Sprintf("%d math", s.MathBlocks))
	}
	if s.Footnotes > 0 {
		blocks = append(blocks, fmt.Sprintf("%d footnote%s", s.Footnotes, pluralS(s.Footnotes)))
	}
	if s.DefLists > 0 {
		blocks = append(blocks, fmt.Sprintf("%d deflist%s", s.DefLists, pluralS(s.DefLists)))
	}
	if s.HTMLBlocks > 0 {
		blocks = append(blocks, fmt.Sprintf("%d HTML", s.HTMLBlocks))
	}
	if len(blocks) > 0 {
		lines = append(lines, strings.Join(blocks, " · "))
	}

	var interactive []string
	if s.Tasks > 0 {
		interactive = append(interactive, fmt.Sprintf("%d task%s (%d done)", s.Tasks, pluralS(s.Tasks), s.TasksChecked))
	}
	if s.WikiLinks > 0 {
		interactive = append(interactive, fmt.Sprintf("%d wiki", s.WikiLinks))
	}
	if s.WikiEmbeds > 0 {
		interactive = append(interactive, fmt.Sprintf("%d embed%s", s.WikiEmbeds, pluralS(s.WikiEmbeds)))
	}
	if len(interactive) > 0 {
		lines = append(lines, strings.Join(interactive, " · "))
	}

	var refs []string
	if s.Mentions > 0 {
		refs = append(refs, fmt.Sprintf("%d @mention%s", s.Mentions, pluralS(s.Mentions)))
	}
	if s.IssueRefs > 0 {
		refs = append(refs, fmt.Sprintf("%d #issue%s", s.IssueRefs, pluralS(s.IssueRefs)))
	}
	if s.CommitRefs > 0 {
		refs = append(refs, fmt.Sprintf("%d sha", s.CommitRefs))
	}
	if s.Emojis > 0 {
		refs = append(refs, fmt.Sprintf("%d emoji", s.Emojis))
	}
	if len(refs) > 0 {
		lines = append(lines, strings.Join(refs, " · "))
	}

	return lines
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
	// Skip headings with no title (e.g. a bare `##` in source).
	if strings.TrimSpace(s.Title) == "" {
		return
	}

	// Connector.
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	// Token count.
	tokenStr := dim + fmt.Sprintf("(%s)", formatTokens(s.Tokens)) + reset

	// Title color by level.
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

	// Dense inline annotation of notables + stats for this section.
	// If the section has none, fall back to its extracted key terms so
	// sparse sections still carry some signal on the title line.
	annotation := notableAnnotation(s)
	if annotation == "" && len(s.KeyTerms) > 0 {
		annotation = strings.Join(s.KeyTerms, ", ")
	}
	annotationStr := ""
	if annotation != "" {
		annotationStr = dim + " · " + annotation + reset
	}

	// Print section title line.
	fmt.Printf("%s%s%s%s%s %s%s\n", prefix, dim, connector, reset, titleColor+s.Title+reset, tokenStr, annotationStr)

	// Sub-item prefix for children.
	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	// Render children sections.
	for i, child := range s.Children {
		childIsLast := i == len(s.Children)-1
		renderSection(child, childPrefix, childIsLast, isFiltered)
	}
}

// notableAnnotation builds a dense one-line summary of every notable
// construct and aggregate stat in a section. Similar items are grouped
// (e.g. all code blocks become `go :142, python :154, ...`) and joined
// with " · ". Returns "" if there's nothing worth showing.
func notableAnnotation(s *parser.Section) string {
	var parts []string

	// Group notables by kind, preserving document order of first appearance.
	byKind := make(map[parser.NodeKind][]parser.Node)
	var order []parser.NodeKind
	for _, n := range s.Notables {
		if _, seen := byKind[n.Kind()]; !seen {
			order = append(order, n.Kind())
		}
		byKind[n.Kind()] = append(byKind[n.Kind()], n)
	}
	for _, kind := range order {
		if group := formatNotableGroup(kind, byKind[kind]); group != "" {
			parts = append(parts, group)
		}
	}

	// Aggregate stats (one piece per non-zero counter).
	if s.Stats.Tasks > 0 {
		parts = append(parts, fmt.Sprintf("%d tasks (%d done)", s.Stats.Tasks, s.Stats.TasksChecked))
	}
	if s.Stats.WikiLinks > 0 {
		parts = append(parts, fmt.Sprintf("%d wiki", s.Stats.WikiLinks))
	}
	if s.Stats.WikiEmbeds > 0 {
		parts = append(parts, fmt.Sprintf("%d embed%s", s.Stats.WikiEmbeds, pluralS(s.Stats.WikiEmbeds)))
	}
	if s.Stats.Mentions > 0 {
		parts = append(parts, fmt.Sprintf("%d @mention%s", s.Stats.Mentions, pluralS(s.Stats.Mentions)))
	}
	if s.Stats.IssueRefs > 0 {
		parts = append(parts, fmt.Sprintf("%d #issue%s", s.Stats.IssueRefs, pluralS(s.Stats.IssueRefs)))
	}
	if s.Stats.CommitRefs > 0 {
		parts = append(parts, fmt.Sprintf("%d sha", s.Stats.CommitRefs))
	}
	if s.Stats.Emojis > 0 {
		parts = append(parts, fmt.Sprintf("%d emoji", s.Stats.Emojis))
	}

	return strings.Join(parts, " · ")
}

// formatNotableGroup renders all notables of one kind as a compact comma
// list, each item carrying its :line location so it can be jumped to.
func formatNotableGroup(kind parser.NodeKind, nodes []parser.Node) string {
	switch kind {
	case parser.KindCallout:
		var pieces []string
		for _, n := range nodes {
			c := n.(*parser.Callout)
			pieces = append(pieces, fmt.Sprintf("%s :%d", c.Variant, c.LineStart()))
		}
		return strings.Join(pieces, ", ")

	case parser.KindTable:
		// Each table gets `:line (first header)` so you can tell which is
		// which by its leading column name.
		var pieces []string
		for _, n := range nodes {
			t := n.(*parser.Table)
			first := ""
			if len(t.Headers) > 0 {
				first = t.Headers[0]
				if len(first) > 16 {
					first = first[:13] + "..."
				}
			}
			if first == "" {
				pieces = append(pieces, fmt.Sprintf(":%d", n.LineStart()))
			} else {
				pieces = append(pieces, fmt.Sprintf(":%d %s", n.LineStart(), first))
			}
		}
		return fmt.Sprintf("%d table%s %s", len(nodes), pluralS(len(nodes)), strings.Join(pieces, ", "))

	case parser.KindCodeBlock:
		var pieces []string
		for _, n := range nodes {
			cb := n.(*parser.CodeBlock)
			lang := cb.Language
			if lang == "" {
				if cb.Fenced {
					lang = "(none)"
				} else {
					lang = "(indent)"
				}
			}
			if cb.LineEnd() > cb.LineStart() {
				pieces = append(pieces, fmt.Sprintf("%s :%d-%d", lang, cb.LineStart(), cb.LineEnd()))
			} else {
				pieces = append(pieces, fmt.Sprintf("%s :%d", lang, cb.LineStart()))
			}
		}
		return strings.Join(pieces, ", ")

	case parser.KindMathBlock:
		var locs []string
		for _, n := range nodes {
			locs = append(locs, fmt.Sprintf(":%d", n.LineStart()))
		}
		return fmt.Sprintf("%d math %s", len(nodes), strings.Join(locs, ", "))

	case parser.KindHTMLBlock:
		var pieces []string
		for _, n := range nodes {
			h := n.(*parser.HTMLBlock)
			tag := htmlTag(h.Raw)
			if tag == "" || strings.HasPrefix(tag, "/") || tag == "comment" {
				continue
			}
			pieces = append(pieces, fmt.Sprintf("<%s> :%d", tag, n.LineStart()))
		}
		return strings.Join(pieces, ", ")

	case parser.KindFootnoteDef:
		var pieces []string
		for _, n := range nodes {
			f := n.(*parser.FootnoteDef)
			pieces = append(pieces, fmt.Sprintf("^%s :%d", f.ID, f.LineStart()))
		}
		return strings.Join(pieces, ", ")

	case parser.KindDefinitionList:
		var pieces []string
		for _, n := range nodes {
			dl := n.(*parser.DefinitionList)
			terms := 0
			for _, k := range dl.Kids {
				if _, ok := k.(*parser.DefinitionTerm); ok {
					terms++
				}
			}
			pieces = append(pieces, fmt.Sprintf("%d terms :%d", terms, dl.LineStart()))
		}
		return strings.Join(pieces, ", ")

	case parser.KindLinkRefDef:
		var pieces []string
		for _, n := range nodes {
			lrd := n.(*parser.LinkRefDef)
			pieces = append(pieces, fmt.Sprintf("[%s] :%d", lrd.Label, lrd.LineStart()))
		}
		return fmt.Sprintf("%d ref%s %s", len(nodes), pluralS(len(nodes)), strings.Join(pieces, ", "))
	}
	return ""
}

// htmlTag pulls the tag name out of a raw HTML block's opening element.
func htmlTag(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "<") {
		return ""
	}
	raw = strings.TrimPrefix(raw, "<")
	if strings.HasPrefix(raw, "!--") {
		return "comment"
	}
	end := 0
	for end < len(raw) && (raw[end] != ' ' && raw[end] != '>' && raw[end] != '\n') {
		end++
	}
	return raw[:end]
}

// TypeFilter drills down into a single node kind across the whole document.
// Equivalent to TypeFilterFiltered with no sub-filter.
func TypeFilter(doc *parser.Document, kindName string) {
	TypeFilterFiltered(doc, kindName, "", "")
}

// TypeFilterFiltered narrows the --type view further by an optional --lang
// (for code blocks) or --kind (for callout variants). Unmatched sub-filters
// are silently ignored when the kind doesn't support them.
func TypeFilterFiltered(doc *parser.Document, kindName, lang, variant string) {
	kind, ok := resolveKindName(kindName)
	if !ok {
		fmt.Printf("Unknown type %q. Try: code, callout, table, math, footnote, deflist, linkref, html, task, wiki, embed, mention, issue, sha, emoji\n", kindName)
		return
	}

	// Walk every section and collect matching nodes along with their
	// section breadcrumb.
	type hit struct {
		section *parser.Section
		nodes   []parser.Node
		count   int // for aggregate-only kinds like Tasks
		extra   int // TasksChecked, etc.
	}
	var hits []hit

	var walkSections func(sections []*parser.Section)
	walkSections = func(sections []*parser.Section) {
		for _, s := range sections {
			h := hit{section: s}
			// Per-instance notables, with optional sub-filter.
			for _, n := range s.Notables {
				if n.Kind() != kind {
					continue
				}
				if !matchesSubFilter(n, lang, variant) {
					continue
				}
				h.nodes = append(h.nodes, n)
			}
			// Aggregate stats.
			switch kind {
			case parser.KindTaskItem:
				h.count = s.Stats.Tasks
				h.extra = s.Stats.TasksChecked
			case parser.KindWikiLink:
				h.count = s.Stats.WikiLinks
			case parser.KindWikiEmbed:
				h.count = s.Stats.WikiEmbeds
			case parser.KindMention:
				h.count = s.Stats.Mentions
			case parser.KindIssueRef:
				h.count = s.Stats.IssueRefs
			case parser.KindCommitRef:
				h.count = s.Stats.CommitRefs
			case parser.KindEmoji:
				h.count = s.Stats.Emojis
			}
			if len(h.nodes) > 0 || h.count > 0 {
				hits = append(hits, h)
			}
			walkSections(s.Children)
		}
	}
	walkSections(doc.Sections)

	// Header box.
	total := 0
	for _, h := range hits {
		total += len(h.nodes) + h.count
	}
	label := kindDisplayName(kind)
	info := fmt.Sprintf("%d %s in %d section%s", total, label, len(hits), pluralS(len(hits)))
	printMiniHeader(doc.Filename+" — "+label, info)

	if len(hits) == 0 {
		fmt.Printf("No %s found.\n\n", label)
		return
	}

	for _, h := range hits {
		crumb := breadcrumb(h.section)
		fmt.Printf("%s%s%s %s(%s)%s\n", bold+cyan, crumb, reset, dim, formatTokens(h.section.Tokens), reset)
		for _, n := range h.nodes {
			fmt.Printf("  %s%s%s\n", dim, formatTypeHit(kind, n), reset)
		}
		if h.count > 0 {
			switch kind {
			case parser.KindTaskItem:
				fmt.Printf("  %s%d tasks (%d done)%s\n", dim, h.count, h.extra, reset)
			default:
				fmt.Printf("  %s%d %s%s\n", dim, h.count, label, reset)
			}
		}
		fmt.Println()
	}
}

// matchesSubFilter checks optional --lang / --kind sub-filters against a
// notable node. An empty filter always matches; unrelated node types
// (where the filter doesn't apply) also always match.
func matchesSubFilter(n parser.Node, lang, variant string) bool {
	if lang != "" {
		if cb, ok := n.(*parser.CodeBlock); ok {
			if !strings.EqualFold(cb.Language, lang) {
				return false
			}
		}
	}
	if variant != "" {
		if c, ok := n.(*parser.Callout); ok {
			if !strings.EqualFold(string(c.Variant), variant) {
				return false
			}
		}
	}
	return true
}

// resolveKindName maps a short CLI token to a parser.NodeKind.
func resolveKindName(name string) (parser.NodeKind, bool) {
	switch strings.ToLower(name) {
	case "code", "codeblock", "code-block":
		return parser.KindCodeBlock, true
	case "callout", "alert", "admonition":
		return parser.KindCallout, true
	case "table":
		return parser.KindTable, true
	case "math":
		return parser.KindMathBlock, true
	case "footnote", "footnotes":
		return parser.KindFootnoteDef, true
	case "deflist", "definition", "definitions":
		return parser.KindDefinitionList, true
	case "linkref", "linkrefs", "ref", "refs":
		return parser.KindLinkRefDef, true
	case "html":
		return parser.KindHTMLBlock, true
	case "task", "tasks":
		return parser.KindTaskItem, true
	case "wiki", "wikilink":
		return parser.KindWikiLink, true
	case "embed":
		return parser.KindWikiEmbed, true
	case "mention", "mentions":
		return parser.KindMention, true
	case "issue", "issues":
		return parser.KindIssueRef, true
	case "sha", "commit":
		return parser.KindCommitRef, true
	case "emoji":
		return parser.KindEmoji, true
	}
	return "", false
}

// kindDisplayName returns the plural, human-friendly label for a kind.
func kindDisplayName(k parser.NodeKind) string {
	switch k {
	case parser.KindCodeBlock:
		return "code blocks"
	case parser.KindCallout:
		return "callouts"
	case parser.KindTable:
		return "tables"
	case parser.KindMathBlock:
		return "math blocks"
	case parser.KindFootnoteDef:
		return "footnotes"
	case parser.KindDefinitionList:
		return "definition lists"
	case parser.KindLinkRefDef:
		return "link refs"
	case parser.KindHTMLBlock:
		return "HTML blocks"
	case parser.KindTaskItem:
		return "tasks"
	case parser.KindWikiLink:
		return "wiki links"
	case parser.KindWikiEmbed:
		return "embeds"
	case parser.KindMention:
		return "mentions"
	case parser.KindIssueRef:
		return "issue refs"
	case parser.KindCommitRef:
		return "commit refs"
	case parser.KindEmoji:
		return "emoji"
	}
	return string(k)
}

// formatTypeHit renders a single node in the drill-down view. Compared to
// the dense tree annotation it can afford to show more detail per item
// since the whole screen is one kind.
func formatTypeHit(kind parser.NodeKind, n parser.Node) string {
	switch v := n.(type) {
	case *parser.CodeBlock:
		lang := v.Language
		if lang == "" {
			if v.Fenced {
				lang = "(none)"
			} else {
				lang = "(indent)"
			}
		}
		if v.LineEnd() > v.LineStart() {
			return fmt.Sprintf(":%d-%d  %-10s", v.LineStart(), v.LineEnd(), lang)
		}
		return fmt.Sprintf(":%d     %-10s", v.LineStart(), lang)
	case *parser.Callout:
		snippet := calloutSnippetText(v)
		return fmt.Sprintf(":%-4d  %-10s  %s", v.LineStart(), v.Variant, snippet)
	case *parser.Table:
		hdrs := strings.Join(v.Headers, " | ")
		return fmt.Sprintf(":%-4d  %dcol  %s", v.LineStart(), len(v.Headers), hdrs)
	case *parser.MathBlock:
		preview := v.TeX
		if len(preview) > 60 {
			preview = preview[:57] + "..."
		}
		return fmt.Sprintf(":%-4d  %s", v.LineStart(), preview)
	case *parser.FootnoteDef:
		return fmt.Sprintf(":%-4d  ^%s", v.LineStart(), v.ID)
	case *parser.DefinitionList:
		terms := 0
		for _, k := range v.Kids {
			if _, ok := k.(*parser.DefinitionTerm); ok {
				terms++
			}
		}
		return fmt.Sprintf(":%-4d  %d terms", v.LineStart(), terms)
	case *parser.LinkRefDef:
		title := v.Title
		if title != "" {
			title = " \"" + title + "\""
		}
		return fmt.Sprintf(":%-4d  [%s] → %s%s", v.LineStart(), v.Label, v.URL, title)
	case *parser.HTMLBlock:
		tag := htmlTag(v.Raw)
		return fmt.Sprintf(":%-4d  <%s>", v.LineStart(), tag)
	}
	return fmt.Sprintf(":%d", n.LineStart())
}

// calloutSnippetText extracts the first line of a callout's body, stripping
// the [!KIND] marker, so --type callout can show meaningful context.
func calloutSnippetText(c *parser.Callout) string {
	if len(c.Kids) == 0 {
		return ""
	}
	p, ok := c.Kids[0].(*parser.Paragraph)
	if !ok {
		return ""
	}
	text := p.Text
	if i := strings.Index(text, "]"); i >= 0 && strings.HasPrefix(text, "[!") {
		text = strings.TrimSpace(text[i+1:])
	}
	if i := strings.Index(text, "\n"); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

// breadcrumb walks up from a section to the root, joining titles with ` > `.
func breadcrumb(s *parser.Section) string {
	var parts []string
	for cur := s; cur != nil; cur = cur.Parent {
		parts = append([]string{cur.Title}, parts...)
	}
	return strings.Join(parts, " > ")
}

// printMiniHeader is a single-line header box for focused views like --type.
func printMiniHeader(title, info string) {
	width := len(title) + 4
	if len(info)+4 > width {
		width = len(info) + 4
	}
	if width < 40 {
		width = 40
	}
	titleLine := fmt.Sprintf(" %s ", title)
	padding := width - len(titleLine)
	left := padding / 2
	right := padding - left
	fmt.Printf("╭%s%s%s╮\n", strings.Repeat("─", left), titleLine, strings.Repeat("─", right))
	fmt.Printf("│ %-*s │\n", width-2, centerText(info, width-2))
	fmt.Printf("╰%s╯\n\n", strings.Repeat("─", width))
}

// AtLine answers "what's at line N?" It finds the tightest containing
// node in the typed AST and prints its kind, location, containing section
// breadcrumb, and a short preview. Useful when an agent has a line number
// from elsewhere (a grep hit, a diff, an error) and needs to know what
// construct lives there.
func AtLine(doc *parser.Document, line int) {
	// Find the deepest block node whose range contains `line`.
	var found parser.Node
	var findDeepest func(nodes []parser.Node)
	findDeepest = func(nodes []parser.Node) {
		for _, n := range nodes {
			if n.LineStart() <= line && (n.LineEnd() == 0 || n.LineEnd() >= line) {
				found = n
			}
			findDeepest(n.Children())
		}
	}
	findDeepest(doc.Nodes)

	// Find the section breadcrumb that contains this line.
	var containingSection *parser.Section
	var walkSections func(sections []*parser.Section)
	walkSections = func(sections []*parser.Section) {
		for _, s := range sections {
			if s.LineStart <= line && (s.LineEnd == 0 || s.LineEnd >= line) {
				containingSection = s
				walkSections(s.Children)
			}
		}
	}
	walkSections(doc.Sections)

	info := fmt.Sprintf("line %d", line)
	printMiniHeader(doc.Filename+" — "+info, nodeAtSummary(found, containingSection))

	if containingSection != nil {
		fmt.Printf("%sSection:%s %s%s%s\n", bold, reset, cyan, breadcrumb(containingSection), reset)
	}
	if found != nil {
		fmt.Printf("%sNode:   %s %s\n", bold, reset, detailForNode(found))
	} else {
		fmt.Println("No matching node.")
	}
	fmt.Println()
}

// nodeAtSummary is the single-line subtitle for the --at header.
func nodeAtSummary(n parser.Node, s *parser.Section) string {
	if n == nil && s == nil {
		return "nothing here"
	}
	if n != nil {
		return string(n.Kind())
	}
	return "in " + s.Title
}

// detailForNode prints a rich description for the most common node kinds,
// falling back to a generic Kind + line range for everything else.
func detailForNode(n parser.Node) string {
	switch v := n.(type) {
	case *parser.Heading:
		return fmt.Sprintf("heading L%d  level=%d  %q", v.LineStart(), v.Level, v.Title)
	case *parser.Paragraph:
		text := v.Text
		if len(text) > 80 {
			text = text[:77] + "..."
		}
		return fmt.Sprintf("paragraph L%d-%d  %s", v.LineStart(), v.LineEnd(), text)
	case *parser.CodeBlock:
		lang := v.Language
		if lang == "" {
			if v.Fenced {
				lang = "(none)"
			} else {
				lang = "(indent)"
			}
		}
		return fmt.Sprintf("code L%d-%d  lang=%s", v.LineStart(), v.LineEnd(), lang)
	case *parser.Callout:
		return fmt.Sprintf("callout L%d  kind=%s  %s", v.LineStart(), v.Variant, calloutSnippetText(v))
	case *parser.Table:
		return fmt.Sprintf("table L%d  %dcol  %s", v.LineStart(), len(v.Headers), strings.Join(v.Headers, " | "))
	case *parser.List:
		kind := "unordered"
		if v.Ordered {
			kind = "ordered"
		}
		return fmt.Sprintf("list L%d-%d  %s", v.LineStart(), v.LineEnd(), kind)
	case *parser.Blockquote:
		return fmt.Sprintf("blockquote L%d-%d", v.LineStart(), v.LineEnd())
	case *parser.MathBlock:
		return fmt.Sprintf("math L%d  %s", v.LineStart(), v.TeX)
	case *parser.HTMLBlock:
		return fmt.Sprintf("html L%d  %s", v.LineStart(), htmlTag(v.Raw))
	case *parser.FootnoteDef:
		return fmt.Sprintf("footnote L%d  ^%s", v.LineStart(), v.ID)
	case *parser.LinkRefDef:
		return fmt.Sprintf("link ref L%d  [%s] → %s", v.LineStart(), v.Label, v.URL)
	case *parser.DefinitionList:
		return fmt.Sprintf("deflist L%d-%d", v.LineStart(), v.LineEnd())
	case *parser.Frontmatter:
		return fmt.Sprintf("frontmatter L%d-%d  format=%s", v.LineStart(), v.LineEnd(), v.Format)
	case *parser.ThematicBreak:
		return fmt.Sprintf("thematic break L%d", v.LineStart())
	}
	return fmt.Sprintf("%s L%d-%d", n.Kind(), n.LineStart(), n.LineEnd())
}

// ChangedSince renders the sections and notables that intersect any line
// in `changed`. It's the CLI answer to "what changed in these docs since
// ref X?" — each hit shows its breadcrumb and the specific constructs
// that sit on changed lines.
func ChangedSince(doc *parser.Document, changed map[int]bool, ref string) {
	if len(changed) == 0 {
		printMiniHeader(doc.Filename+" — since "+ref, "no changes")
		return
	}

	// Find sections whose range intersects any changed line, and within
	// each, the notables whose start line is in the changed set.
	type hit struct {
		section  *parser.Section
		notables []parser.Node
		lines    []int
	}
	var hits []hit

	var walkSections func(sections []*parser.Section)
	walkSections = func(sections []*parser.Section) {
		for _, s := range sections {
			if sectionHasChangedLine(s, changed) {
				h := hit{section: s}
				for _, n := range s.Notables {
					if changed[n.LineStart()] {
						h.notables = append(h.notables, n)
					}
				}
				// Collect which changed lines fall inside this section (not
				// its children) so we can show them even when no notable
				// lives on those exact lines.
				for line := s.LineStart; line <= s.LineEnd; line++ {
					if changed[line] && !lineBelongsToDeeperSection(line, s.Children) {
						h.lines = append(h.lines, line)
					}
				}
				if len(h.notables) > 0 || len(h.lines) > 0 {
					hits = append(hits, h)
				}
			}
			walkSections(s.Children)
		}
	}
	walkSections(doc.Sections)

	totalLines := len(changed)
	info := fmt.Sprintf("%d changed line%s across %d section%s",
		totalLines, pluralS(totalLines), len(hits), pluralS(len(hits)))
	printMiniHeader(doc.Filename+" — since "+ref, info)

	if len(hits) == 0 {
		fmt.Println("Changes are outside any heading (frontmatter, intro, etc).")
		fmt.Println()
		return
	}

	for _, h := range hits {
		crumb := breadcrumb(h.section)
		fmt.Printf("%s%s%s %s(%s)%s\n", bold+cyan, crumb, reset, dim, formatTokens(h.section.Tokens), reset)
		for _, n := range h.notables {
			fmt.Printf("  %s%s%s\n", dim, detailForNode(n), reset)
		}
		if len(h.notables) == 0 && len(h.lines) > 0 {
			fmt.Printf("  %s%d line%s changed%s\n", dim, len(h.lines), pluralS(len(h.lines)), reset)
		}
		fmt.Println()
	}
}

func sectionHasChangedLine(s *parser.Section, changed map[int]bool) bool {
	for line := s.LineStart; line <= s.LineEnd; line++ {
		if changed[line] {
			return true
		}
	}
	return false
}

func lineBelongsToDeeperSection(line int, children []*parser.Section) bool {
	for _, c := range children {
		if c.LineStart <= line && line <= c.LineEnd {
			return true
		}
	}
	return false
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

// MultiTree renders multiple documents as a combined directory view with
// aggregated inventory summary at the top and a per-file one-liner that
// surfaces each file's most signal-dense notables.
func MultiTree(docs []*parser.Document, dirName string) {
	// Aggregate summary across every doc so the header shows a project-wide
	// inventory — "what's in this whole directory?"
	agg := aggregateSummary(docs)

	totalTokens := 0
	totalSections := 0
	for _, doc := range docs {
		totalTokens += doc.TotalTokens
		totalSections += len(doc.GetAllSections())
	}
	mainInfo := fmt.Sprintf("%d files │ %d sections │ ~%s tokens", len(docs), totalSections, formatTokens(totalTokens))
	summaryLines := buildSummaryLines(agg)
	allLines := append([]string{mainInfo}, summaryLines...)

	// Compute width from the widest line.
	innerWidth := 60
	titleLine := fmt.Sprintf(" %s/ ", dirName)
	if len(titleLine) > innerWidth {
		innerWidth = len(titleLine) + 4
	}
	for _, line := range allLines {
		if len(line)+4 > innerWidth {
			innerWidth = len(line) + 4
		}
	}

	// Header box.
	padding := innerWidth - len(titleLine)
	leftPad := padding / 2
	rightPad := padding - leftPad
	fmt.Printf("╭%s%s%s╮\n", strings.Repeat("─", leftPad), titleLine, strings.Repeat("─", rightPad))
	for _, line := range allLines {
		fmt.Printf("│ %-*s │\n", innerWidth-2, centerText(line, innerWidth-2))
	}
	fmt.Printf("╰%s╯\n", strings.Repeat("─", innerWidth))
	fmt.Println()

	// Per-file one-liner with each file's own mini-summary.
	for i, doc := range docs {
		isLast := i == len(docs)-1
		renderDocSummary(doc, isLast)
	}
	fmt.Println()
}

// aggregateSummary sums ContentSummary across every doc so the multi-file
// header can show a single project-wide inventory.
func aggregateSummary(docs []*parser.Document) parser.ContentSummary {
	var agg parser.ContentSummary
	for _, d := range docs {
		s := d.Summary()
		agg.Callouts += s.Callouts
		agg.Tables += s.Tables
		agg.CodeBlocks += s.CodeBlocks
		agg.MathBlocks += s.MathBlocks
		agg.HTMLBlocks += s.HTMLBlocks
		agg.Footnotes += s.Footnotes
		agg.DefLists += s.DefLists
		agg.LinkRefDefs += s.LinkRefDefs
		agg.Tasks += s.Tasks
		agg.TasksChecked += s.TasksChecked
		agg.WikiLinks += s.WikiLinks
		agg.WikiEmbeds += s.WikiEmbeds
		agg.Mentions += s.Mentions
		agg.IssueRefs += s.IssueRefs
		agg.CommitRefs += s.CommitRefs
		agg.Emojis += s.Emojis
	}
	return agg
}

// renderDocSummary prints one line per document showing the filename, token
// count, section count, and a condensed notable inventory — enough for an
// agent to decide which file to open.
func renderDocSummary(doc *parser.Document, isLast bool) {
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	sectionCount := len(doc.GetAllSections())
	tokenStr := dim + fmt.Sprintf("(%s, %d §)", formatTokens(doc.TotalTokens), sectionCount) + reset

	// Build a one-line notable digest for this file: counts of interesting
	// constructs, skipping anything with zero.
	s := doc.Summary()
	var parts []string
	if s.CodeBlocks > 0 {
		parts = append(parts, fmt.Sprintf("%d code", s.CodeBlocks))
	}
	if s.Callouts > 0 {
		parts = append(parts, fmt.Sprintf("%d callouts", s.Callouts))
	}
	if s.Tables > 0 {
		parts = append(parts, fmt.Sprintf("%d tables", s.Tables))
	}
	if s.Tasks > 0 {
		parts = append(parts, fmt.Sprintf("%d tasks (%d done)", s.Tasks, s.TasksChecked))
	}
	if s.MathBlocks > 0 {
		parts = append(parts, fmt.Sprintf("%d math", s.MathBlocks))
	}
	if s.Footnotes > 0 {
		parts = append(parts, fmt.Sprintf("%d footnotes", s.Footnotes))
	}
	if s.WikiLinks > 0 {
		parts = append(parts, fmt.Sprintf("%d wiki", s.WikiLinks))
	}
	if s.WikiEmbeds > 0 {
		parts = append(parts, fmt.Sprintf("%d embeds", s.WikiEmbeds))
	}
	annotation := ""
	if len(parts) > 0 {
		annotation = dim + " · " + strings.Join(parts, " · ") + reset
	}

	fmt.Printf("%s%s%s%s %s%s\n", dim, connector, reset, bold+green+doc.Filename+reset, tokenStr, annotation)
}

// SearchResult holds a matched section with its file context
type SearchResult struct {
	Filename string
	Path     string // e.g. "endpoints > Ban Member"
	Section  *parser.Section
}

// SearchResults searches all docs for sections matching the query and renders results
func SearchResults(docs []*parser.Document, query string) {
	query = strings.ToLower(query)
	var results []SearchResult

	for _, doc := range docs {
		searchSections(doc.Filename, doc.Sections, "", query, &results)
	}

	if len(results) == 0 {
		fmt.Printf("No sections matching '%s'\n", query)
		return
	}

	// Header
	fmt.Printf("%s%d matches for '%s'%s\n\n", bold, len(results), query, reset)

	for i, r := range results {
		isLast := i == len(results)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		tokenStr := dim + fmt.Sprintf("(%s)", formatTokens(r.Section.Tokens)) + reset
		filePart := dim + r.Filename + " > " + reset
		if r.Path != "" {
			filePart = dim + r.Filename + " > " + r.Path + " > " + reset
		}
		fmt.Printf("%s%s%s%s%s%s %s\n", dim, connector, reset, filePart, bold+cyan+r.Section.Title+reset, "", tokenStr)

		// Show children summary if present
		if len(r.Section.Children) > 0 {
			childPrefix := "│   "
			if isLast {
				childPrefix = "    "
			}
			for j, child := range r.Section.Children {
				if j >= 5 {
					fmt.Printf("%s%s... %d more%s\n", childPrefix, dim, len(r.Section.Children)-5, reset)
					break
				}
				childIsLast := j == len(r.Section.Children)-1 || j == 4
				childConn := "├─ "
				if childIsLast {
					childConn = "└─ "
				}
				fmt.Printf("%s%s%s%s %s\n", childPrefix, dim, childConn, child.Title+reset, dim+fmt.Sprintf("(%s)", formatTokens(child.Tokens))+reset)
			}
		}
	}

	fmt.Println()
}

func searchSections(filename string, sections []*parser.Section, parentPath string, query string, results *[]SearchResult) {
	for _, s := range sections {
		match := strings.Contains(strings.ToLower(s.Title), query) ||
			strings.Contains(strings.ToLower(s.Content), query) ||
			notablesMatch(s, query)

		if match {
			*results = append(*results, SearchResult{
				Filename: filename,
				Path:     parentPath,
				Section:  s,
			})
		}

		// Search children
		childPath := parentPath
		if childPath != "" {
			childPath += " > " + s.Title
		} else {
			childPath = s.Title
		}
		searchSections(filename, s.Children, childPath, query, results)
	}
}

// notablesMatch returns true if any of a section's notable nodes match the
// query. The per-kind comparisons cover the fields an agent is likely to
// search by: code language, callout variant, wiki link target, link ref
// label, footnote id, etc.
func notablesMatch(s *parser.Section, query string) bool {
	for _, n := range s.Notables {
		switch v := n.(type) {
		case *parser.CodeBlock:
			if strings.Contains(strings.ToLower(v.Language), query) ||
				strings.Contains(strings.ToLower(v.Code), query) {
				return true
			}
		case *parser.Callout:
			if strings.Contains(strings.ToLower(string(v.Variant)), query) {
				return true
			}
			for _, k := range v.Kids {
				if p, ok := k.(*parser.Paragraph); ok {
					if strings.Contains(strings.ToLower(p.Text), query) {
						return true
					}
				}
			}
		case *parser.Table:
			for _, h := range v.Headers {
				if strings.Contains(strings.ToLower(h), query) {
					return true
				}
			}
		case *parser.MathBlock:
			if strings.Contains(strings.ToLower(v.TeX), query) {
				return true
			}
		case *parser.FootnoteDef:
			if strings.Contains(strings.ToLower(v.ID), query) {
				return true
			}
		case *parser.LinkRefDef:
			if strings.Contains(strings.ToLower(v.Label), query) ||
				strings.Contains(strings.ToLower(v.URL), query) {
				return true
			}
		case *parser.HTMLBlock:
			if strings.Contains(strings.ToLower(v.Raw), query) {
				return true
			}
		}
	}
	return false
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
	if len(titleLine) > innerWidth {
		innerWidth = len(titleLine) + 4
	}
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
