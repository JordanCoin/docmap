package mcp

import (
	"strings"

	"github.com/JordanCoin/docmap/internal/docset"
	"github.com/JordanCoin/docmap/parser"
)

func documentsJSON(ld *loaded) docset.JSONOutput {
	return docset.DocumentsJSON(ld.Root, ld.Docs)
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
			"filename": doc.Filename, "tokens": doc.TotalTokens, "sections": secs, "summary": docset.ConvertSummary(sum),
		})
	}
	return map[string]any{
		"root": ld.Root, "total_docs": len(ld.Docs), "total_sections": totalSections,
		"total_tokens": totalTokens, "summary": docset.ConvertSummary(agg), "files": files,
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
		recent = docset.RecentByMtime(ld.Docs, ld.Root, 5)
	}
	return map[string]any{
		"root": ld.Root, "total_docs": len(ld.Docs), "markdown_docs": md,
		"total_sections": totalSections, "total_tokens": totalTokens,
		"recent": recent, "stale_count": staleCount, "stale_days": days,
	}
}
