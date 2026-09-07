package docset

import "github.com/JordanCoin/docmap/parser"

// JSONOutput is the top-level --json / MCP documents payload.
type JSONOutput struct {
	Root        string         `json:"root"`
	Since       string         `json:"since,omitempty"`
	TotalTokens int            `json:"total_tokens"`
	TotalDocs   int            `json:"total_docs"`
	Documents   []JSONDocument `json:"documents"`
}

// JSONDocument is one file in a JSONOutput.
type JSONDocument struct {
	Filename     string        `json:"filename"`
	Change       string        `json:"change,omitempty"`
	OldPath      string        `json:"old_path,omitempty"`
	ChangedLines []int         `json:"changed_lines,omitempty"`
	Tokens       int           `json:"tokens"`
	Summary      JSONSummary   `json:"summary"`
	Sections     []JSONSection `json:"sections"`
	Nodes        []JSONNode    `json:"nodes,omitempty"`
	References   []JSONRef     `json:"references,omitempty"`
}

// JSONSummary mirrors parser.ContentSummary for JSON consumers.
type JSONSummary struct {
	Callouts     int `json:"callouts,omitempty"`
	Tables       int `json:"tables,omitempty"`
	CodeBlocks   int `json:"code_blocks,omitempty"`
	MathBlocks   int `json:"math_blocks,omitempty"`
	HTMLBlocks   int `json:"html_blocks,omitempty"`
	Footnotes    int `json:"footnotes,omitempty"`
	DefLists     int `json:"definition_lists,omitempty"`
	LinkRefDefs  int `json:"link_ref_defs,omitempty"`
	Tasks        int `json:"tasks,omitempty"`
	TasksChecked int `json:"tasks_checked,omitempty"`
	WikiLinks    int `json:"wiki_links,omitempty"`
	WikiEmbeds   int `json:"wiki_embeds,omitempty"`
	Mentions     int `json:"mentions,omitempty"`
	IssueRefs    int `json:"issue_refs,omitempty"`
	CommitRefs   int `json:"commit_refs,omitempty"`
	Emojis       int `json:"emojis,omitempty"`
}

// JSONSection is a section tree node for JSON output.
type JSONSection struct {
	Level     int           `json:"level"`
	Title     string        `json:"title"`
	Tokens    int           `json:"tokens"`
	LineStart int           `json:"line_start,omitempty"`
	LineEnd   int           `json:"line_end,omitempty"`
	KeyTerms  []string      `json:"key_terms,omitempty"`
	Notables  []JSONNode    `json:"notables,omitempty"`
	Children  []JSONSection `json:"children,omitempty"`
}

// JSONNode is the typed-AST-aware serialization format. Kind identifies
// the node type (e.g. "code_block", "callout"); remaining fields are
// populated per kind. Agents can switch on Kind to deserialize.
type JSONNode struct {
	Kind      string     `json:"kind"`
	LineStart int        `json:"line_start,omitempty"`
	LineEnd   int        `json:"line_end,omitempty"`
	Tokens    int        `json:"tokens,omitempty"`
	Title     string     `json:"title,omitempty"`    // Heading
	Level     int        `json:"level,omitempty"`    // Heading
	Language  string     `json:"language,omitempty"` // CodeBlock
	Code      string     `json:"code,omitempty"`     // CodeBlock
	Variant   string     `json:"variant,omitempty"`  // Callout
	Headers   []string   `json:"headers,omitempty"`  // Table
	Aligns    []string   `json:"aligns,omitempty"`   // Table
	TeX       string     `json:"tex,omitempty"`      // MathBlock / InlineMath
	ID        string     `json:"id,omitempty"`       // FootnoteDef
	Label     string     `json:"label,omitempty"`    // LinkRefDef
	URL       string     `json:"url,omitempty"`      // LinkRefDef / Link
	Checked   *bool      `json:"checked,omitempty"`  // TaskItem
	Raw       string     `json:"raw,omitempty"`      // HTMLBlock / Frontmatter
	Format    string     `json:"format,omitempty"`   // Frontmatter
	Target    string     `json:"target,omitempty"`   // WikiLink / WikiEmbed
	Children  []JSONNode `json:"children,omitempty"`
}

// JSONRef is a cross-document reference edge.
type JSONRef struct {
	Text   string `json:"text"`
	Target string `json:"target"`
	Line   int    `json:"line"`
}

// ConvertSummary maps a ContentSummary into JSONSummary.
func ConvertSummary(s parser.ContentSummary) JSONSummary {
	return JSONSummary{
		Callouts:     s.Callouts,
		Tables:       s.Tables,
		CodeBlocks:   s.CodeBlocks,
		MathBlocks:   s.MathBlocks,
		HTMLBlocks:   s.HTMLBlocks,
		Footnotes:    s.Footnotes,
		DefLists:     s.DefLists,
		LinkRefDefs:  s.LinkRefDefs,
		Tasks:        s.Tasks,
		TasksChecked: s.TasksChecked,
		WikiLinks:    s.WikiLinks,
		WikiEmbeds:   s.WikiEmbeds,
		Mentions:     s.Mentions,
		IssueRefs:    s.IssueRefs,
		CommitRefs:   s.CommitRefs,
		Emojis:       s.Emojis,
	}
}

