package render

import (
	"testing"

	"github.com/JordanCoin/docmap/parser"
)

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{10000, "10.0k"},
	}

	for _, tc := range tests {
		got := formatTokens(tc.input)
		if got != tc.expected {
			t.Errorf("formatTokens(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCenterText(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"test", 10, "   test"},
		{"hello", 5, "hello"},
		{"hi", 6, "  hi"},
	}

	for _, tc := range tests {
		got := centerText(tc.input, tc.width)
		if got != tc.expected {
			t.Errorf("centerText(%q, %d) = %q, want %q", tc.input, tc.width, got, tc.expected)
		}
	}
}

func TestResolveKindName(t *testing.T) {
	tests := []struct {
		input string
		want  parser.NodeKind
		ok    bool
	}{
		{"code", parser.KindCodeBlock, true},
		{"codeblock", parser.KindCodeBlock, true},
		{"callout", parser.KindCallout, true},
		{"alert", parser.KindCallout, true},
		{"table", parser.KindTable, true},
		{"task", parser.KindTaskItem, true},
		{"tasks", parser.KindTaskItem, true},
		{"wiki", parser.KindWikiLink, true},
		{"embed", parser.KindWikiEmbed, true},
		{"linkref", parser.KindLinkRefDef, true},
		{"nonexistent", "", false},
	}
	for _, tc := range tests {
		got, ok := resolveKindName(tc.input)
		if got != tc.want || ok != tc.ok {
			t.Errorf("resolveKindName(%q) = (%q, %v), want (%q, %v)", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMatchesSubFilter(t *testing.T) {
	code := &parser.CodeBlock{Language: "python"}
	callout := &parser.Callout{Variant: parser.CalloutWarning}

	if !matchesSubFilter(code, "python", "") {
		t.Error("python code should match --lang python")
	}
	if matchesSubFilter(code, "go", "") {
		t.Error("python code should not match --lang go")
	}
	if !matchesSubFilter(code, "PYTHON", "") {
		t.Error("sub-filter should be case-insensitive")
	}
	if !matchesSubFilter(callout, "", "warning") {
		t.Error("warning callout should match --kind warning")
	}
	if matchesSubFilter(callout, "", "tip") {
		t.Error("warning callout should not match --kind tip")
	}
	// Code should match on kind filter since --kind doesn't apply to it.
	if !matchesSubFilter(code, "", "warning") {
		t.Error("unrelated node should ignore --kind filter")
	}
}

func TestBuildSummaryLines(t *testing.T) {
	s := parser.ContentSummary{
		Callouts:     5,
		Tables:       4,
		CodeBlocks:   8,
		Tasks:        6,
		TasksChecked: 3,
		WikiLinks:    4,
		Mentions:     2,
	}
	lines := buildSummaryLines(s)
	if len(lines) != 3 {
		t.Fatalf("expected 3 summary lines, got %d", len(lines))
	}

	// Block line should mention callouts, tables, and code blocks.
	if !containsAll(lines[0], "5 callouts", "4 tables", "8 code blocks") {
		t.Errorf("line 1 missing expected tokens: %q", lines[0])
	}
	// Interactive line should have tasks and wiki.
	if !containsAll(lines[1], "6 tasks (3 done)", "4 wiki") {
		t.Errorf("line 2 missing expected tokens: %q", lines[1])
	}
	// Refs line should have mentions.
	if !containsAll(lines[2], "2 @mentions") {
		t.Errorf("line 3 missing expected tokens: %q", lines[2])
	}
}

func TestBuildSummaryLinesEmpty(t *testing.T) {
	lines := buildSummaryLines(parser.ContentSummary{})
	if len(lines) != 0 {
		t.Errorf("empty summary should produce 0 lines, got %d", len(lines))
	}
}

func TestBuildSummaryLinesSingleTask(t *testing.T) {
	s := parser.ContentSummary{Tasks: 1, TasksChecked: 1}
	lines := buildSummaryLines(s)
	if len(lines) != 1 || !containsAll(lines[0], "1 task (1 done)") {
		t.Errorf("expected singular 'task', got %v", lines)
	}
}

func TestNotableAnnotationCallouts(t *testing.T) {
	s := &parser.Section{
		Notables: []parser.Node{
			&parser.Callout{
				BaseNode: parser.BaseNode{NKind: parser.KindCallout, Start: 10},
				Variant:  parser.CalloutNote,
			},
			&parser.Callout{
				BaseNode: parser.BaseNode{NKind: parser.KindCallout, Start: 20},
				Variant:  parser.CalloutWarning,
			},
		},
	}
	got := notableAnnotation(s)
	if !containsAll(got, "note :10", "warning :20") {
		t.Errorf("callout annotation missing expected tokens: %q", got)
	}
}

func TestNotableAnnotationCodeBlocks(t *testing.T) {
	s := &parser.Section{
		Notables: []parser.Node{
			&parser.CodeBlock{
				BaseNode: parser.BaseNode{NKind: parser.KindCodeBlock, Start: 5, End: 10},
				Language: "go",
				Fenced:   true,
			},
			&parser.CodeBlock{
				BaseNode: parser.BaseNode{NKind: parser.KindCodeBlock, Start: 15, End: 15},
				Language: "",
				Fenced:   true,
			},
		},
	}
	got := notableAnnotation(s)
	if !containsAll(got, "go :5-10", "(none) :15") {
		t.Errorf("code block annotation missing expected tokens: %q", got)
	}
}

func TestNotableAnnotationStats(t *testing.T) {
	s := &parser.Section{
		Stats: parser.NotableStats{
			Tasks:        6,
			TasksChecked: 3,
			WikiLinks:    4,
			WikiEmbeds:   2,
			Mentions:     1,
		},
	}
	got := notableAnnotation(s)
	if !containsAll(got, "6 tasks (3 done)", "4 wiki", "2 embeds", "1 @mention") {
		t.Errorf("stats annotation missing expected tokens: %q", got)
	}
}

func TestHTMLTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<div class=\"note\">content</div>", "div"},
		{"<details>", "details"},
		{"<!-- comment -->", "comment"},
		{"<kbd>C</kbd>", "kbd"},
		{"plain text", ""},
	}
	for _, tc := range tests {
		got := htmlTag(tc.input)
		if got != tc.want {
			t.Errorf("htmlTag(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// containsAll reports whether all needles are present in haystack.
// Test helper for multi-token matches.
func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !contains(haystack, n) {
			return false
		}
	}
	return true
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
