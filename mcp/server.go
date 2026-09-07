package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is reported in the MCP initialize handshake. main sets this from its version var.
var Version = "dev"

// Run starts the docmap MCP server on stdio until the client disconnects.
func Run(ctx context.Context) error {
	return NewServer().Run(ctx, &mcpsdk.StdioTransport{})
}

// NewServer builds an MCP server with all docmap tools registered.
func NewServer() *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "docmap",
		Version: Version,
	}, nil)

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__inventory",
		Description: "ContentSummary inventory for a file or directory of docs",
	}, handleInventory)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__tree",
		Description: "Typed section tree with notable annotations for a file or directory",
	}, handleTree)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__brief",
		Description: "Session-start digest: file/section/token counts, recent docs, stale claim count",
	}, handleBrief)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__section",
		Description: "One named section's subtree and notables",
	}, handleSection)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__expand",
		Description: "Raw source content of a named section (including children)",
	}, handleExpand)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__find_by_type",
		Description: "Typed node list filtered by kind (code, callout, table, …) with optional lang/variant",
	}, handleFindByType)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__at_line",
		Description: "Breadcrumb and node detail at a 1-indexed line number",
	}, handleAtLine)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__since",
		Description: "Constructs on lines changed since a git ref (JSON; includes deletions)",
	}, handleSince)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__stale",
		Description: "Flag stale path/binary/date/env claims in docs",
	}, handleStale)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__search",
		Description: "AST-aware search across titles, content, languages, callout variants, headers",
	}, handleSearch)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__json",
		Description: "Full typed AST for a file or directory (same shape as CLI --json)",
	}, handleJSON)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap__refs",
		Description: "Cross-file markdown reference graph",
	}, handleRefs)

	return server
}

func jsonResult(v any) (*mcpsdk.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult("json encode: " + err.Error()), nil, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(b)}},
	}, v, nil
}

func errorResult(msg string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: msg}},
		IsError: true,
	}
}

func toolErr(format string, args ...any) (*mcpsdk.CallToolResult, any, error) {
	return errorResult(fmt.Sprintf(format, args...)), nil, nil
}
