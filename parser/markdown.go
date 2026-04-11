package parser

import (
	"regexp"
	"strings"
)

// Parse parses markdown content into a Document using goldmark (CommonMark +
// GFM + Obsidian extensions) and derives the legacy Section tree and
// cross-file Reference list from the resulting typed AST.
func Parse(content string) *Document {
	doc := &Document{}
	source := []byte(content)

	// Build the typed AST first; everything below is derived from it.
	doc.Nodes = parseWithGoldmark(source)

	// Derive the legacy heading-only Section tree.
	doc.Sections, doc.TotalTokens = sectionsFromNodes(doc.Nodes)

	// Collect cross-file .md links from the AST for the references view.
	doc.References = referencesFromNodes(doc.Nodes)

	return doc
}

// sectionsFromNodes flattens the top-level AST in document order, turning
// every Heading into a Section and attaching the blocks that follow it
// (until the next heading) as that section's content, notables, and stats.
func sectionsFromNodes(nodes []Node) ([]*Section, int) {
	var all []*Section
	var current *Section
	var contentBuf strings.Builder

	finalize := func(endLine int) {
		if current == nil {
			return
		}
		raw := strings.TrimSpace(contentBuf.String())
		current.Content = raw
		current.KeyTerms = extractKeyTerms(raw)
		if current.LineEnd == 0 {
			current.LineEnd = endLine
		}
		contentBuf.Reset()
	}

	for _, n := range nodes {
		if h, ok := n.(*Heading); ok {
			finalize(h.LineStart() - 1)
			current = &Section{
				Level:     h.Level,
				Title:     h.Title,
				LineStart: h.LineStart(),
				Tokens:    h.Tokens(),
			}
			all = append(all, current)
			continue
		}
		if current == nil {
			// Blocks before the first heading (frontmatter, intro paragraphs)
			// are ignored by the legacy Section view.
			continue
		}
		contentBuf.WriteString(nodeRaw(n))
		contentBuf.WriteString("\n")
		current.Tokens += n.Tokens()
		current.LineEnd = n.LineEnd()

		nots, stats := collectNotables(n)
		current.Notables = append(current.Notables, nots...)
		current.Stats.add(stats)
	}
	finalize(0)

	roots := buildTree(all)

	// Parent sections' LineEnd only reflects their own direct content,
	// stopping where the first child subsection begins. Extend LineEnd so
	// each section covers its entire subtree — this makes AtLine and
	// section-bounded searches work correctly.
	for _, r := range roots {
		extendSectionRange(r)
	}

	total := 0
	for _, s := range all {
		total += s.Tokens
	}
	return roots, total
}

// extendSectionRange recursively pulls up the deepest LineEnd from a
// section's descendants so parents enclose every child.
func extendSectionRange(s *Section) {
	for _, child := range s.Children {
		extendSectionRange(child)
		if child.LineEnd > s.LineEnd {
			s.LineEnd = child.LineEnd
		}
	}
}

// add accumulates counts from other into s.
func (s *NotableStats) add(other NotableStats) {
	s.Tasks += other.Tasks
	s.TasksChecked += other.TasksChecked
	s.WikiLinks += other.WikiLinks
	s.WikiEmbeds += other.WikiEmbeds
	s.Mentions += other.Mentions
	s.IssueRefs += other.IssueRefs
	s.CommitRefs += other.CommitRefs
	s.Emojis += other.Emojis
}

// collectNotables walks root and classifies descendants into per-instance
// notables (returned as a slice) and aggregated counts (stats). Callouts,
// tables, and definition lists are treated as atomic units: their children
// are not traversed, so a wiki link inside a callout won't be counted twice.
func collectNotables(root Node) ([]Node, NotableStats) {
	var notables []Node
	var stats NotableStats

	Walk(root, func(n Node) bool {
		switch v := n.(type) {
		case *Callout:
			notables = append(notables, v)
			return false
		case *Table:
			notables = append(notables, v)
			return false
		case *CodeBlock:
			notables = append(notables, v)
			return false
		case *MathBlock:
			notables = append(notables, v)
			return false
		case *HTMLBlock:
			notables = append(notables, v)
			return false
		case *FootnoteDef:
			notables = append(notables, v)
			return false
		case *DefinitionList:
			notables = append(notables, v)
			return false
		case *LinkRefDef:
			notables = append(notables, v)
			return false
		case *TaskItem:
			stats.Tasks++
			if v.Checked {
				stats.TasksChecked++
			}
			return true
		case *WikiLink:
			stats.WikiLinks++
			return true
		case *WikiEmbed:
			stats.WikiEmbeds++
			return true
		case *Mention:
			stats.Mentions++
			return true
		case *IssueRef:
			stats.IssueRefs++
			return true
		case *CommitRef:
			stats.CommitRefs++
			return true
		case *Emoji:
			stats.Emojis++
			return true
		}
		return true
	})
	return notables, stats
}

