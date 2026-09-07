package mcp

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JordanCoin/docmap/internal/docset"
	"github.com/JordanCoin/docmap/parser"
	"github.com/JordanCoin/docmap/stale"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type pathInput struct {
	Path string `json:"path" jsonschema:"File or directory path to analyze"`
}

type briefInput struct {
	Path string `json:"path" jsonschema:"File or directory path"`
	Days int    `json:"days,omitempty" jsonschema:"Status-date freshness window in days (default 90)"`
}

type sectionInput struct {
	Path    string `json:"path" jsonschema:"File or directory path"`
	Section string `json:"section" jsonschema:"Section title to match (substring, case-insensitive)"`
}

type findByTypeInput struct {
	Path    string `json:"path" jsonschema:"File or directory path"`
	Kind    string `json:"kind" jsonschema:"Construct kind: code, callout, table, math, footnote, deflist, linkref, html, task, wiki, embed, mention, issue, sha, emoji"`
	Lang    string `json:"lang,omitempty" jsonschema:"Optional language filter for code blocks"`
	Variant string `json:"variant,omitempty" jsonschema:"Optional callout variant filter (note, tip, warning, …)"`
}

type atLineInput struct {
	Path string `json:"path" jsonschema:"Markdown/PDF/YAML file path"`
	Line int    `json:"line" jsonschema:"1-indexed line number"`
}

type sinceInput struct {
	Path string `json:"path" jsonschema:"File or directory path"`
	Ref  string `json:"ref" jsonschema:"Git ref to diff against (e.g. HEAD, main, HEAD~3)"`
}

type staleInput struct {
	Path         string   `json:"path" jsonschema:"File or directory path"`
	Days         int      `json:"days,omitempty" jsonschema:"Status-date freshness window in days (default 90)"`
	CheckFlags   bool     `json:"check_flags,omitempty" jsonschema:"Verify backticked CLI flags against binary --help"`
	Remote       bool     `json:"remote,omitempty" jsonschema:"HEAD-check URLs and compare config/version values"`
	AllowDomains []string `json:"allow_domains,omitempty" jsonschema:"When remote=true, only check these domains"`
}

type searchInput struct {
	Path  string `json:"path" jsonschema:"File or directory path"`
	Query string `json:"query" jsonschema:"Search query matched against titles, content, languages, variants, headers"`
}

func handleInventory(_ context.Context, _ *mcpsdk.CallToolRequest, in pathInput) (*mcpsdk.CallToolResult, any, error) {
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	return jsonResult(inventoryJSON(ld))
}

func handleTree(_ context.Context, _ *mcpsdk.CallToolRequest, in pathInput) (*mcpsdk.CallToolResult, any, error) {
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	docs := make([]map[string]any, 0, len(ld.Docs))
	for _, doc := range ld.Docs {
		docs = append(docs, map[string]any{
			"filename": doc.Filename,
			"tokens":   doc.TotalTokens,
			"summary":  docset.ConvertSummary(doc.Summary()),
			"sections": docset.ConvertSections(doc.Sections),
		})
	}
	return jsonResult(map[string]any{"root": ld.Root, "documents": docs})
}

func handleBrief(_ context.Context, _ *mcpsdk.CallToolRequest, in briefInput) (*mcpsdk.CallToolResult, any, error) {
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	findings := stale.Check(ld.Docs, stale.Options{Root: ld.Root, Days: in.Days})
	return jsonResult(briefJSON(ld, in.Days, len(findings)))
}

func handleSection(_ context.Context, _ *mcpsdk.CallToolRequest, in sectionInput) (*mcpsdk.CallToolResult, any, error) {
	if strings.TrimSpace(in.Section) == "" {
		return toolErr("section is required")
	}
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	var hits []map[string]any
	for _, doc := range ld.Docs {
		sec := doc.GetSection(in.Section)
		if sec == nil {
			continue
		}
		hits = append(hits, map[string]any{
			"filename": doc.Filename,
			"section":  docset.ConvertSections([]*parser.Section{sec})[0],
		})
	}
	if len(hits) == 0 {
		return toolErr("section %q not found", in.Section)
	}
	return jsonResult(map[string]any{"root": ld.Root, "matches": hits})
}

func handleExpand(_ context.Context, _ *mcpsdk.CallToolRequest, in sectionInput) (*mcpsdk.CallToolResult, any, error) {
	if strings.TrimSpace(in.Section) == "" {
		return toolErr("section is required")
	}
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	var hits []map[string]any
	for _, doc := range ld.Docs {
		sec := doc.GetSection(in.Section)
		if sec == nil {
			continue
		}
		hits = append(hits, map[string]any{
			"filename":   doc.Filename,
			"title":      sec.Title,
			"line_start": sec.LineStart,
			"line_end":   sec.LineEnd,
			"content":    doc.SourceSpan(sec.LineStart, sec.LineEnd),
		})
	}
	if len(hits) == 0 {
		return toolErr("section %q not found", in.Section)
	}
	return jsonResult(map[string]any{"root": ld.Root, "matches": hits})
}

