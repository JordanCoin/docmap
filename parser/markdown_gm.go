package parser

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// parseWithGoldmark parses source into docmap Node values using goldmark
// for the CommonMark/GFM core and post-passes for everything goldmark does
// not natively handle: YAML frontmatter, math ($$ and \[), GFM callouts,
// Obsidian wiki links and embeds, HTML entities, @mentions, #issue refs,
// commit SHAs, :emoji:, and hard-line-break classification.
func parseWithGoldmark(source []byte) []Node {
	// Step 1: split frontmatter off the top of the file.
	fmNode, body, fmLines := splitFrontmatter(source)

	// Step 2: parse the body with goldmark + GFM + footnote + definition list.
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
		),
	)
	reader := text.NewReader(body)
	root := md.Parser().Parse(reader)

	// Step 3: map goldmark's AST to our Node types.
	conv := &converter{source: body, lineOffset: fmLines}
	var nodes []Node
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		// FootnoteList is transparent: flatten its children up.
		if _, ok := c.(*extast.FootnoteList); ok {
			for fn := c.FirstChild(); fn != nil; fn = fn.NextSibling() {
				if n := conv.convertBlock(fn); n != nil {
					nodes = append(nodes, n)
				}
			}
			continue
		}
		if n := conv.convertBlock(c); n != nil {
			nodes = append(nodes, n)
		}
	}

	// Step 3b: collect link reference definitions by scanning the raw body,
	// since goldmark absorbs them into its reference table and drops them
	// from the AST.
	nodes = append(nodes, collectLinkRefDefs(body)...)

	// Step 3c: restore document order. Goldmark places FootnoteList at the
	// end of the root regardless of where the definitions appear, and our
	// link-ref-def post-pass appends at the end too. Sort by LineStart so
	// each node lands under the heading whose content it came from.
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].LineStart() < nodes[j].LineStart()
	})

	// Step 4: post-passes for constructs goldmark doesn't know about.
	nodes = detectMathBlocks(nodes, body, fmLines)
	detectCallouts(nodes)
	postProcessInline(nodes)

	// Step 5: prepend frontmatter if present.
	if fmNode != nil {
		nodes = append([]Node{fmNode}, nodes...)
	}
	return nodes
}

// ---------- Frontmatter splitter ----------

var frontmatterFence = regexp.MustCompile(`(?m)\A---\r?\n`)

// splitFrontmatter peels a `---\n...\n---\n` YAML block off the top of source.
// It returns the Frontmatter node, the remaining body, and how many lines
// were consumed (so downstream line numbers can be shifted back).
func splitFrontmatter(source []byte) (Node, []byte, int) {
	if !frontmatterFence.Match(source) {
		return nil, source, 0
	}
	// Find the closing fence after the opening one.
	rest := source[4:] // skip "---\n" (len 4 for unix LF)
	// Handle CRLF opener.
	if len(source) >= 5 && source[3] == '\r' {
		rest = source[5:]
	}
	closingIdx := bytes.Index(rest, []byte("\n---"))
	if closingIdx < 0 {
		return nil, source, 0
	}
	// Make sure the closing fence is at a line boundary followed by newline or EOF.
	after := closingIdx + 4 // past "\n---"
	if after < len(rest) && rest[after] != '\n' && rest[after] != '\r' {
		return nil, source, 0
	}

	raw := string(rest[:closingIdx])
	// Count consumed lines in the original source (opening fence through closing).
	consumedEnd := len(source) - len(rest) + after
	if consumedEnd < len(source) && source[consumedEnd] == '\n' {
		consumedEnd++
	} else if consumedEnd+1 < len(source) && source[consumedEnd] == '\r' && source[consumedEnd+1] == '\n' {
		consumedEnd += 2
	}
	consumedLines := bytes.Count(source[:consumedEnd], []byte{'\n'})

	// Replace the consumed prefix with blank lines so line numbers in the
	// remaining body stay stable relative to the original source.
	body := append(bytes.Repeat([]byte{'\n'}, consumedLines), source[consumedEnd:]...)

	fm := &Frontmatter{
		BaseNode: BaseNode{
			NKind:    KindFrontmatter,
			Start:    1,
			End:      consumedLines,
			TokCount: estimateTokens(raw),
		},
		Format: FrontmatterYAML,
		Raw:    raw,
	}
	return fm, body, consumedLines
}