// referencesFromNodes walks the AST and returns every Link whose target is
// a local .md file (with any #anchor stripped).
func referencesFromNodes(nodes []Node) []Reference {
	var out []Reference
	for _, root := range nodes {
		Walk(root, func(n Node) bool {
			l, ok := n.(*Link)
			if !ok {
				return true
			}
			target := l.URL
			if idx := strings.Index(target, "#"); idx >= 0 {
				target = target[:idx]
			}
			if !strings.HasSuffix(target, ".md") {
				return true
			}
			out = append(out, Reference{
				Text:   l.Text,
				Target: target,
				Line:   l.LineStart(),
			})
			return true
		})
	}
	return out
}

// nodeRaw returns the original markdown source fragment for a node so the
// legacy key-term extractor can still scan for **bold** and `code` markers.
// For container nodes it concatenates the raw source of each descendant.
func nodeRaw(n Node) string {
	switch v := n.(type) {
	case *Paragraph:
		return v.Raw
	case *CodeBlock:
		return v.Code
	case *MathBlock:
		return v.TeX
	case *HTMLBlock:
		return v.Raw
	case *Frontmatter:
		return v.Raw
	case *Heading:
		return v.Title
	}
	// Containers: concatenate children's raw text.
	var b strings.Builder
	for _, child := range n.Children() {
		b.WriteString(nodeRaw(child))
		b.WriteString("\n")
	}
	return b.String()
}

// Document represents a parsed markdown document.
//
// The legacy Sections tree is heading-only and is what the renderer
// currently consumes. Nodes holds the richer typed AST produced by the
// new goldmark-based parser (frontmatter, tables, callouts, code blocks,
// Obsidian wiki links, etc.) once it lands. Both exist side-by-side
// during the migration so existing callers keep working.
type Document struct {
	Filename    string
	TotalTokens int
	Sections    []*Section
	References  []Reference // Links to other .md files
	Nodes       []Node      // Typed AST (populated by the new parser)
}

// Reference represents a link to another markdown file
type Reference struct {
	Text   string // Link text
	Target string // Target file path
	Line   int    // Line number where reference appears
}

// Section represents a heading and the content that follows it up to the
// next same-or-higher heading.
//
// Notables lists the discoverable block nodes inside this section that an
// agent might want to jump to directly — callouts, tables, code blocks,
// math blocks, HTML blocks, footnote definitions, definition lists. It is
// populated one-per-instance so the renderer can print each as its own line.
//
// Stats aggregates counts of things that would be noisy per-instance
// (task list items, Obsidian wiki links, Obsidian embeds). They are
// rendered as a single summary line per section.
type Section struct {
	Level     int // 1 = #, 2 = ##, etc.
	Title     string
	Content   string   // raw content (excluding children)
	Tokens    int      // estimated tokens for this section
	KeyTerms  []string // extracted key concepts
	Children  []*Section
	Parent    *Section
	LineStart int
	LineEnd   int
	Notables  []Node
	Stats     NotableStats
}

// NotableStats aggregates counts of constructs that would be noisy if
// listed per-instance under a section.
type NotableStats struct {
	Tasks        int
	TasksChecked int
	WikiLinks    int
	WikiEmbeds   int
	Mentions     int
	IssueRefs    int
	CommitRefs   int
	Emojis       int
}

// Token estimation: ~4 chars per token (rough approximation)
func estimateTokens(s string) int {
	return len(s) / 4
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

// Summary walks the typed AST once and returns inventory counts for every
// notable construct in the document. Used by the renderer to draw the
// file header's content summary.
func (d *Document) Summary() ContentSummary {
	var s ContentSummary
	for _, root := range d.Nodes {
		Walk(root, func(n Node) bool {
			switch v := n.(type) {
			case *Callout:
				s.Callouts++
				return false
			case *Table:
				s.Tables++
				return false
			case *CodeBlock:
				s.CodeBlocks++
				return false
			case *MathBlock:
				s.MathBlocks++
				return false
			case *HTMLBlock:
				raw := strings.TrimSpace(v.Raw)
				if !strings.HasPrefix(raw, "<!--") && !strings.HasPrefix(raw, "</") {
					s.HTMLBlocks++
				}
				return false
			case *FootnoteDef:
				s.Footnotes++
				return false
			case *DefinitionList:
				s.DefLists++
				return false
			case *LinkRefDef:
				s.LinkRefDefs++
				return false
			case *TaskItem:
				s.Tasks++
				if v.Checked {
					s.TasksChecked++
				}
			case *WikiLink:
				s.WikiLinks++
			case *WikiEmbed:
				s.WikiEmbeds++
			case *Mention:
				s.Mentions++
			case *IssueRef:
				s.IssueRefs++
			case *CommitRef:
				s.CommitRefs++
			case *Emoji:
				s.Emojis++
			}
			return true
		})
	}
	return s
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
