package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// parseFixture loads testdata/kitchen_sink.md and parses it.
// Every kitchen-sink test goes through this helper so we have one place
// to swap in a richer parser later.
func parseFixture(t *testing.T) *Document {
	t.Helper()
	path := filepath.Join("testdata", "kitchen_sink.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return Parse(string(data))
}

// collect walks every root in Document.Nodes and returns descendants
// whose Kind matches the target.
func collect(doc *Document, kind NodeKind) []Node {
	var out []Node
	for _, root := range doc.Nodes {
		out = append(out, FindByKind(root, kind)...)
	}
	return out
}

// ---------------- Frontmatter ----------------

func TestKitchenSink_Frontmatter(t *testing.T) {
	doc := parseFixture(t)
	fms := collect(doc, KindFrontmatter)
	if len(fms) != 1 {
		t.Fatalf("expected 1 frontmatter, got %d", len(fms))
	}
	fm, ok := fms[0].(*Frontmatter)
	if !ok {
		t.Fatalf("expected *Frontmatter, got %T", fms[0])
	}
	if fm.Format != FrontmatterYAML {
		t.Errorf("expected YAML format, got %q", fm.Format)
	}
	if fm.Raw == "" {
		t.Error("expected frontmatter Raw content to be populated")
	}
}

// ---------------- Headings ----------------

func TestKitchenSink_SetextHeadings(t *testing.T) {
	doc := parseFixture(t)
	headings := collect(doc, KindHeading)

	var setext []*Heading
	for _, n := range headings {
		h := n.(*Heading)
		if h.IsSetext {
			setext = append(setext, h)
		}
	}
	if len(setext) != 2 {
		t.Fatalf("expected 2 setext headings, got %d", len(setext))
	}
	if setext[0].Level != 1 || setext[0].Title != "Setext H1" {
		t.Errorf("first setext = level %d %q, want 1 %q", setext[0].Level, setext[0].Title, "Setext H1")
	}
	if setext[1].Level != 2 || setext[1].Title != "Setext H2" {
		t.Errorf("second setext = level %d %q, want 2 %q", setext[1].Level, setext[1].Title, "Setext H2")
	}
}

func TestKitchenSink_ATXHeadingsAllLevels(t *testing.T) {
	doc := parseFixture(t)
	headings := collect(doc, KindHeading)

	seen := map[int]bool{}
	for _, n := range headings {
		h := n.(*Heading)
		if !h.IsSetext && h.Title == "ATX Heading "+string(rune('0'+h.Level)) {
			seen[h.Level] = true
		}
	}
	for level := 1; level <= 6; level++ {
		if !seen[level] {
			t.Errorf("missing ATX heading at level %d", level)
		}
	}
}

func TestKitchenSink_InvalidHeadingRejected(t *testing.T) {
	doc := parseFixture(t)
	for _, n := range collect(doc, KindHeading) {
		h := n.(*Heading)
		if h.Title == "Not a heading (7+ hashes is invalid)" {
			t.Error("7-hash line was parsed as a heading; it should be text")
		}
	}
}

// ---------------- Callouts (GFM alerts) ----------------

func TestKitchenSink_AllFiveCallouts(t *testing.T) {
	doc := parseFixture(t)
	callouts := collect(doc, KindCallout)

	want := map[CalloutKind]bool{
		CalloutNote:      false,
		CalloutTip:       false,
		CalloutImportant: false,
		CalloutWarning:   false,
		CalloutCaution:   false,
	}
	for _, n := range callouts {
		c := n.(*Callout)
		if _, ok := want[c.Variant]; ok {
			want[c.Variant] = true
		}
	}
	for kind, found := range want {
		if !found {
			t.Errorf("missing callout variant %q", kind)
		}
	}
}

// ---------------- Code blocks ----------------

func TestKitchenSink_CodeBlocksByLanguage(t *testing.T) {
	doc := parseFixture(t)
	blocks := collect(doc, KindCodeBlock)

	byLang := map[string]int{}
	for _, n := range blocks {
		cb := n.(*CodeBlock)
		byLang[cb.Language]++
	}

	for _, lang := range []string{"go", "python", "bash", "rust", "mermaid", "javascript"} {
		if byLang[lang] == 0 {
			t.Errorf("expected at least one %q code block", lang)
		}
	}
	if byLang[""] == 0 {
		t.Error("expected at least one unlabeled code block")
	}
}

func TestKitchenSink_IndentedCodeBlockParsed(t *testing.T) {
	doc := parseFixture(t)
	blocks := collect(doc, KindCodeBlock)
	var indented int
	for _, n := range blocks {
		if !n.(*CodeBlock).Fenced {
			indented++
		}
	}
	if indented == 0 {
		t.Error("expected at least one indented (non-fenced) code block")
	}
}

// ---------------- Tables ----------------

func TestKitchenSink_TablesWithAlignment(t *testing.T) {
	doc := parseFixture(t)
	tables := collect(doc, KindTable)
	if len(tables) < 3 {
		t.Fatalf("expected at least 3 tables, got %d", len(tables))
	}

	// The "Aligned table" should have left / center / right alignment.
	var alignedFound bool
	for _, n := range tables {
		tbl := n.(*Table)
		if len(tbl.Aligns) == 3 &&
			tbl.Aligns[0] == AlignLeft &&
			tbl.Aligns[1] == AlignCenter &&
			tbl.Aligns[2] == AlignRight {
			alignedFound = true
			break
		}
	}
	if !alignedFound {
		t.Error("expected an aligned table with left/center/right columns")
	}
}

// ---------------- Lists & tasks ----------------

func TestKitchenSink_TaskItems(t *testing.T) {
	doc := parseFixture(t)
	tasks := collect(doc, KindTaskItem)
	if len(tasks) < 6 {
		t.Fatalf("expected at least 6 task items, got %d", len(tasks))
	}

	var checked, unchecked int
	for _, n := range tasks {
		if n.(*TaskItem).Checked {
			checked++
		} else {
			unchecked++
		}
	}
	if checked < 3 {
		t.Errorf("expected at least 3 checked tasks, got %d", checked)
	}
	if unchecked < 3 {
		t.Errorf("expected at least 3 unchecked tasks, got %d", unchecked)
	}
}

func TestKitchenSink_OrderedAndUnorderedLists(t *testing.T) {
	doc := parseFixture(t)
	lists := collect(doc, KindList)
	if len(lists) < 2 {
		t.Fatalf("expected at least 2 lists, got %d", len(lists))
	}
	var ordered, unordered int
	for _, n := range lists {
		if n.(*List).Ordered {
			ordered++
		} else {
			unordered++
		}
	}
	if ordered == 0 || unordered == 0 {
		t.Errorf("expected both ordered and unordered lists, got ordered=%d unordered=%d", ordered, unordered)
	}
}

// ---------------- Math ----------------

func TestKitchenSink_MathBlocks(t *testing.T) {
	doc := parseFixture(t)
	blocks := collect(doc, KindMathBlock)
	if len(blocks) < 2 {
		t.Errorf("expected at least 2 math blocks, got %d", len(blocks))
	}
}

func TestKitchenSink_InlineMath(t *testing.T) {
	doc := parseFixture(t)
	inline := collect(doc, KindInlineMath)
	if len(inline) < 2 {
		t.Errorf("expected at least 2 inline math spans, got %d", len(inline))
	}
}

// ---------------- Thematic breaks ----------------

func TestKitchenSink_ThematicBreaks(t *testing.T) {
	doc := parseFixture(t)
	breaks := collect(doc, KindThematicBreak)
	if len(breaks) < 4 {
		t.Errorf("expected at least 4 thematic breaks (---, ***, ___, - - -), got %d", len(breaks))
	}
}

// ---------------- Footnotes ----------------

func TestKitchenSink_FootnoteDefinitions(t *testing.T) {
	doc := parseFixture(t)
	defs := collect(doc, KindFootnoteDef)
	if len(defs) < 3 {
		t.Errorf("expected at least 3 footnote definitions, got %d", len(defs))
	}
}

func TestKitchenSink_FootnoteReferences(t *testing.T) {
	doc := parseFixture(t)
	refs := collect(doc, KindFootnoteRef)
	if len(refs) < 3 {
		t.Errorf("expected at least 3 footnote references, got %d", len(refs))
	}
}

// ---------------- Definition lists ----------------

func TestKitchenSink_DefinitionList(t *testing.T) {
	doc := parseFixture(t)
	lists := collect(doc, KindDefinitionList)
	if len(lists) == 0 {
		t.Error("expected at least one definition list")
	}
	terms := collect(doc, KindDefTerm)
	if len(terms) < 3 {
		t.Errorf("expected at least 3 definition terms, got %d", len(terms))
	}
}

// ---------------- Links ----------------

func TestKitchenSink_LinkReferenceDefinitions(t *testing.T) {
	doc := parseFixture(t)
	defs := collect(doc, KindLinkRefDef)
	if len(defs) < 3 {
		t.Errorf("expected at least 3 link reference definitions, got %d", len(defs))
	}
}

func TestKitchenSink_Autolinks(t *testing.T) {
	doc := parseFixture(t)
	links := collect(doc, KindAutoLink)
	if len(links) < 2 {
		t.Errorf("expected at least 2 autolinks, got %d", len(links))
	}

	var hasBare, hasMail bool
	for _, n := range links {
		al := n.(*AutoLink)
		if al.IsBare {
			hasBare = true
		}
		if al.IsMail {
			hasMail = true
		}
	}
	if !hasBare {
		t.Error("expected a GFM bare-URL autolink")
	}
	if !hasMail {
		t.Error("expected an email autolink")
	}
}

// ---------------- HTML ----------------

func TestKitchenSink_HTMLBlocks(t *testing.T) {
	doc := parseFixture(t)
	blocks := collect(doc, KindHTMLBlock)
	if len(blocks) < 2 {
		t.Errorf("expected at least 2 HTML blocks (<div>, <details>), got %d", len(blocks))
	}
}

func TestKitchenSink_HTMLEntities(t *testing.T) {
	doc := parseFixture(t)
	ents := collect(doc, KindEntity)
	if len(ents) < 5 {
		t.Errorf("expected at least 5 HTML entities, got %d", len(ents))
	}
}

// ---------------- GFM extras ----------------

func TestKitchenSink_Mentions(t *testing.T) {
	doc := parseFixture(t)
	m := collect(doc, KindMention)
	if len(m) < 2 {
		t.Errorf("expected at least 2 @mentions, got %d", len(m))
	}
}

func TestKitchenSink_IssueReferences(t *testing.T) {
	doc := parseFixture(t)
	refs := collect(doc, KindIssueRef)
	if len(refs) < 2 {
		t.Errorf("expected at least 2 issue references, got %d", len(refs))
	}
}

func TestKitchenSink_Emoji(t *testing.T) {
	doc := parseFixture(t)
	e := collect(doc, KindEmoji)
	if len(e) < 4 {
		t.Errorf("expected at least 4 emoji shortcodes, got %d", len(e))
	}
}

// ---------------- Obsidian ----------------

func TestKitchenSink_ObsidianWikiLinks(t *testing.T) {
	doc := parseFixture(t)
	links := collect(doc, KindWikiLink)
	if len(links) < 4 {
		t.Errorf("expected at least 4 wiki links, got %d", len(links))
	}

	var hasAlias, hasAnchor, hasBlock bool
	for _, n := range links {
		wl := n.(*WikiLink)
		if wl.Alias != "" {
			hasAlias = true
		}
		if wl.Anchor != "" {
			hasAnchor = true
		}
		if wl.Block != "" {
			hasBlock = true
		}
	}
	if !hasAlias {
		t.Error("expected a wiki link with alias ([[Page|alias]])")
	}
	if !hasAnchor {
		t.Error("expected a wiki link with header anchor ([[Page#Header]])")
	}
	if !hasBlock {
		t.Error("expected a wiki link with block reference ([[Page#^block]])")
	}
}

func TestKitchenSink_ObsidianEmbeds(t *testing.T) {
	doc := parseFixture(t)
	embeds := collect(doc, KindWikiEmbed)
	if len(embeds) < 5 {
		t.Errorf("expected at least 5 wiki embeds, got %d", len(embeds))
	}

	var hasSized bool
	for _, n := range embeds {
		we := n.(*WikiEmbed)
		if we.Width > 0 {
			hasSized = true
		}
	}
	if !hasSized {
		t.Error("expected an embed with explicit width (e.g. ![[img|200]])")
	}
}

// ---------------- Line breaks ----------------

func TestKitchenSink_HardLineBreaks(t *testing.T) {
	doc := parseFixture(t)
	breaks := collect(doc, KindLineBreak)

	var hard int
	for _, n := range breaks {
		if n.(*LineBreak).Hard {
			hard++
		}
	}
	if hard < 2 {
		t.Errorf("expected at least 2 hard line breaks (two-space + backslash), got %d", hard)
	}
}