// ConvertSections serializes a section tree.
func ConvertSections(sections []*parser.Section) []JSONSection {
	var result []JSONSection
	for _, s := range sections {
		result = append(result, JSONSection{
			Level:     s.Level,
			Title:     s.Title,
			Tokens:    s.Tokens,
			LineStart: s.LineStart,
			LineEnd:   s.LineEnd,
			KeyTerms:  s.KeyTerms,
			Notables:  ConvertNodeList(s.Notables),
			Children:  ConvertSections(s.Children),
		})
	}
	return result
}

// ConvertSectionsSince keeps sections that touch changed lines (or have
// children that do).
func ConvertSectionsSince(sections []*parser.Section, changed map[int]bool) []JSONSection {
	var result []JSONSection
	for _, s := range sections {
		kids := ConvertSectionsSince(s.Children, changed)
		hit := false
		for line := s.LineStart; line <= s.LineEnd; line++ {
			if changed[line] {
				hit = true
				break
			}
		}
		if !hit && len(kids) == 0 {
			continue
		}
		result = append(result, JSONSection{
			Level:     s.Level,
			Title:     s.Title,
			Tokens:    s.Tokens,
			LineStart: s.LineStart,
			LineEnd:   s.LineEnd,
			KeyTerms:  s.KeyTerms,
			Notables:  ConvertNodesSince(s.Notables, changed),
			Children:  kids,
		})
	}
	return result
}

// ConvertNodeList serializes a flat node list.
func ConvertNodeList(nodes []parser.Node) []JSONNode {
	var out []JSONNode
	for _, n := range nodes {
		out = append(out, ConvertNode(n))
	}
	return out
}

// ConvertNodesSince keeps nodes whose line range intersects changed.
func ConvertNodesSince(nodes []parser.Node, changed map[int]bool) []JSONNode {
	var out []JSONNode
	for _, n := range nodes {
		end := n.LineEnd()
		if end < n.LineStart() {
			end = n.LineStart()
		}
		hit := false
		for line := n.LineStart(); line <= end; line++ {
			if changed[line] {
				hit = true
				break
			}
		}
		if hit {
			out = append(out, ConvertNode(n))
		}
	}
	return out
}

// ConvertNode serializes one typed AST node into JSON-friendly form.
// Only fields relevant to the kind are populated; omitempty keeps the
// output compact.
func ConvertNode(n parser.Node) JSONNode {
	j := JSONNode{
		Kind:      string(n.Kind()),
		LineStart: n.LineStart(),
		LineEnd:   n.LineEnd(),
		Tokens:    n.Tokens(),
	}
	switch v := n.(type) {
	case *parser.Heading:
		j.Title = v.Title
		j.Level = v.Level
	case *parser.CodeBlock:
		j.Language = v.Language
		j.Code = v.Code
	case *parser.Callout:
		j.Variant = string(v.Variant)
	case *parser.Table:
		j.Headers = v.Headers
		for _, a := range v.Aligns {
			j.Aligns = append(j.Aligns, string(a))
		}
	case *parser.MathBlock:
		j.TeX = v.TeX
	case *parser.InlineMath:
		j.TeX = v.TeX
	case *parser.FootnoteDef:
		j.ID = v.ID
	case *parser.LinkRefDef:
		j.Label = v.Label
		j.URL = v.URL
	case *parser.Link:
		j.URL = v.URL
		j.Title = v.Text
	case *parser.TaskItem:
		checked := v.Checked
		j.Checked = &checked
	case *parser.HTMLBlock:
		j.Raw = v.Raw
	case *parser.Frontmatter:
		j.Raw = v.Raw
		j.Format = string(v.Format)
	case *parser.WikiLink:
		j.Target = v.Target
	case *parser.WikiEmbed:
		j.Target = v.Target
	}
	for _, c := range n.Children() {
		j.Children = append(j.Children, ConvertNode(c))
	}
	return j
}

// DocumentsJSON builds a full JSONOutput for a set of loaded documents.
func DocumentsJSON(root string, docs []*parser.Document) JSONOutput {
	out := JSONOutput{Root: root, TotalDocs: len(docs)}
	for _, doc := range docs {
		jd := JSONDocument{
			Filename: doc.Filename,
			Tokens:   doc.TotalTokens,
			Summary:  ConvertSummary(doc.Summary()),
			Sections: ConvertSections(doc.Sections),
			Nodes:    ConvertNodeList(doc.Nodes),
		}
		for _, ref := range doc.References {
			jd.References = append(jd.References, JSONRef{
				Text: ref.Text, Target: ref.Target, Line: ref.Line,
			})
		}
		out.Documents = append(out.Documents, jd)
		out.TotalTokens += doc.TotalTokens
	}
	return out
}