// ---------- Line number helper ----------

func lineAt(source []byte, offset int) int {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	return bytes.Count(source[:offset], []byte{'\n'}) + 1
}

// ---------- Converter ----------

type converter struct {
	source     []byte
	lineOffset int // informational only; body bytes already carry blank-line padding
}

// rangeOf returns the 1-indexed start and end line numbers for a goldmark node.
// Many container nodes (Blockquote, Table, DefinitionList, Footnote) do not
// carry Lines() directly — their segments live on descendants — so we fall
// back to walking the node's children until we find segments.
func (c *converter) rangeOf(n ast.Node) (int, int) {
	if lc, ok := n.(interface{ Lines() *text.Segments }); ok {
		segs := lc.Lines()
		if segs != nil && segs.Len() > 0 {
			first := segs.At(0)
			last := segs.At(segs.Len() - 1)
			return lineAt(c.source, first.Start), lineAt(c.source, last.Stop)
		}
	}
	// Fallback: scan descendants for any block node with segments and
	// take the outermost span. Inline nodes panic on Lines(), so we must
	// guard on node.Type() first.
	start, end := 0, 0
	ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		if lc, ok := node.(interface{ Lines() *text.Segments }); ok {
			segs := lc.Lines()
			if segs != nil && segs.Len() > 0 {
				s := lineAt(c.source, segs.At(0).Start)
				e := lineAt(c.source, segs.At(segs.Len()-1).Stop)
				if start == 0 || s < start {
					start = s
				}
				if e > end {
					end = e
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return start, end
}

// textOf concatenates the segments of a block node into a string,
// inserting '\n' between segments so callers see the original line shape.
func (c *converter) textOf(n ast.Node) string {
	if lc, ok := n.(interface{ Lines() *text.Segments }); ok {
		segs := lc.Lines()
		if segs != nil && segs.Len() > 0 {
			var b strings.Builder
			for i := 0; i < segs.Len(); i++ {
				seg := segs.At(i)
				val := seg.Value(c.source)
				b.Write(val)
				// Segments usually end at the newline; only add one if missing.
				if len(val) == 0 || val[len(val)-1] != '\n' {
					if i < segs.Len()-1 {
						b.WriteByte('\n')
					}
				}
			}
			return b.String()
		}
	}
	// Fallback: collect Text/String descendants.
	var b strings.Builder
	ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := node.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(c.source))
		case *ast.String:
			b.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// lineStartsWithHash reports whether the line containing `offset` begins
// with an ATX `#`. Used to tell ATX headings from Setext headings.
func lineStartsWithHash(source []byte, offset int) bool {
	if offset < 0 || offset > len(source) {
		return false
	}
	lineStart := offset
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}
	return lineStart < len(source) && source[lineStart] == '#'
}

// inlineText returns the plain-text rendering of an inline-bearing node.
func (c *converter) inlineText(n ast.Node) string {
	var b strings.Builder
	ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := node.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(c.source))
		case *ast.String:
			b.Write(t.Value)
		case *ast.CodeSpan:
			for ch := t.FirstChild(); ch != nil; ch = ch.NextSibling() {
				if tx, ok := ch.(*ast.Text); ok {
					b.Write(tx.Segment.Value(c.source))
				}
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func (c *converter) convertBlock(n ast.Node) Node {
	start, end := c.rangeOf(n)
	base := BaseNode{Start: start, End: end}

	switch node := n.(type) {

	case *ast.Heading:
		title := c.inlineText(node)
		isSetext := false
		if segs := node.Lines(); segs != nil && segs.Len() > 0 {
			first := segs.At(0)
			isSetext = !lineStartsWithHash(c.source, first.Start)
		}
		base.NKind = KindHeading
		base.TokCount = estimateTokens(title)
		return &Heading{
			BaseNode: base,
			Level:    node.Level,
			Title:    title,
			RawTitle: title,
			IsSetext: isSetext,
		}

	case *ast.Paragraph:
		txt := c.inlineText(node)
		raw := c.textOf(node)
		base.NKind = KindParagraph
		base.TokCount = estimateTokens(txt)
		p := &Paragraph{BaseNode: base, Text: txt, Raw: raw}
		c.attachInline(node, p)
		return p

	case *ast.TextBlock:
		// Goldmark uses TextBlock for inline content inside tight list items.
		// Treat it as an inline paragraph so inline scanners run on it.
		txt := c.inlineText(node)
		raw := c.textOf(node)
		base.NKind = KindParagraph
		base.TokCount = estimateTokens(txt)
		p := &Paragraph{BaseNode: base, Text: txt, Raw: raw}
		c.attachInline(node, p)
		return p

	case *ast.Blockquote:
		base.NKind = KindBlockquote
		bq := &Blockquote{BaseNode: base}
		c.convertChildrenInto(node, &bq.Kids)
		bq.TokCount = sumTokens(bq.Kids)
		return bq

	case *ast.List:
		base.NKind = KindList
		l := &List{
			BaseNode: base,
			Ordered:  node.IsOrdered(),
			Tight:    node.IsTight,
			Start:    node.Start,
			Marker:   rune(node.Marker),
		}
		c.convertChildrenInto(node, &l.Kids)
		l.TokCount = sumTokens(l.Kids)
		return l

	case *ast.ListItem:
		// Detect GFM task item. Goldmark puts the checkbox as the first
		// child of the first block child (which can be Paragraph, TextBlock,
		// or similar inline container) of the list item.
		if first := node.FirstChild(); first != nil {
			if cb, ok := first.FirstChild().(*extast.TaskCheckBox); ok {
				base.NKind = KindTaskItem
				t := &TaskItem{BaseNode: base, Checked: cb.IsChecked}
				c.convertChildrenInto(node, &t.Kids)
				t.TokCount = sumTokens(t.Kids)
				return t
			}
		}
		base.NKind = KindListItem
		li := &ListItem{BaseNode: base}
		c.convertChildrenInto(node, &li.Kids)
		li.TokCount = sumTokens(li.Kids)
		return li

	case *ast.FencedCodeBlock:
		lang := ""
		if node.Info != nil {
			info := string(node.Info.Segment.Value(c.source))
			// Language is the first whitespace-delimited token of the info string.
			if i := strings.IndexAny(info, " \t"); i >= 0 {
				lang = info[:i]
			} else {
				lang = info
			}
		}
		code := c.textOf(node)
		base.NKind = KindCodeBlock
		base.TokCount = estimateTokens(code)
		return &CodeBlock{
			BaseNode: base,
			Language: lang,
			Info:     lang,
			Fenced:   true,
			Code:     code,
		}

	case *ast.CodeBlock:
		code := c.textOf(node)
		base.NKind = KindCodeBlock
		base.TokCount = estimateTokens(code)
		return &CodeBlock{
			BaseNode: base,
			Language: "",
			Fenced:   false,
			Code:     code,
		}

	case *ast.ThematicBreak:
		base.NKind = KindThematicBreak
		return &ThematicBreak{BaseNode: base}

	case *ast.HTMLBlock:
		raw := c.textOf(node)
		base.NKind = KindHTMLBlock
		base.TokCount = estimateTokens(raw)
		return &HTMLBlock{BaseNode: base, Raw: raw}

	case *extast.Table:
		base.NKind = KindTable
		tbl := &Table{BaseNode: base}
		for _, a := range node.Alignments {
			switch a {
			case extast.AlignLeft:
				tbl.Aligns = append(tbl.Aligns, AlignLeft)
			case extast.AlignRight:
				tbl.Aligns = append(tbl.Aligns, AlignRight)
			case extast.AlignCenter:
				tbl.Aligns = append(tbl.Aligns, AlignCenter)
			default:
				tbl.Aligns = append(tbl.Aligns, AlignNone)
			}
		}
		// Collect header titles from the first row of the header.
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if header, ok := child.(*extast.TableHeader); ok {
				for cell := header.FirstChild(); cell != nil; cell = cell.NextSibling() {
					tbl.Headers = append(tbl.Headers, c.inlineText(cell))
				}
				break
			}
		}
		c.convertChildrenInto(node, &tbl.Kids)
		tbl.TokCount = sumTokens(tbl.Kids)
		return tbl

	case *extast.TableHeader:
		base.NKind = KindTableRow
		row := &TableRow{BaseNode: base, IsHeader: true}
		c.convertChildrenInto(node, &row.Kids)
		row.TokCount = sumTokens(row.Kids)
		return row

	case *extast.TableRow:
		base.NKind = KindTableRow
		row := &TableRow{BaseNode: base}
		c.convertChildrenInto(node, &row.Kids)
		row.TokCount = sumTokens(row.Kids)
		return row

	case *extast.TableCell:
		base.NKind = KindTableCell
		cell := &TableCell{BaseNode: base}
		cell.TokCount = estimateTokens(c.inlineText(node))
		return cell

	case *extast.Footnote:
		id := string(node.Ref)
		base.NKind = KindFootnoteDef
		base.TokCount = estimateTokens(c.textOf(node))
		return &FootnoteDef{BaseNode: base, ID: id}

	case *extast.FootnoteList:
		// Unwrap: emit each footnote as its own top-level node.
		// We can't return multiple nodes here, so wrap in a definition list shim.
		// Caller (convertChildrenInto) handles the flattening via FootnoteList
		// being a structural container — treat it as transparent.
		return nil

	case *extast.DefinitionList:
		base.NKind = KindDefinitionList
		dl := &DefinitionList{BaseNode: base}
		c.convertChildrenInto(node, &dl.Kids)
		dl.TokCount = sumTokens(dl.Kids)
		return dl

	case *extast.DefinitionTerm:
		term := c.inlineText(node)
		base.NKind = KindDefTerm
		base.TokCount = estimateTokens(term)
		return &DefinitionTerm{BaseNode: base, Term: term}

	case *extast.DefinitionDescription:
		base.NKind = KindDefinition
		d := &Definition{BaseNode: base}
		c.convertChildrenInto(node, &d.Kids)
		d.TokCount = sumTokens(d.Kids)
		return d
	}

	return nil
}

// convertChildrenInto walks every block child of n, converts each to a Node,
// and appends to dst. FootnoteList is transparent — its children are flattened.
func (c *converter) convertChildrenInto(n ast.Node, dst *[]Node) {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if _, ok := child.(*extast.FootnoteList); ok {
			// Flatten: emit each footnote as a sibling of the FootnoteList.
			for fn := child.FirstChild(); fn != nil; fn = fn.NextSibling() {
				if node := c.convertBlock(fn); node != nil {
					*dst = append(*dst, node)
				}
			}
			continue
		}
		if node := c.convertBlock(child); node != nil {
			*dst = append(*dst, node)
		}
	}
}

// attachInline extracts inline constructs (autolinks, links, images, footnote
// refs, code spans, hard line breaks) from a paragraph-like goldmark node
// and attaches docmap-native Node children to parent.
func (c *converter) attachInline(n ast.Node, parent *Paragraph) {
	ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := node.(type) {
		case *ast.AutoLink:
			// Goldmark's *ast.AutoLink covers both `<url>` and GFM bare URLs
			// (via the Linkify extension). The AST doesn't distinguish the
			// two, so we inspect the paragraph's raw source: if the URL is
			// not wrapped in `<>`, it's a GFM bare-URL autolink.
			url := string(v.URL(c.source))
			isBare := v.AutoLinkType != ast.AutoLinkEmail &&
				!strings.Contains(parent.Raw, "<"+url+">")
			parent.Kids = append(parent.Kids, &AutoLink{
				BaseNode: BaseNode{NKind: KindAutoLink, Start: parent.Start, End: parent.End},
				URL:      url,
				IsMail:   v.AutoLinkType == ast.AutoLinkEmail,
				IsBare:   isBare,
			})
		case *ast.Link:
			parent.Kids = append(parent.Kids, &Link{
				BaseNode: BaseNode{NKind: KindLink, Start: parent.Start, End: parent.End},
				URL:      string(v.Destination),
				Title:    string(v.Title),
				Text:     c.inlineText(v),
			})
		case *ast.Image:
			parent.Kids = append(parent.Kids, &Image{
				BaseNode: BaseNode{NKind: KindImage, Start: parent.Start, End: parent.End},
				URL:      string(v.Destination),
				Title:    string(v.Title),
				Alt:      c.inlineText(v),
			})
		case *ast.CodeSpan:
			code := ""
			for ch := v.FirstChild(); ch != nil; ch = ch.NextSibling() {
				if tx, ok := ch.(*ast.Text); ok {
					code += string(tx.Segment.Value(c.source))
				}
			}
			parent.Kids = append(parent.Kids, &InlineCode{
				BaseNode: BaseNode{NKind: KindInlineCode, Start: parent.Start, End: parent.End},
				Code:     code,
			})
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			// Goldmark marks hard line breaks on Text nodes via SoftLineBreak()
			// + HardLineBreak(). A hard break is either a "two trailing spaces
			// then newline" or a "backslash newline".
			if v.HardLineBreak() {
				parent.Kids = append(parent.Kids, &LineBreak{
					BaseNode: BaseNode{NKind: KindLineBreak, Start: parent.Start, End: parent.End},
					Hard:     true,
				})
			}
		case *extast.FootnoteLink:
			parent.Kids = append(parent.Kids, &FootnoteRef{
				BaseNode: BaseNode{NKind: KindFootnoteRef, Start: parent.Start, End: parent.End},
				ID:       strconv.Itoa(v.Index),
			})
		}
		return ast.WalkContinue, nil
	})
}

