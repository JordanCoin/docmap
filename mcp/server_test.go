package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JordanCoin/docmap/internal/docset"
)

func TestToolContracts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guide.md")
	content := "# Guide\n\n## Install\n\nRun `missing-bin --help`.\n\n```python\nprint(1)\n```\n\n## API\n\nEndpoints.\n\n> [!WARNING]\n> Careful\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("inventory", func(t *testing.T) {
		res, out, err := handleInventory(ctx, nil, pathInput{Path: dir})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("inventory: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if m["total_docs"].(int) != 1 {
			t.Fatalf("inventory: %#v", m)
		}
	})

	t.Run("tree", func(t *testing.T) {
		res, out, err := handleTree(ctx, nil, pathInput{Path: path})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("tree: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if _, ok := m["documents"]; !ok {
			t.Fatalf("missing documents: %#v", m)
		}
	})

	t.Run("brief", func(t *testing.T) {
		res, out, err := handleBrief(ctx, nil, briefInput{Path: dir})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("brief: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if m["total_docs"].(int) != 1 || m["total_sections"].(int) < 2 {
			t.Fatalf("brief counts: %#v", m)
		}
		if _, ok := m["stale_count"]; !ok {
			t.Fatalf("missing stale_count: %#v", m)
		}
	})

	t.Run("section", func(t *testing.T) {
		res, out, err := handleSection(ctx, nil, sectionInput{Path: path, Section: "API"})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("section: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		matches := m["matches"].([]map[string]any)
		if len(matches) != 1 {
			t.Fatalf("section matches: %#v", m)
		}
	})

	t.Run("expand", func(t *testing.T) {
		res, out, err := handleExpand(ctx, nil, sectionInput{Path: path, Section: "Install"})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("expand: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		matches := m["matches"].([]map[string]any)
		if len(matches) != 1 {
			t.Fatalf("expand: %#v", m)
		}
		if _, ok := matches[0]["content"].(string); !ok {
			t.Fatalf("expand missing content: %#v", matches[0])
		}
	})

	t.Run("find_by_type", func(t *testing.T) {
		res, out, err := handleFindByType(ctx, nil, findByTypeInput{Path: path, Kind: "code", Lang: "python"})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("find_by_type: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if m["count"].(int) < 1 {
			t.Fatalf("expected code hit: %#v", m)
		}
	})

	t.Run("at_line", func(t *testing.T) {
		res, out, err := handleAtLine(ctx, nil, atLineInput{Path: path, Line: 1})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("at_line: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if m["line"].(int) != 1 {
			t.Fatalf("at_line: %#v", m)
		}
	})

	t.Run("stale", func(t *testing.T) {
		res, out, err := handleStale(ctx, nil, staleInput{Path: dir})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("stale: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if _, ok := m["findings"]; !ok {
			t.Fatalf("missing findings: %#v", m)
		}
	})

	t.Run("search", func(t *testing.T) {
		res, out, err := handleSearch(ctx, nil, searchInput{Path: dir, Query: "API"})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("search: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if m["count"].(int) < 1 {
			t.Fatalf("search: %#v", m)
		}
	})

	t.Run("json", func(t *testing.T) {
		res, out, err := handleJSON(ctx, nil, pathInput{Path: path})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("json: res=%v err=%v", res, err)
		}
		jo := out.(docset.JSONOutput)
		if jo.TotalDocs != 1 {
			t.Fatalf("json: %#v", jo)
		}
	})

	t.Run("refs", func(t *testing.T) {
		res, out, err := handleRefs(ctx, nil, pathInput{Path: dir})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("refs: res=%v err=%v", res, err)
		}
		m := out.(map[string]any)
		if _, ok := m["refs"]; !ok {
			t.Fatalf("refs: %#v", m)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		res, _, err := handleTree(ctx, nil, pathInput{})
		if err != nil {
			t.Fatal(err)
		}
		if res == nil || !res.IsError {
			t.Fatal("expected error result for empty path")
		}
	})

	t.Run("since missing ref", func(t *testing.T) {
		res, _, err := handleSince(ctx, nil, sinceInput{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if res == nil || !res.IsError {
			t.Fatal("expected error for missing ref")
		}
	})
}

func TestNewServerConstructs(t *testing.T) {
	if NewServer() == nil {
		t.Fatal("nil server")
	}
}

func TestResolveKindName(t *testing.T) {
	k, ok := resolveKindName("code")
	if !ok || string(k) != "code_block" {
		t.Fatalf("got %v %v", k, ok)
	}
	if _, ok := resolveKindName("nope"); ok {
		t.Fatal("expected unknown")
	}
}
