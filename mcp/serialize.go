package mcp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JordanCoin/docmap/parser"
)

type jsonOutput struct {
	Root        string         `json:"root"`
	Since       string         `json:"since,omitempty"`
	TotalTokens int            `json:"total_tokens"`
	TotalDocs   int            `json:"total_docs"`
	Documents   []jsonDocument `json:"documents"`
}

type jsonDocument struct {
	Filename     string        `json:"filename"`
	Change       string        `json:"change,omitempty"`
	OldPath      string        `json:"old_path,omitempty"`
	ChangedLines []int         `json:"changed_lines,omitempty"`
	Tokens       int           `json:"tokens"`
	Summary      jsonSummary   `json:"summary"`
	Sections     []jsonSection `json:"sections"`
	Nodes        []jsonNode    `json:"nodes,omitempty"`
	References   []jsonRef     `json:"references,omitempty"`
}

type jsonSummary struct {
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

type jsonSection struct {
	Level     int           `json:"level"`
	Title     string        `json:"title"`
	Tokens    int           `json:"tokens"`
	LineStart int           `json:"line_start,omitempty"`
	LineEnd   int           `json:"line_end,omitempty"`
	KeyTerms  []string      `json:"key_terms,omitempty"`
	Notables  []jsonNode    `json:"notables,omitempty"`
	Children  []jsonSection `json:"children,omitempty"`
}

type jsonNode struct {
	Kind      string     `json:"kind"`
	LineStart int        `json:"line_start,omitempty"`
	LineEnd   int        `json:"line_end,omitempty"`
	Tokens    int        `json:"tokens,omitempty"`
	Title     string     `json:"title,omitempty"`
	Level     int        `json:"level,omitempty"`
	Language  string     `json:"language,omitempty"`
	Code      string     `json:"code,omitempty"`
	Variant   string     `json:"variant,omitempty"`
	Headers   []string   `json:"headers,omitempty"`
	Aligns    []string   `json:"aligns,omitempty"`
	TeX       string     `json:"tex,omitempty"`
	ID        string     `json:"id,omitempty"`
	Label     string     `json:"label,omitempty"`
	URL       string     `json:"url,omitempty"`
	Checked   *bool      `json:"checked,omitempty"`
	Raw       string     `json:"raw,omitempty"`
	Format    string     `json:"format,omitempty"`
	Target    string     `json:"target,omitempty"`
	Children  []jsonNode `json:"children,omitempty"`
}

type jsonRef struct {
	Text   string `json:"text"`
	Target string `json:"target"`
	Line   int    `json:"line"`
}

func convertSummary(s parser.ContentSummary) jsonSummary {
	return jsonSummary{
		Callouts: s.Callouts, Tables: s.Tables, CodeBlocks: s.CodeBlocks,
		MathBlocks: s.MathBlocks, HTMLBlocks: s.HTMLBlocks, Footnotes: s.Footnotes,
		DefLists: s.DefLists, LinkRefDefs: s.LinkRefDefs, Tasks: s.Tasks,
		TasksChecked: s.TasksChecked, WikiLinks: s.WikiLinks, WikiEmbeds: s.WikiEmbeds,
		Mentions: s.Mentions, IssueRefs: s.IssueRefs, CommitRefs: s.CommitRefs, Emojis: s.Emojis,
	}
}

func convertNode(n parser.Node) jsonNode {
	j := jsonNode{
		Kind: string(n.Kind()), LineStart: n.LineStart(), LineEnd: n.LineEnd(), Tokens: n.Tokens(),
	}
	switch v := n.(type) {
	case *parser.Heading:
		j.Title, j.Level = v.Title, v.Level
	case *parser.CodeBlock:
		j.Language, j.Code = v.Language, v.Code
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
		j.Label, j.URL = v.Label, v.URL
	case *parser.Link:
		j.URL, j.Title = v.URL, v.Text
	case *parser.TaskItem:
		checked := v.Checked
		j.Checked = &checked
	case *parser.HTMLBlock:
		j.Raw = v.Raw
	case *parser.Frontmatter:
		j.Raw, j.Format = v.Raw, string(v.Format)
	case *parser.WikiLink:
		j.Target = v.Target
	case *parser.WikiEmbed:
		j.Target = v.Target
	}
	for _, c := range n.Children() {
		j.Children = append(j.Children, convertNode(c))
	}
	return j
}

func convertNodeList(nodes []parser.Node) []jsonNode {
	out := make([]jsonNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, convertNode(n))
	}
	return out
}

func convertSections(sections []*parser.Section) []jsonSection {
	out := make([]jsonSection, 0, len(sections))
	for _, s := range sections {
		out = append(out, jsonSection{
			Level: s.Level, Title: s.Title, Tokens: s.Tokens,
			LineStart: s.LineStart, LineEnd: s.LineEnd, KeyTerms: s.KeyTerms,
			Notables: convertNodeList(s.Notables), Children: convertSections(s.Children),
		})
	}
	return out
}