// sumTokens returns the total token count across a node list (top-level only).
func sumTokens(nodes []Node) int {
	total := 0
	for _, n := range nodes {
		total += n.Tokens()
	}
	return total
}

// ---------- Post-pass: math blocks ----------

// detectMathBlocks rewrites Paragraph nodes whose raw source begins with $$
// or \[ fences into MathBlock nodes. Goldmark sees these as plain paragraphs.
func detectMathBlocks(nodes []Node, source []byte, lineOffset int) []Node {
	_ = lineOffset
	for i, n := range nodes {
		if p, ok := n.(*Paragraph); ok {
			if mb := tryMathBlock(p); mb != nil {
				nodes[i] = mb
				continue
			}
		}
		walkChildren(n, func(kids []Node) {
			detectMathBlocks(kids, source, lineOffset)
		})
	}
	return nodes
}

func tryMathBlock(p *Paragraph) Node {
	txt := strings.TrimRight(p.Raw, "\n")
	lines := strings.Split(txt, "\n")
	if len(lines) < 2 {
		return nil
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])

	if first == "$$" && last == "$$" {
		tex := strings.Join(lines[1:len(lines)-1], "\n")
		return &MathBlock{
			BaseNode: BaseNode{
				NKind:    KindMathBlock,
				Start:    p.Start,
				End:      p.End,
				TokCount: estimateTokens(tex),
			},
			TeX:   tex,
			Fence: "$$",
		}
	}
	if first == `\[` && last == `\]` {
		tex := strings.Join(lines[1:len(lines)-1], "\n")
		return &MathBlock{
			BaseNode: BaseNode{
				NKind:    KindMathBlock,
				Start:    p.Start,
				End:      p.End,
				TokCount: estimateTokens(tex),
			},
			TeX:   tex,
			Fence: `\[`,
		}
	}
	return nil
}

