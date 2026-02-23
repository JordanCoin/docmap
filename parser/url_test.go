package parser

import (
	"runtime"
	"testing"
)

func TestFindChrome(t *testing.T) {
	path, err := findChrome()
	if err != nil {
		// Chrome not installed is okay for CI — just verify the error is clear
		t.Logf("Chrome not found (expected in CI): %v", err)
		return
	}
	if path == "" {
		t.Error("findChrome returned empty path with no error")
	}
	t.Logf("Found Chrome at: %s", path)
}

func TestFindChromeEnvOverride(t *testing.T) {
	t.Setenv("CHROME_PATH", "/nonexistent/chrome")
	_, err := findChrome()
	if err == nil {
		t.Error("expected error for nonexistent CHROME_PATH")
	}
}

func TestFindChromePlatformPaths(t *testing.T) {
	// Verify findChrome checks platform-appropriate paths
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		// Just verify it doesn't panic
		findChrome()
	default:
		t.Logf("Skipping platform test for %s", runtime.GOOS)
	}
}

func TestDetectHeadingsBasic(t *testing.T) {
	lines := []textLine{
		{Text: "Main Title", FontSize: 24.0, IsBold: true},
		{Text: "Some body text here.", FontSize: 12.0},
		{Text: "More body text.", FontSize: 12.0},
		{Text: "Subtitle", FontSize: 18.0, IsBold: true},
		{Text: "Body under subtitle.", FontSize: 12.0},
		{Text: "Another body line.", FontSize: 12.0},
	}

	headings := detectHeadings(lines)
	if len(headings) != 2 {
		t.Fatalf("expected 2 headings, got %d", len(headings))
	}

	// "Main Title" at 24pt should be level 1
	if headings[0].LineIdx != 0 {
		t.Errorf("expected first heading at line 0, got %d", headings[0].LineIdx)
	}
	if headings[0].Level != 1 {
		t.Errorf("expected level 1 for Main Title, got %d", headings[0].Level)
	}

	// "Subtitle" at 18pt should be level 2
	if headings[1].LineIdx != 3 {
		t.Errorf("expected second heading at line 3, got %d", headings[1].LineIdx)
	}
	if headings[1].Level != 2 {
		t.Errorf("expected level 2 for Subtitle, got %d", headings[1].Level)
	}
}

func TestDetectHeadingsThreeLevels(t *testing.T) {
	lines := []textLine{
		{Text: "H1", FontSize: 28.0},
		{Text: "body", FontSize: 12.0},
		{Text: "body", FontSize: 12.0},
		{Text: "body", FontSize: 12.0},
		{Text: "H2", FontSize: 20.0},
		{Text: "body", FontSize: 12.0},
		{Text: "H3", FontSize: 16.0},
		{Text: "body", FontSize: 12.0},
	}

	headings := detectHeadings(lines)
	if len(headings) != 3 {
		t.Fatalf("expected 3 headings, got %d", len(headings))
	}

	if headings[0].Level != 1 {
		t.Errorf("expected H1 level 1, got %d", headings[0].Level)
	}
	if headings[1].Level != 2 {
		t.Errorf("expected H2 level 2, got %d", headings[1].Level)
	}
	if headings[2].Level != 3 {
		t.Errorf("expected H3 level 3, got %d", headings[2].Level)
	}
}

func TestDetectHeadingsNoHeadings(t *testing.T) {
	lines := []textLine{
		{Text: "All same size.", FontSize: 12.0},
		{Text: "Still same size.", FontSize: 12.0},
		{Text: "Yep same size.", FontSize: 12.0},
	}

	headings := detectHeadings(lines)
	if len(headings) != 0 {
		t.Errorf("expected 0 headings for uniform text, got %d", len(headings))
	}
}

func TestDetectHeadingsEmpty(t *testing.T) {
	headings := detectHeadings(nil)
	if headings != nil {
		t.Errorf("expected nil for empty input, got %v", headings)
	}
}