func convertSectionsSince(sections []*parser.Section, changed map[int]bool) []jsonSection {
	var out []jsonSection
	for _, s := range sections {
		children := convertSectionsSince(s.Children, changed)
		if !sectionTouches(s, changed) && len(children) == 0 {
			continue
		}
		var notables []parser.Node
		for _, n := range s.Notables {
			if nodeTouches(n, changed) {
				notables = append(notables, n)
			}
		}
		out = append(out, jsonSection{
			Level: s.Level, Title: s.Title, Tokens: s.Tokens,
			LineStart: s.LineStart, LineEnd: s.LineEnd, KeyTerms: s.KeyTerms,
			Notables: convertNodeList(notables), Children: children,
		})
	}
	return out
}

func convertNodesSince(nodes []parser.Node, changed map[int]bool) []jsonNode {
	var out []jsonNode
	for _, n := range nodes {
		if nodeTouches(n, changed) {
			out = append(out, convertNode(n))
		}
	}
	return out
}

func sectionTouches(s *parser.Section, changed map[int]bool) bool {
	for line := s.LineStart; line <= s.LineEnd; line++ {
		if changed[line] {
			return true
		}
	}
	return false
}

func nodeTouches(n parser.Node, changed map[int]bool) bool {
	end := n.LineEnd()
	if end == 0 {
		end = n.LineStart()
	}
	for line := n.LineStart(); line <= end; line++ {
		if changed[line] {
			return true
		}
	}
	return false
}

func documentsJSON(ld *loaded) jsonOutput {
	out := jsonOutput{Root: ld.Root, TotalDocs: len(ld.Docs)}
	for _, doc := range ld.Docs {
		jd := jsonDocument{
			Filename: doc.Filename, Tokens: doc.TotalTokens,
			Summary: convertSummary(doc.Summary()), Sections: convertSections(doc.Sections),
			Nodes: convertNodeList(doc.Nodes),
		}
		for _, ref := range doc.References {
			jd.References = append(jd.References, jsonRef{Text: ref.Text, Target: ref.Target, Line: ref.Line})
		}
		out.Documents = append(out.Documents, jd)
		out.TotalTokens += doc.TotalTokens
	}
	return out
}

func inventoryJSON(ld *loaded) map[string]any {
	files := make([]map[string]any, 0, len(ld.Docs))
	agg := parser.ContentSummary{}
	totalSections, totalTokens := 0, 0
	for _, doc := range ld.Docs {
		sum := doc.Summary()
		agg.Callouts += sum.Callouts
		agg.Tables += sum.Tables
		agg.CodeBlocks += sum.CodeBlocks
		agg.MathBlocks += sum.MathBlocks
		agg.HTMLBlocks += sum.HTMLBlocks
		agg.Footnotes += sum.Footnotes
		agg.DefLists += sum.DefLists
		agg.LinkRefDefs += sum.LinkRefDefs
		agg.Tasks += sum.Tasks
		agg.TasksChecked += sum.TasksChecked
		agg.WikiLinks += sum.WikiLinks
		agg.WikiEmbeds += sum.WikiEmbeds
		agg.Mentions += sum.Mentions
		agg.IssueRefs += sum.IssueRefs
		agg.CommitRefs += sum.CommitRefs
		agg.Emojis += sum.Emojis
		secs := len(doc.GetAllSections())
		totalSections += secs
		totalTokens += doc.TotalTokens
		files = append(files, map[string]any{
			"filename": doc.Filename, "tokens": doc.TotalTokens, "sections": secs, "summary": convertSummary(sum),
		})
	}
	return map[string]any{
		"root": ld.Root, "total_docs": len(ld.Docs), "total_sections": totalSections,
		"total_tokens": totalTokens, "summary": convertSummary(agg), "files": files,
	}
}

func briefJSON(ld *loaded, days, staleCount int) map[string]any {
	totalSections, totalTokens, md := 0, 0, 0
	for _, d := range ld.Docs {
		totalTokens += d.TotalTokens
		totalSections += len(d.GetAllSections())
		if strings.HasSuffix(strings.ToLower(d.Filename), ".md") {
			md++
		}
	}
	recent := parser.RecentFiles(ld.Root, 5)
	if len(recent) == 0 {
		recent = recentByMtime(ld.Docs, ld.Root, 5)
	}
	return map[string]any{
		"root": ld.Root, "total_docs": len(ld.Docs), "markdown_docs": md,
		"total_sections": totalSections, "total_tokens": totalTokens,
		"recent": recent, "stale_count": staleCount, "stale_days": days,
	}
}

func recentByMtime(docs []*parser.Document, root string, limit int) []string {
	type rec struct {
		name string
		mod  int64
	}
	var rows []rec
	for _, d := range docs {
		info, err := os.Stat(filepath.Join(root, d.Filename))
		if err != nil {
			continue
		}
		rows = append(rows, rec{d.Filename, info.ModTime().UnixNano()})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].mod > rows[j].mod })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.name
	}
	return out
}