func handleFindByType(_ context.Context, _ *mcpsdk.CallToolRequest, in findByTypeInput) (*mcpsdk.CallToolResult, any, error) {
	kind, ok := resolveKindName(in.Kind)
	if !ok {
		return toolErr("unknown kind %q (try code, callout, table, math, …)", in.Kind)
	}
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	var hits []map[string]any
	for _, doc := range ld.Docs {
		var walk func([]*parser.Section)
		walk = func(sections []*parser.Section) {
			for _, s := range sections {
				for _, n := range s.Notables {
					if n.Kind() != kind || !matchesSubFilter(n, in.Lang, in.Variant) {
						continue
					}
					hits = append(hits, map[string]any{
						"filename": doc.Filename,
						"section":  sectionBreadcrumb(s),
						"node":     docset.ConvertNode(n),
					})
				}
				walk(s.Children)
			}
		}
		walk(doc.Sections)
	}
	return jsonResult(map[string]any{"root": ld.Root, "kind": string(kind), "count": len(hits), "hits": hits})
}

func handleAtLine(_ context.Context, _ *mcpsdk.CallToolRequest, in atLineInput) (*mcpsdk.CallToolResult, any, error) {
	if in.Line < 1 {
		return toolErr("line must be >= 1")
	}
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	if len(ld.Docs) != 1 {
		return toolErr("at_line requires a single file path")
	}
	doc := ld.Docs[0]
	var found parser.Node
	var findDeepest func([]parser.Node)
	findDeepest = func(nodes []parser.Node) {
		for _, n := range nodes {
			if n.LineStart() <= in.Line && (n.LineEnd() == 0 || n.LineEnd() >= in.Line) {
				found = n
			}
			findDeepest(n.Children())
		}
	}
	findDeepest(doc.Nodes)

	var containing *parser.Section
	var walk func([]*parser.Section)
	walk = func(sections []*parser.Section) {
		for _, s := range sections {
			if s.LineStart <= in.Line && (s.LineEnd == 0 || s.LineEnd >= in.Line) {
				containing = s
				walk(s.Children)
			}
		}
	}
	walk(doc.Sections)

	out := map[string]any{"filename": doc.Filename, "line": in.Line}
	if containing != nil {
		out["section"] = sectionBreadcrumb(containing)
		out["section_title"] = containing.Title
		out["section_lines"] = []int{containing.LineStart, containing.LineEnd}
	}
	if found != nil {
		out["node"] = docset.ConvertNode(found)
	}
	return jsonResult(out)
}

func handleSince(_ context.Context, _ *mcpsdk.CallToolRequest, in sinceInput) (*mcpsdk.CallToolResult, any, error) {
	if strings.TrimSpace(in.Ref) == "" {
		return toolErr("ref is required")
	}
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	if err := parser.EnsureRef(ld.Root, in.Ref); err != nil {
		return toolErr("%v", err)
	}
	status := map[string]parser.PathChange{}
	if changes, err := parser.ChangedPaths(ld.Root, in.Ref); err == nil {
		for _, c := range changes {
			status[c.Path] = c
		}
	}
	paths := make([]string, len(ld.Docs))
	for i, doc := range ld.Docs {
		paths[i] = filepath.Join(ld.Root, doc.Filename)
	}
	batch, err := parser.ChangedLinesBatch(ld.Root, in.Ref, paths)
	if err != nil {
		return toolErr("%v", err)
	}
	out := docset.JSONOutput{Root: ld.Root, Since: in.Ref}
	seen := map[string]bool{}
	for _, doc := range ld.Docs {
		path := filepath.Join(ld.Root, doc.Filename)
		changed := batch[path]
		if len(changed) == 0 {
			continue
		}
		ch, oldPath := "M", ""
		if pc, ok := status[filepath.ToSlash(doc.Filename)]; ok {
			ch, oldPath = pc.Status, pc.OldPath
		}
		out.Documents = append(out.Documents, docset.JSONDocument{
			Filename: doc.Filename, Change: ch, OldPath: oldPath,
			ChangedLines: sortedLines(changed), Tokens: doc.TotalTokens,
			Summary:  docset.ConvertSummary(doc.Summary()),
			Sections: docset.ConvertSectionsSince(doc.Sections, changed),
			Nodes:    docset.ConvertNodesSince(doc.Nodes, changed),
		})
		out.TotalTokens += doc.TotalTokens
		seen[filepath.ToSlash(doc.Filename)] = true
	}
	if changes, err := parser.ChangedPaths(ld.Root, in.Ref); err == nil {
		for _, c := range changes {
			switch {
			case c.Status == "D":
				out.Documents = append(out.Documents, docset.JSONDocument{Filename: c.Path, Change: "D"})
			case (c.Status == "R" || c.Status == "C") && c.OldPath != "" && !seen[c.Path]:
				out.Documents = append(out.Documents, docset.JSONDocument{Filename: c.Path, Change: c.Status, OldPath: c.OldPath})
			}
		}
	}
	out.TotalDocs = len(out.Documents)
	return jsonResult(out)
}