// ---------- Pre/post-scan: link reference definitions ----------

var linkRefDefRe = regexp.MustCompile(`(?m)^\s{0,3}\[([^\]]+)\]:\s*(\S+)(?:\s+"([^"]*)")?\s*$`)

// collectLinkRefDefs scans the body for `[label]: url "title"` lines,
// since goldmark absorbs these into its reference table and drops them
// from the AST.
func collectLinkRefDefs(source []byte) []Node {
	var out []Node
	for _, m := range linkRefDefRe.FindAllSubmatchIndex(source, -1) {
		label := string(source[m[2]:m[3]])
		url := string(source[m[4]:m[5]])
		var title string
		if m[6] >= 0 {
			title = string(source[m[6]:m[7]])
		}
		line := lineAt(source, m[0])
		out = append(out, &LinkRefDef{
			BaseNode: BaseNode{
				NKind:    KindLinkRefDef,
				Start:    line,
				End:      line,
				TokCount: estimateTokens(url + " " + title),
			},
			Label: label,
			URL:   url,
			Title: title,
		})
	}
	return out
}

// ---------- Post-pass: GFM callouts ----------

var calloutRe = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*`)

// detectCallouts walks the tree looking for Blockquotes whose first paragraph
// begins with a GFM alert marker, and upgrades them to Callout nodes.
func detectCallouts(nodes []Node) {
	for i, n := range nodes {
		if bq, ok := n.(*Blockquote); ok {
			if variant := calloutVariant(bq); variant != "" {
				c := &Callout{
					BaseNode: BaseNode{
						NKind:    KindCallout,
						Start:    bq.Start,
						End:      bq.End,
						TokCount: bq.TokCount,
						Kids:     bq.Kids,
					},
					Variant: variant,
				}
				nodes[i] = c
				detectCallouts(c.Kids)
				continue
			}
		}
		// Recurse.
		walkChildren(n, detectCallouts)
	}
}

func calloutVariant(bq *Blockquote) CalloutKind {
	if len(bq.Kids) == 0 {
		return ""
	}
	p, ok := bq.Kids[0].(*Paragraph)
	if !ok {
		return ""
	}
	first := strings.TrimSpace(strings.SplitN(p.Text, "\n", 2)[0])
	m := calloutRe.FindStringSubmatch(first)
	if m == nil {
		return ""
	}
	switch m[1] {
	case "NOTE":
		return CalloutNote
	case "TIP":
		return CalloutTip
	case "IMPORTANT":
		return CalloutImportant
	case "WARNING":
		return CalloutWarning
	case "CAUTION":
		return CalloutCaution
	}
	return ""
}

// ---------- Post-pass: inline extraction for Paragraphs, headings, etc. ----------

var (
	wikiEmbedRe = regexp.MustCompile(`!\[\[([^\]|#^]+)(?:#([^\]|^]*))?(?:\^([^\]|]+))?(?:\|([^\]]+))?\]\]`)
	wikiLinkRe  = regexp.MustCompile(`(^|[^!])\[\[([^\]|#^]+)(?:#([^\]|^]*))?(?:\^([^\]|]+))?(?:\|([^\]]+))?\]\]`)
	entityRe    = regexp.MustCompile(`&(?:#x[0-9A-Fa-f]+|#\d+|[A-Za-z][A-Za-z0-9]*);`)
	mentionRe   = regexp.MustCompile(`(^|[^\w/\\])@([A-Za-z0-9][A-Za-z0-9_-]{0,38})\b`)
	// Issue refs: require the preceding character to not be a word char, slash,
	// or ampersand — the last exclusion keeps HTML numeric entities like &#169;
	// from being misread as "issue 169".
	issueRefRe   = regexp.MustCompile(`(?:(^|[^\w/&])#(\d+))|(?:\b([A-Za-z][\w.-]*/[A-Za-z][\w.-]*)?#(\d+))|(?:\bGH-(\d+))`)
	commitRefRe  = regexp.MustCompile(`\b([0-9a-f]{7,40})\b`)
	emojiRe      = regexp.MustCompile(`:([a-z0-9_+-]{1,40}):`)
	inlineMathRe = regexp.MustCompile(`\$([^$\n]+)\$`)
	bareURLRe    = regexp.MustCompile(`https?://[^\s<>)\]]+`)
)

// postProcessInline walks every Paragraph/Heading and scans its text for
// inline constructs goldmark doesn't natively surface: wiki links, embeds,
// entities, mentions, issue refs, commit SHAs, emoji, inline math, bare URLs.
func postProcessInline(nodes []Node) {
	for _, n := range nodes {
		switch v := n.(type) {
		case *Paragraph:
			// Raw preserves newlines from the original source, which matters
			// for wiki link regexes and for the bare-URL scan to see the
			// full text of multi-line paragraphs.
			blob := v.Raw
			if blob == "" {
				blob = v.Text
			}
			v.Kids = append(v.Kids, scanInline(blob, v.Start, v.Kids)...)
		case *Heading:
			v.Kids = append(v.Kids, scanInline(v.Title, v.Start, v.Kids)...)
		}
		walkChildren(n, func(kids []Node) { postProcessInline(kids) })
	}
}

// scanInline looks for wiki links, embeds, entities, mentions, issue refs,
// commit SHAs, emoji shortcodes, inline math, and GFM bare URLs in a text blob.
// existing is consulted so we don't double-emit things goldmark already gave
// us as children (inline links, angle-bracket autolinks).
func scanInline(text string, line int, existing []Node) []Node {
	var out []Node
	base := func(k NodeKind) BaseNode {
		return BaseNode{NKind: k, Start: line, End: line}
	}

	// Collect URLs that goldmark already created Link/AutoLink nodes for,
	// so the bare-URL scan below doesn't double-count them.
	seenURLs := map[string]bool{}
	for _, n := range existing {
		switch v := n.(type) {
		case *Link:
			seenURLs[v.URL] = true
		case *AutoLink:
			seenURLs[v.URL] = true
		}
	}

	// Wiki embeds (![[...]]) — match first so the wiki-link regex doesn't re-catch them.
	for _, m := range wikiEmbedRe.FindAllStringSubmatch(text, -1) {
		target := strings.TrimSpace(m[1])
		size := m[4]
		w, h := parseEmbedSize(size)
		out = append(out, &WikiEmbed{
			BaseNode: base(KindWikiEmbed),
			Target:   target,
			Width:    w,
			Height:   h,
		})
	}

	// Wiki links ([[...]]) — only if not preceded by '!'.
	for _, m := range wikiLinkRe.FindAllStringSubmatch(text, -1) {
		target := strings.TrimSpace(m[2])
		anchor := m[3]
		block := m[4]
		alias := m[5]
		out = append(out, &WikiLink{
			BaseNode: base(KindWikiLink),
			Target:   target,
			Anchor:   anchor,
			Block:    block,
			Alias:    alias,
		})
	}

	// HTML entities.
	for _, m := range entityRe.FindAllString(text, -1) {
		out = append(out, &Entity{
			BaseNode: base(KindEntity),
			Raw:      m,
		})
	}

	// @mentions.
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		out = append(out, &Mention{
			BaseNode: base(KindMention),
			User:     m[2],
		})
	}

	// Issue references: #123, GH-123, user/repo#123.
	for _, m := range issueRefRe.FindAllStringSubmatch(text, -1) {
		var numStr, repo string
		switch {
		case m[2] != "":
			numStr = m[2]
		case m[4] != "":
			numStr = m[4]
			repo = m[3]
		case m[5] != "":
			numStr = m[5]
		}
		if numStr == "" {
			continue
		}
		n, _ := strconv.Atoi(numStr)
		out = append(out, &IssueRef{
			BaseNode: base(KindIssueRef),
			Repo:     repo,
			Number:   n,
		})
	}

	// Commit SHA autolinks (7-40 hex chars).
	for _, m := range commitRefRe.FindAllString(text, -1) {
		// Avoid matching inside other tokens: the regex already uses \b,
		// but also require the token is long enough to look like a SHA.
		if len(m) >= 7 {
			out = append(out, &CommitRef{
				BaseNode: base(KindCommitRef),
				SHA:      m,
			})
		}
	}

	// Emoji shortcodes.
	for _, m := range emojiRe.FindAllStringSubmatch(text, -1) {
		out = append(out, &Emoji{
			BaseNode: base(KindEmoji),
			Name:     m[1],
		})
	}

	// Inline math.
	for _, m := range inlineMathRe.FindAllStringSubmatch(text, -1) {
		out = append(out, &InlineMath{
			BaseNode: base(KindInlineMath),
			TeX:      strings.TrimSpace(m[1]),
		})
	}

	// GFM bare URLs (not already covered by an inline link or angle autolink).
	for _, url := range bareURLRe.FindAllString(text, -1) {
		// Trim trailing punctuation commonly glued to URLs in prose.
		url = strings.TrimRight(url, ".,;:!?)")
		if seenURLs[url] {
			continue
		}
		seenURLs[url] = true
		out = append(out, &AutoLink{
			BaseNode: base(KindAutoLink),
			URL:      url,
			IsBare:   true,
		})
	}

	return out
}

func parseEmbedSize(s string) (int, int) {
	if s == "" {
		return 0, 0
	}
	if i := strings.Index(s, "x"); i >= 0 {
		w, _ := strconv.Atoi(s[:i])
		h, _ := strconv.Atoi(s[i+1:])
		return w, h
	}
	w, _ := strconv.Atoi(s)
	return w, 0
}

// walkChildren invokes fn on every Node's children that live behind one of
// the typed fields we use. It's a helper so post-passes can recurse without
// each switch arm being duplicated.
func walkChildren(n Node, fn func([]Node)) {
	switch v := n.(type) {
	case *Blockquote:
		fn(v.Kids)
	case *Callout:
		fn(v.Kids)
	case *List:
		fn(v.Kids)
	case *ListItem:
		fn(v.Kids)
	case *TaskItem:
		fn(v.Kids)
	case *Table:
		fn(v.Kids)
	case *TableRow:
		fn(v.Kids)
	case *DefinitionList:
		fn(v.Kids)
	case *Definition:
		fn(v.Kids)
	}
}