func TestDetectHeadingsBodySizeByCharCount(t *testing.T) {
	// Body text has more total characters even if heading lines are numerous
	lines := []textLine{
		{Text: "Title", FontSize: 24.0},
		{Text: "This is a much longer body text paragraph with many words in it.", FontSize: 12.0},
		{Text: "Another long paragraph of body text that contains a lot of content.", FontSize: 12.0},
	}

	headings := detectHeadings(lines)
	if len(headings) != 1 {
		t.Fatalf("expected 1 heading, got %d", len(headings))
	}
	if headings[0].LineIdx != 0 {
		t.Errorf("expected heading at line 0, got %d", headings[0].LineIdx)
	}
}

func TestBuildSectionsFromLinesBasic(t *testing.T) {
	lines := []textLine{
		{Text: "Introduction", FontSize: 24.0},
		{Text: "Welcome to our docs.", FontSize: 12.0},
		{Text: "Getting Started", FontSize: 24.0},
		{Text: "Install the package.", FontSize: 12.0},
		{Text: "Run the setup command.", FontSize: 12.0},
	}

	headings := []headingInfo{
		{LineIdx: 0, Level: 1},
		{LineIdx: 2, Level: 1},
	}

	sections := buildSectionsFromLines(lines, headings)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}

	if sections[0].Title != "Introduction" {
		t.Errorf("expected 'Introduction', got '%s'", sections[0].Title)
	}
	if sections[0].Content != "Welcome to our docs." {
		t.Errorf("expected body content, got '%s'", sections[0].Content)
	}

	if sections[1].Title != "Getting Started" {
		t.Errorf("expected 'Getting Started', got '%s'", sections[1].Title)
	}
	if sections[1].Tokens == 0 {
		t.Error("expected non-zero tokens for section with content")
	}
}

func TestBuildSectionsFromLinesNested(t *testing.T) {
	lines := []textLine{
		{Text: "Chapter 1", FontSize: 24.0},
		{Text: "Chapter intro.", FontSize: 12.0},
		{Text: "Section 1.1", FontSize: 18.0},
		{Text: "Section content.", FontSize: 12.0},
	}

	headings := []headingInfo{
		{LineIdx: 0, Level: 1},
		{LineIdx: 2, Level: 2},
	}

	sections := buildSectionsFromLines(lines, headings)
	if len(sections) != 1 {
		t.Fatalf("expected 1 root section, got %d", len(sections))
	}

	if sections[0].Title != "Chapter 1" {
		t.Errorf("expected 'Chapter 1', got '%s'", sections[0].Title)
	}
	if len(sections[0].Children) != 1 {
		t.Fatalf("expected 1 child section, got %d", len(sections[0].Children))
	}
	if sections[0].Children[0].Title != "Section 1.1" {
		t.Errorf("expected 'Section 1.1', got '%s'", sections[0].Children[0].Title)
	}
}

func TestBuildSectionsFromLinesNoHeadings(t *testing.T) {
	lines := []textLine{
		{Text: "Just some text.", FontSize: 12.0},
		{Text: "More text here.", FontSize: 12.0},
	}

	sections := buildSectionsFromLines(lines, nil)
	if len(sections) != 1 {
		t.Fatalf("expected 1 fallback section, got %d", len(sections))
	}
	if sections[0].Title != "Just some text." {
		t.Errorf("expected first line as title, got '%s'", sections[0].Title)
	}
}

func TestBuildSectionsFromLinesEmpty(t *testing.T) {
	sections := buildSectionsFromLines(nil, nil)
	if sections != nil {
		t.Errorf("expected nil for empty input, got %v", sections)
	}
}

