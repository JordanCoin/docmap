package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer builds an MCP server that wraps the docmap CLI.
func NewServer(version string, runner Runner) *mcpsdk.Server {
	if runner == nil {
		runner = ExecRunner{}
	}
	if version == "" {
		version = "dev"
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "docmap",
		Version: version,
	}, nil)

	registerTools(server, runner)
	return server
}

// RunStdio serves MCP over stdin/stdout until the client disconnects.
func RunStdio(ctx context.Context, version string, runner Runner) error {
	return NewServer(version, runner).Run(ctx, &mcpsdk.StdioTransport{})
}

func registerTools(server *mcpsdk.Server, runner Runner) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap_brief",
		Description: "Session-start digest: file/section/token counts and recent docs. Optionally include a stale claim count.",
	}, makeBrief(runner))

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap_tree",
		Description: "Full typed documentation tree for a file or directory as JSON (same shape as `docmap --json`).",
	}, makeTree(runner))

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap_since",
		Description: "Constructs on lines changed since a git ref, as JSON (same shape as `docmap --since <ref> --json`).",
	}, makeSince(runner))

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap_stale",
		Description: "Flag stale path/binary/date/env claims in docs as JSON (same shape as `docmap --stale --json`).",
	}, makeStale(runner))

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap_search",
		Description: "AST-aware search across documentation titles, content, code languages, callouts, and other notables.",
	}, makeSearch(runner))

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "docmap_at_line",
		Description: "Reverse lookup: what construct lives at a 1-indexed line in a documentation file.",
	}, makeAtLine(runner))
}

type pathArgs struct {
	Path string `json:"path" jsonschema:"file or directory to map"`
}

type briefArgs struct {
	Path  string `json:"path" jsonschema:"file or directory to summarize"`
	Stale bool   `json:"stale,omitempty" jsonschema:"if true, include a stale claim count"`
	Days  int    `json:"days,omitempty" jsonschema:"with stale: status dates older than N days (default 90)"`
}

type sinceArgs struct {
	Path string `json:"path" jsonschema:"file or directory to inspect"`
	Ref  string `json:"ref" jsonschema:"git ref to diff against (e.g. HEAD~5, main)"`
}

type staleArgs struct {
	Path   string `json:"path" jsonschema:"file or directory to check"`
	Days   int    `json:"days,omitempty" jsonschema:"status dates older than N days (default 90)"`
	Remote bool   `json:"remote,omitempty" jsonschema:"if true, also HEAD-check URLs and compare config values"`
}

type searchArgs struct {
	Path  string `json:"path" jsonschema:"file or directory to search"`
	Query string `json:"query" jsonschema:"search string matched against the typed AST"`
}

type atLineArgs struct {
	Path string `json:"path" jsonschema:"documentation file to inspect"`
	Line int    `json:"line" jsonschema:"1-indexed line number"`
}

func makeBrief(runner Runner) func(context.Context, *mcpsdk.CallToolRequest, briefArgs) (*mcpsdk.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in briefArgs) (*mcpsdk.CallToolResult, any, error) {
		path, err := requirePath(in.Path)
		if err != nil {
			return toolErr(err), nil, nil
		}
		args := []string{path, "--brief"}
		if in.Stale {
			args = append(args, "--stale")
		}
		if in.Days > 0 {
			args = append(args, "--days", strconv.Itoa(in.Days))
		}
		return runText(ctx, runner, args...)
	}
}

func makeTree(runner Runner) func(context.Context, *mcpsdk.CallToolRequest, pathArgs) (*mcpsdk.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in pathArgs) (*mcpsdk.CallToolResult, any, error) {
		path, err := requirePath(in.Path)
		if err != nil {
			return toolErr(err), nil, nil
		}
		return runText(ctx, runner, path, "--json")
	}
}

func makeSince(runner Runner) func(context.Context, *mcpsdk.CallToolRequest, sinceArgs) (*mcpsdk.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in sinceArgs) (*mcpsdk.CallToolResult, any, error) {
		path, err := requirePath(in.Path)
		if err != nil {
			return toolErr(err), nil, nil
		}
		ref := strings.TrimSpace(in.Ref)
		if ref == "" {
			return toolErr(fmt.Errorf("ref is required")), nil, nil
		}
		return runText(ctx, runner, path, "--since", ref, "--json")
	}
}

func makeStale(runner Runner) func(context.Context, *mcpsdk.CallToolRequest, staleArgs) (*mcpsdk.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in staleArgs) (*mcpsdk.CallToolResult, any, error) {
		path, err := requirePath(in.Path)
		if err != nil {
			return toolErr(err), nil, nil
		}
		args := []string{path, "--stale", "--json"}
		if in.Days > 0 {
			args = append(args, "--days", strconv.Itoa(in.Days))
		}
		if in.Remote {
			args = append(args, "--remote")
		}
		return runText(ctx, runner, args...)
	}
}

func makeSearch(runner Runner) func(context.Context, *mcpsdk.CallToolRequest, searchArgs) (*mcpsdk.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in searchArgs) (*mcpsdk.CallToolResult, any, error) {
		path, err := requirePath(in.Path)
		if err != nil {
			return toolErr(err), nil, nil
		}
		query := strings.TrimSpace(in.Query)
		if query == "" {
			return toolErr(fmt.Errorf("query is required")), nil, nil
		}
		return runText(ctx, runner, path, "--search", query, "--json")
	}
}

func makeAtLine(runner Runner) func(context.Context, *mcpsdk.CallToolRequest, atLineArgs) (*mcpsdk.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in atLineArgs) (*mcpsdk.CallToolResult, any, error) {
		path, err := requirePath(in.Path)
		if err != nil {
			return toolErr(err), nil, nil
		}
		if in.Line <= 0 {
			return toolErr(fmt.Errorf("line must be a positive 1-indexed line number")), nil, nil
		}
		return runText(ctx, runner, path, "--at", strconv.Itoa(in.Line))
	}
}

func requirePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	return path, nil
}

func runText(ctx context.Context, runner Runner, args ...string) (*mcpsdk.CallToolResult, any, error) {
	out, err := runner.Run(ctx, args...)
	if err != nil {
		return toolErr(err), nil, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: out}},
	}, nil, nil
}

func toolErr(err error) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
		IsError: true,
	}
}
