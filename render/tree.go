package render

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/JordanCoin/docmap/parser"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	cyan   = "\033[36m"
	yellow = "\033[33m"
	green  = "\033[32m"
	blue   = "\033[34m"
)

func init() { applyColorPolicy() }

// applyColorPolicy blanks every escape when stdout is not a terminal or
// NO_COLOR is set (https://no-color.org). Agents capture stdout; raw
// escapes in their context are noise.
func applyColorPolicy() {
	if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd())) {
		reset, bold, dim, cyan, yellow, green, blue = "", "", "", "", "", "", ""
	}
}

// largestSection returns the title of the section with the most tokens, so
// the footer names a real target.
func largestSection(doc *parser.Document) string {
	best := ""
	max := -1
	for _, s := range doc.GetAllSections() {
		// Skip the H1 when deeper sections exist: expanding it dumps the file.
		if s.Level == 1 && len(s.Children) > 0 {
			continue
		}
		if s.Tokens > max && strings.TrimSpace(s.Title) != "" {
			best, max = s.Title, s.Tokens
		}
	}
	return best
}

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
	if title := largestSection(doc); title != "" {
		fmt.Printf("expand a section: docmap %s --expand %q\n", doc.Filename, title)
	}
}

func printHeader(doc *parser.Document) {
	// Main info line.
	sectionCount := len(doc.GetAllSections())
	mainInfo := fmt.Sprintf("Sections: %d │ ~%s tokens", sectionCount, formatTokens(doc.TotalTokens))

	// Build content summary lines from the typed AST.
	summaryLines := buildSummaryLines(doc.Summary())

	// Join all info lines for width calculation.
	allLines := append([]string{mainInfo}, summaryLines...)

	displayName := doc.Filename

	// Inner width = max of title, main info, and every summary line.
	innerWidth := 60
	titleLine := " " + displayName + " "
	if runewidth.StringWidth(titleLine) > innerWidth {
		innerWidth = runewidth.StringWidth(titleLine) + 4
	}
	for _, line := range allLines {
		if len(line)+4 > innerWidth {
			innerWidth = len(line) + 4
		}
	}

	// Top border with centered title.
	top, innerWidth := boxLine(displayName, innerWidth)
	fmt.Println(top)

	// Info lines: main line, then one line per summary group.
	for _, line := range allLines {
		fmt.Println(boxInfo(line, innerWidth))
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
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) >= width {
		return runewidth.Truncate(s, width, "")
	}
	padding := (width - runewidth.StringWidth(s)) / 2
	return strings.Repeat(" ", padding) + s
}

// boxLine returns a centered title border and the effective inner width. The
// width is capped to the terminal so long paths are shortened instead of
// wrapping or producing negative Repeat counts.
func boxLine(title string, minWidth int) (string, int) {
	if minWidth < 1 {
		minWidth = 1
	}
	maxInner := terminalWidth() - 2
	if maxInner < 1 {
		maxInner = 1
	}
	width := minWidth
	titleWidth := runewidth.StringWidth(title) + 2
	if titleWidth > width {
		width = titleWidth
	}
	if width > maxInner {
		width = maxInner
	}
	titleWidth = width - 2
	titleLine := title
	if width >= 3 {
		title = truncateTitle(title, titleWidth)
		titleLine = " " + title + " "
	} else {
		title = truncateTitle(title, width)
		titleLine = title
	}
	padding := width - runewidth.StringWidth(titleLine)
	left := padding / 2
	right := padding - left
	return fmt.Sprintf("╭%s%s%s╮", strings.Repeat("─", left), titleLine, strings.Repeat("─", right)), width
}

func truncateTitle(title string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(title) <= width {
		return title
	}
	if width <= 3 {
		return runewidth.Truncate(title, width, "…")
	}
	return runewidth.Truncate(title, width, "…")
}

func boxInfo(line string, width int) string {
	if width < 2 {
		return "│" + strings.Repeat(" ", width) + "│"
	}
	inner := width - 2
	return "│ " + runewidth.FillRight(centerText(line, inner), inner) + " │"
}

func terminalWidth() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 2 {
		return n
	}
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 2 {
		return width
	}
	return 80
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
	typeFilterDoc(doc, kind, lang, variant, false)
}

// TypeFilterDocs is TypeFilterFiltered across a directory: every document
// that has the construct, skipping the ones that do not, then a total.
// "Where are the bash examples in these docs" is a directory question.
func TypeFilterDocs(docs []*parser.Document, kindName, lang, variant string) {
	kind, ok := resolveKindName(kindName)
	if !ok {
		fmt.Printf("Unknown type %q. Try: code, callout, table, math, footnote, deflist, linkref, html, task, wiki, embed, mention, issue, sha, emoji\n", kindName)
		return
	}
	total, files := 0, 0
	for _, doc := range docs {
		if n := typeFilterDoc(doc, kind, lang, variant, true); n > 0 {
			total += n
			files++
		}
	}
	label := kindDisplayName(kind)
	if total == 0 {
		fmt.Printf("No %s found in %d files.\n", label, len(docs))
		return
	}
	if total == 1 {
		label = strings.TrimSuffix(label, "s")
	}
	fmt.Printf("%s%d %s in %d of %d files%s\n", dim, total, label, files, len(docs), reset)
}