func TestBuildSectionsFromLinesPreHeadingContent(t *testing.T) {
	// Lines before the first heading should be dropped (nav/header noise)
	lines := []textLine{
		{Text: "Nav link 1", FontSize: 10.0},
		{Text: "Nav link 2", FontSize: 10.0},
		{Text: "Real Title", FontSize: 24.0},
		{Text: "Real content.", FontSize: 12.0},
	}

	headings := []headingInfo{
		{LineIdx: 2, Level: 1},
	}

	sections := buildSectionsFromLines(lines, headings)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if sections[0].Title != "Real Title" {
		t.Errorf("expected 'Real Title', got '%s'", sections[0].Title)
	}
}

func TestMergeCharsToLine(t *testing.T) {
	chars := []charInfo{
		{text: "H", x: 10, fontSize: 24, isBold: true, fontName: "Arial-Bold"},
		{text: "e", x: 22, fontSize: 24, isBold: true, fontName: "Arial-Bold"},
		{text: "l", x: 34, fontSize: 24, isBold: true, fontName: "Arial-Bold"},
		{text: "l", x: 46, fontSize: 24, isBold: true, fontName: "Arial-Bold"},
		{text: "o", x: 58, fontSize: 24, isBold: true, fontName: "Arial-Bold"},
	}

	line := mergeCharsToLine(chars)
	if line.Text != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", line.Text)
	}
	if !line.IsBold {
		t.Error("expected bold line")
	}
	if line.FontSize != 24.0 {
		t.Errorf("expected font size 24.0, got %f", line.FontSize)
	}
	if line.FontName != "Arial-Bold" {
		t.Errorf("expected font name 'Arial-Bold', got '%s'", line.FontName)
	}
}

func TestMergeCharsToLineWithSpaces(t *testing.T) {
	// Characters with a large gap should produce a space
	chars := []charInfo{
		{text: "A", x: 10, fontSize: 12, fontName: "Arial"},
		{text: "B", x: 17, fontSize: 12, fontName: "Arial"}, // close (gap 7 < 10.8)
		{text: "C", x: 80, fontSize: 12, fontName: "Arial"},  // far away
	}

	line := mergeCharsToLine(chars)
	if line.Text != "AB C" {
		t.Errorf("expected 'AB C', got '%s'", line.Text)
	}
}

func TestExportYAMLRoundTrip(t *testing.T) {
	doc := &Document{
		Filename:    "test.md",
		TotalTokens: 100,
		Sections: []*Section{
			{
				Level:   1,
				Title:   "Introduction",
				Content: "Welcome to the docs.",
				Tokens:  50,
				Children: []*Section{
					{
						Level:   2,
						Title:   "Getting Started",
						Content: "Install the package.",
						Tokens:  25,
					},
				},
			},
			{
				Level:   1,
				Title:   "API Reference",
				Content: "Endpoint docs.",
				Tokens:  25,
			},
		},
	}

	yamlContent, err := ExportYAML(doc)
	if err != nil {
		t.Fatalf("ExportYAML failed: %v", err)
	}

	if yamlContent == "" {
		t.Fatal("expected non-empty YAML output")
	}

	// Parse it back
	parsed, err := ParseYAML(yamlContent)
	if err != nil {
		t.Fatalf("ParseYAML of exported content failed: %v", err)
	}

	// Verify structure was preserved
	if len(parsed.Sections) == 0 {
		t.Fatal("expected sections in round-tripped document")
	}

	// Find the "sections" key and verify it has children
	var sectionsNode *Section
	for _, s := range parsed.Sections {
		if s.Title == "sections" {
			sectionsNode = s
			break
		}
	}
	if sectionsNode == nil {
		t.Fatal("expected 'sections' key in parsed YAML")
	}
	if len(sectionsNode.Children) != 2 {
		t.Errorf("expected 2 section children, got %d", len(sectionsNode.Children))
	}
}

func TestExportYAMLEmpty(t *testing.T) {
	doc := &Document{}
	yamlContent, err := ExportYAML(doc)
	if err != nil {
		t.Fatalf("ExportYAML failed on empty doc: %v", err)
	}
	if yamlContent == "" {
		t.Fatal("expected non-empty YAML even for empty doc")
	}
}