func handleStale(_ context.Context, _ *mcpsdk.CallToolRequest, in staleInput) (*mcpsdk.CallToolResult, any, error) {
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	findings := stale.Check(ld.Docs, stale.Options{
		Root: ld.Root, Days: in.Days, CheckFlags: in.CheckFlags,
		Remote: in.Remote, AllowDomains: in.AllowDomains,
	})
	if findings == nil {
		findings = []stale.Finding{}
	}
	return jsonResult(map[string]any{"root": ld.Root, "count": len(findings), "findings": findings})
}

func handleSearch(_ context.Context, _ *mcpsdk.CallToolRequest, in searchInput) (*mcpsdk.CallToolResult, any, error) {
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return toolErr("query is required")
	}
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	needle := strings.ToLower(q)
	var hits []map[string]any
	for _, doc := range ld.Docs {
		for _, sec := range doc.GetAllSections() {
			if sectionMatches(sec, needle) {
				hits = append(hits, map[string]any{
					"filename": doc.Filename, "section": sectionBreadcrumb(sec),
					"title": sec.Title, "tokens": sec.Tokens,
					"lines": []int{sec.LineStart, sec.LineEnd},
				})
			}
		}
	}
	return jsonResult(map[string]any{"root": ld.Root, "query": q, "count": len(hits), "hits": hits})
}

func handleJSON(_ context.Context, _ *mcpsdk.CallToolRequest, in pathInput) (*mcpsdk.CallToolResult, any, error) {
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	return jsonResult(documentsJSON(ld))
}

func handleRefs(_ context.Context, _ *mcpsdk.CallToolRequest, in pathInput) (*mcpsdk.CallToolResult, any, error) {
	ld, err := loadPath(in.Path)
	if err != nil {
		return toolErr("%v", err)
	}
	var edges []map[string]any
	for _, doc := range ld.Docs {
		for _, ref := range doc.References {
			edges = append(edges, map[string]any{
				"from": doc.Filename, "to": ref.Target, "text": ref.Text, "line": ref.Line,
			})
		}
	}
	return jsonResult(map[string]any{"root": ld.Root, "count": len(edges), "refs": edges})
}

func sectionBreadcrumb(s *parser.Section) string {
	var parts []string
	for cur := s; cur != nil; cur = cur.Parent {
		parts = append([]string{cur.Title}, parts...)
	}
	return strings.Join(parts, " > ")
}

func sectionMatches(sec *parser.Section, needle string) bool {
	if strings.Contains(strings.ToLower(sec.Title), needle) || strings.Contains(strings.ToLower(sec.Content), needle) {
		return true
	}
	for _, t := range sec.KeyTerms {
		if strings.Contains(strings.ToLower(t), needle) {
			return true
		}
	}
	for _, n := range sec.Notables {
		if nodeMatches(n, needle) {
			return true
		}
	}
	return false
}

func nodeMatches(n parser.Node, needle string) bool {
	if strings.Contains(strings.ToLower(string(n.Kind())), needle) {
		return true
	}
	switch v := n.(type) {
	case *parser.CodeBlock:
		return strings.Contains(strings.ToLower(v.Language), needle) || strings.Contains(strings.ToLower(v.Code), needle)
	case *parser.Callout:
		return strings.Contains(strings.ToLower(string(v.Variant)), needle)
	case *parser.Table:
		for _, h := range v.Headers {
			if strings.Contains(strings.ToLower(h), needle) {
				return true
			}
		}
	case *parser.MathBlock:
		return strings.Contains(strings.ToLower(v.TeX), needle)
	}
	return false
}

func matchesSubFilter(n parser.Node, lang, variant string) bool {
	if lang != "" {
		if cb, ok := n.(*parser.CodeBlock); ok && !strings.EqualFold(cb.Language, lang) {
			return false
		}
	}
	if variant != "" {
		if c, ok := n.(*parser.Callout); ok && !strings.EqualFold(string(c.Variant), variant) {
			return false
		}
	}
	return true
}

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

func sortedLines(changed map[int]bool) []int {
	out := make([]int, 0, len(changed))
	for line := range changed {
		out = append(out, line)
	}
	sort.Ints(out)
	return out
}