// typeFilterDoc renders one document's constructs of kind and returns how
// many there are. With skipEmpty, a document with none prints nothing.
func typeFilterDoc(doc *parser.Document, kind parser.NodeKind, lang, variant string, skipEmpty bool) int {

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
	if total == 0 && skipEmpty {
		return 0
	}
	label := kindDisplayName(kind)
	info := fmt.Sprintf("%d %s in %d section%s", total, label, len(hits), pluralS(len(hits)))
	printMiniHeader(doc.Filename+" — "+label, info)

	if len(hits) == 0 {
		fmt.Printf("No %s found.\n\n", label)
		return 0
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
	return total
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
		// The first line of code says which block this is; a list of
		// identical "bash" rows does not.
		first := ""
		for _, l := range strings.Split(v.Code, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				first = l
				break
			}
		}
		if r := []rune(first); len(r) > 72 {
			first = string(r[:71]) + "…"
		}
		if v.LineEnd() > v.LineStart() {
			return strings.TrimRight(fmt.Sprintf(":%d-%d  %-10s  %s", v.LineStart(), v.LineEnd(), lang, first), " ")
		}
		return strings.TrimRight(fmt.Sprintf(":%d     %-10s  %s", v.LineStart(), lang, first), " ")
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
	top, width := boxLine(title, width)
	fmt.Println(top)
	fmt.Println(boxInfo(info, width))
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
		if text := strings.TrimSpace(doc.SourceSpan(line, line)); text != "" {
			fmt.Printf("%d: %s\n", line, text)
		} else {
			fmt.Println("No matching node.")
		}
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
					if nodeHasChangedLine(n, changed) {
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

func nodeHasChangedLine(n parser.Node, changed map[int]bool) bool {
	end := n.LineEnd()
	if end < n.LineStart() {
		end = n.LineStart()
	}
	for line := n.LineStart(); line <= end; line++ {
		if changed[line] {
			return true
		}
	}
	return false
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

// SectionNotFound prints the not-found line plus the closest titles, so the
// next attempt does not have to start with a full tree.
func SectionNotFound(docs []*parser.Document, name string) {
	fmt.Printf("Section '%s' not found\n", name)
	var near []string
	for _, doc := range docs {
		for _, t := range doc.SuggestSections(name, 6-len(near)) {
			if len(docs) > 1 {
				t = doc.Filename + " > " + t
			}
			near = append(near, t)
		}
		if len(near) >= 6 {
			break
		}
	}
	if len(near) == 0 && len(docs) == 1 {
		// Nothing close: show the top of the outline. A lone H1 is the
		// document title, so its children are the useful level.
		top := docs[0].Sections
		if len(top) == 1 && len(top[0].Children) > 0 {
			top = top[0].Children
		}
		for _, s := range top {
			if len(near) < 8 {
				near = append(near, s.Title)
			}
		}
		if len(near) > 0 {
			fmt.Println("sections here:")
		}
	} else if len(near) > 0 {
		fmt.Println("closest:")
	}
	for _, t := range near {
		fmt.Printf("  %s\n", t)
	}
}

// noteOtherMatches tells the reader, on stderr, when name was ambiguous and
// which other sections it could have meant.
func noteOtherMatches(doc *parser.Document, name string, found []*parser.Section) {
	if len(found) < 2 || strings.EqualFold(strings.TrimSpace(found[0].Title), strings.TrimSpace(name)) &&
		!strings.EqualFold(strings.TrimSpace(found[1].Title), strings.TrimSpace(name)) {
		return
	}
	var others []string
	for _, s := range found[1:] {
		if len(others) == 4 {
			others = append(others, fmt.Sprintf("… %d more", len(found)-5))
			break
		}
		others = append(others, fmt.Sprintf("%s (:%d)", breadcrumb(s), s.LineStart))
	}
	fmt.Fprintf(os.Stderr, "note: '%s' also matches in %s: %s. Use the full title or \"Parent > Child\".\n",
		name, doc.Filename, strings.Join(others, "; "))
}

// FilteredTree shows only sections matching the filter
func FilteredTree(doc *parser.Document, filter string) {
	found := doc.FindSections(filter)
	if len(found) == 0 {
		SectionNotFound([]*parser.Document{doc}, filter)
		return
	}
	section := found[0]

	// Print mini header: title, size, where it is and what it holds.
	loc := ""
	if section.LineStart > 0 {
		loc = fmt.Sprintf(" %s:%d-%d", doc.Filename, section.LineStart, section.LineEnd)
	}
	notes := notableAnnotation(section)
	if notes != "" {
		notes = " · " + notes
	}
	fmt.Printf("%s╭── %s%s%s (%s tokens)%s%s%s\n", dim, reset, bold+cyan, section.Title, formatTokens(section.Tokens), loc, notes, reset)

	// Print children
	for i, child := range section.Children {
		isLast := i == len(section.Children)-1
		renderSection(child, "", isLast, true)
	}
	if len(section.Children) == 0 {
		fmt.Printf("%sno subsections; read it: docmap %s --expand %q%s\n", dim, doc.Filename, section.Title, reset)
	}

	fmt.Println()
	noteOtherMatches(doc, filter, found)
}

// ExpandSection prints the original markdown for a named section, including
// nested headings, with a file:line-range locator agents can jump to.
func ExpandSection(doc *parser.Document, name string) {
	found := doc.FindSections(name)
	if len(found) == 0 {
		SectionNotFound([]*parser.Document{doc}, name)
		return
	}
	printSectionSource(doc, found[0])
	noteOtherMatches(doc, name, found)
}

func printSectionSource(doc *parser.Document, section *parser.Section) {
	raw := doc.SourceSpan(section.LineStart, section.LineEnd)
	if raw == "" {
		raw = section.Content
	}
	fmt.Printf("%s:%d-%d  %s\n", doc.Filename, section.LineStart, section.LineEnd, section.Title)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println(raw)
}

// ExpandAtLine dumps the section that contains the given line.
func ExpandAtLine(doc *parser.Document, line int) {
	section := sectionAtLine(doc.Sections, line)
	if section == nil {
		fmt.Printf("No section contains line %d\n", line)
		return
	}
	printSectionSource(doc, section)
}

func sectionAtLine(sections []*parser.Section, line int) *parser.Section {
	var best *parser.Section
	var walk func([]*parser.Section)
	walk = func(ss []*parser.Section) {
		for _, s := range ss {
			if s.LineStart <= line && line <= s.LineEnd {
				best = s
				walk(s.Children)
			}
		}
	}
	walk(sections)
	return best
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
	top, innerWidth := boxLine(dirName+"/", innerWidth)
	fmt.Println(top)
	for _, line := range allLines {
		fmt.Println(boxInfo(line, innerWidth))
	}
	fmt.Printf("╰%s╯\n", strings.Repeat("─", innerWidth))
	fmt.Println()

	// Per-file one-liner with each file's own mini-summary.
	for i, doc := range docs {
		isLast := i == len(docs)-1
		renderDocSummary(doc, isLast)
	}
	fmt.Println()
	fmt.Printf("find content: docmap %s --search <term>\n", dirName)
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

// LineHit is one source line that matches a search query.
type LineHit = parser.LineHit

// SearchResult holds a matched section with its file context
type SearchResult struct {
	Filename   string
	Path       string // e.g. "endpoints > Ban Member"
	Section    *parser.Section
	TitleMatch bool
	// WholeWord is true when the query matched as a word of its own, not
	// only inside a longer one ("hub" in "github").
	WholeWord bool
	// Hits are the matching lines among the section's own lines (not its
	// subsections'), in order. Count is how many there are; Hits holds at
	// most maxHitsPerSection of them. Both are empty when the section
	// matched only by title or by a typed construct (a code language, a
	// callout kind), or when the document has no line-addressable source.
	Hits  []LineHit
	Count int
}

const (
	// maxSearchSections bounds the rendered result list. A query that hits
	// more sections than this is too broad to read; the count says so.
	maxSearchSections = 25
)

// FindSearchResults returns sections matching query using the same title,
// content, and typed-AST matching as the human renderer, in document order.
func FindSearchResults(docs []*parser.Document, query string) []SearchResult {
	var results []SearchResult
	query = strings.ToLower(query)
	for _, doc := range docs {
		searchSections(doc, doc.Sections, "", query, &results)
	}
	return results
}

// RankSearchResults orders results by usefulness: sections whose title
// matches first, then whole-word matches before matches inside a longer
// word, then by number of matching lines, keeping document order among
// equals.
func RankSearchResults(results []SearchResult) []SearchResult {
	ranked := append([]SearchResult(nil), results...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].TitleMatch != ranked[j].TitleMatch {
			return ranked[i].TitleMatch
		}
		if ranked[i].WholeWord != ranked[j].WholeWord {
			return ranked[i].WholeWord
		}
		return ranked[i].Count > ranked[j].Count
	})
	return ranked
}

// SearchResults searches all docs for sections matching the query and
// renders them best first, each with the lines that matched:
//
//	docs/HOOKS.md > Hooks > Why This Matters (93)
//	  :41  files imported by 3+ others are hub files
//
// so the reader can jump to file:line without expanding the section first.
func SearchResults(docs []*parser.Document, query string) {
	results := RankSearchResults(FindSearchResults(docs, query))
	query = strings.ToLower(query)

	if len(results) == 0 {
		fmt.Printf("No sections matching '%s'\n", query)
		return
	}

	files := map[string]bool{}
	lines := 0
	for _, r := range results {
		files[r.Filename] = true
		lines += r.Count
	}
	header := fmt.Sprintf("%d sections match '%s'", len(results), query)
	if len(results) == 1 {
		header = fmt.Sprintf("1 section matches '%s'", query)
	}
	if lines > 0 {
		header += fmt.Sprintf(" · %d line%s", lines, pluralS(lines))
	}
	if len(files) > 1 {
		header += fmt.Sprintf(" · %d files", len(files))
	}
	fmt.Printf("%s%s%s\n\n", bold, header, reset)

	shown := results
	if len(shown) > maxSearchSections {
		shown = shown[:maxSearchSections]
	}
	for _, r := range shown {
		where := r.Filename + " > "
		if r.Path != "" {
			where += r.Path + " > "
		}
		loc := ""
		if r.Section.LineStart > 0 && len(r.Hits) == 0 {
			loc = fmt.Sprintf(" :%d", r.Section.LineStart)
		}
		fmt.Printf("%s%s%s%s%s%s %s(%s)%s\n", dim, where, reset, bold+cyan, r.Section.Title, reset,
			dim, formatTokens(r.Section.Tokens), loc+reset)
		for _, h := range r.Hits {
			fmt.Printf("  %s:%d%s  %s\n", dim, h.Line, reset, h.Text)
		}
		if extra := r.Count - len(r.Hits); extra > 0 {
			fmt.Printf("  %s… %d more line%s%s\n", dim, extra, pluralS(extra), reset)
		}
	}
	if extra := len(results) - len(shown); extra > 0 {
		fmt.Printf("\n%s… %d more sections. Narrow the query or path, or list them all with --compact%s\n", dim, extra, reset)
	}
	fmt.Println()
}

// hasWholeWord reports whether query occurs in text with no letter or digit
// directly before or after it. Both must be lowercase.
func hasWholeWord(text, query string) bool {
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], query)
		if i < 0 {
			return false
		}
		at := from + i
		before, _ := utf8.DecodeLastRuneInString(text[:at])
		after, _ := utf8.DecodeRuneInString(text[at+len(query):])
		if (at == 0 || !isWord(before)) && (at+len(query) == len(text) || !isWord(after)) {
			return true
		}
		from = at + 1
	}
	return false
}

func searchSections(doc *parser.Document, sections []*parser.Section, parentPath string, query string, results *[]SearchResult) {
	for _, s := range sections {
		titleMatch := strings.Contains(strings.ToLower(s.Title), query)
		hits, count := parser.SectionLineHits(doc, s, query)
		if titleMatch || count > 0 || parser.ContainsText(s.Content, query) || notablesMatch(s, query) {
			whole := hasWholeWord(strings.ToLower(s.Title), query)
			for _, h := range hits {
				whole = whole || hasWholeWord(strings.ToLower(h.Text), query)
			}
			*results = append(*results, SearchResult{
				Filename:   doc.Filename,
				Path:       parentPath,
				Section:    s,
				TitleMatch: titleMatch,
				WholeWord:  whole,
				Hits:       hits,
				Count:      count,
			})
		}

		// Search children
		childPath := parentPath
		if childPath != "" {
			childPath += " > " + s.Title
		} else {
			childPath = s.Title
		}
		searchSections(doc, s.Children, childPath, query, results)
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

// resolveRefTarget turns a link target as written in the file from into a
// path relative to the walked root, so "../README.md" linked from
// docs/a.md and "README.md" linked from CONTRIBUTING.md are the same node.
// Targets that leave the root, are absolute, or are URLs stay as written.
func resolveRefTarget(from, target string) string {
	if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "/") {
		return target
	}
	joined := path.Clean(path.Join(path.Dir(filepath.ToSlash(from)), filepath.ToSlash(target)))
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return target
	}
	return joined
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
			to := resolveRefTarget(doc.Filename, ref.Target)
			allRefs = append(allRefs, RefInfo{
				From: doc.Filename,
				To:   to,
				Text: ref.Text,
				Line: ref.Line,
			})
			fileRefs[doc.Filename] = append(fileRefs[doc.Filename], to)
			fileRefBy[to] = append(fileRefBy[to], doc.Filename)
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
	top, innerWidth := boxLine(dirName+"/", innerWidth)
	fmt.Println(top)

	info := fmt.Sprintf("References: %d links between docs", len(allRefs))
	fmt.Println(boxInfo(info, innerWidth))
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
