package parser

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ParsePDF parses a PDF file into a Document structure
func ParsePDF(filepath string) (*Document, error) {
	f, r, err := pdf.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	doc := &Document{}

	// Try to extract outline (bookmarks) first
	outline := r.Outline()
	if hasOutline(outline) {
		doc.Sections = parseOutline(outline)
		text := extractText(r)
		if !anchorSections(doc, text) {
			// Too few titles found in the text layer (scanned PDF, or an
			// outline that does not match the body): keep the tree, estimate tokens.
			addPageTokens(doc, text)
		}
	} else {
		// Fall back to page-based structure
		doc.Sections = parseByPage(r)
	}

	// Calculate total tokens
	for _, s := range doc.GetAllSections() {
		doc.TotalTokens += s.Tokens
	}

	return doc, nil
}

// hasOutline checks if the PDF has a meaningful outline structure
func hasOutline(outline pdf.Outline) bool {
	return len(outline.Child) > 0
}

// parseOutline extracts document structure from PDF bookmarks
func parseOutline(outline pdf.Outline) []*Section {
	var sections []*Section

	for _, item := range outline.Child {
		section := outlineItemToSection(item, 1)
		sections = append(sections, section)
	}

	return sections
}

// outlineItemToSection converts a PDF outline item to a Section
func outlineItemToSection(item pdf.Outline, level int) *Section {
	section := &Section{
		Level: level,
		Title: strings.TrimSpace(item.Title),
	}

	// Process children recursively
	for _, child := range item.Child {
		childSection := outlineItemToSection(child, level+1)
		childSection.Parent = section
		section.Children = append(section.Children, childSection)
	}

	return section
}

// extractText concatenates the plain text of every page, one page per line
// at minimum. Pages that fail to decode are skipped.
func extractText(r *pdf.Reader) string {
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil || text == "" {
			continue
		}
		b.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// distinctiveTitleLen is the normalized length above which a title is
// trusted to anchor on its own. Shorter titles (Results, Methods) recur in
// prose and are placed relative to their neighbours instead.
const distinctiveTitleLen = 12

// ligatures are letter runs a PDF font may draw as one glyph with no
// ToUnicode entry, so the text layer yields a control byte where the letters
// belong. Longest first so "ffi" is not split into "ff" + "i".
var ligatures = []string{"ffi", "ffl", "ff", "fi", "fl", "th"}

// maxLigatureSites caps the 2^n variant fan-out of keyVariants.
const maxLigatureSites = 6

// normalizeForMatch lowercases s and keeps only [a-z0-9], returning the
// normalized bytes and, for each, the byte offset it came from in s. Outline
// titles and text layers disagree on spaces, hyphen variants, quotes and
// stray control bytes; dropping everything else sidesteps all of them.
func normalizeForMatch(s string) (norm []byte, idx []int) {
	norm = make([]byte, 0, len(s))
	idx = make([]int, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			c += 'a' - 'A'
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		default:
			continue
		}
		norm = append(norm, c)
		idx = append(idx, i)
	}
	return norm, idx
}

// keyVariants returns key followed by every copy of it with some subset of
// its ligature sites deleted, longest first, so an exact match always wins
// and a stripped variant is only consulted when the font really dropped the
// glyph. Past maxLigatureSites only the fully stripped copy is added.
func keyVariants(key []byte) [][]byte {
	type site struct{ at, n int }
	var sites []site
	for i := 0; i < len(key); {
		matched := false
		for _, lig := range ligatures {
			if bytes.HasPrefix(key[i:], []byte(lig)) {
				sites = append(sites, site{i, len(lig)})
				i += len(lig)
				matched = true
				break
			}
		}
		if !matched {
			i++
		}
	}
	strip := func(mask int) []byte {
		out := make([]byte, 0, len(key))
		prev := 0
		for b, s := range sites {
			if mask&(1<<b) == 0 {
				continue
			}
			out = append(out, key[prev:s.at]...)
			prev = s.at + s.n
		}
		return append(out, key[prev:]...)
	}
	variants := [][]byte{key}
	if len(sites) == 0 {
		return variants
	}
	if len(sites) > maxLigatureSites {
		return append(variants, strip(1<<len(sites)-1))
	}
	for mask := 1; mask < 1<<len(sites); mask++ {
		variants = append(variants, strip(mask))
	}
	sort.SliceStable(variants, func(a, b int) bool { return len(variants[a]) > len(variants[b]) })
	return variants
}

// firstMatch returns the start and length of the earliest occurrence of the
// longest variant found in norm[lo:hi], or -1, 0.
func firstMatch(norm []byte, variants [][]byte, lo, hi int) (int, int) {
	for _, v := range variants {
		if j := bytes.Index(norm[lo:hi], v); j >= 0 {
			return lo + j, len(v)
		}
	}
	return -1, 0
}

// lastMatch is firstMatch for the latest occurrence.
func lastMatch(norm []byte, variants [][]byte, lo, hi int) (int, int) {
	for _, v := range variants {
		if j := bytes.LastIndex(norm[lo:hi], v); j >= 0 {
			return lo + j, len(v)
		}
	}
	return -1, 0
}

// anchorTitles finds each title's start offset and matched length in norm,
// or -1 when it cannot be placed. Titles arrive in outline order.
//
// Pass one anchors distinctive titles in a forward scan, each searched after
// the previous anchor, so a title that also occurs earlier as a prose mention
// or a running header is still placed in sequence. Pass two retries the
// distinctive titles that scan missed anywhere in the text, because outlines
// list figure captions and appendices out of reading order. Pass three places
// each short title inside the window between its anchored neighbours, taking
// the last occurrence: a heading sits right before its body and its first
// subsection, so the last candidate before the next anchor is the heading and
// earlier candidates are prose mentions.
//
// ponytail: first-occurrence matching. A distinctive title that the
// introduction quotes verbatim anchors on the quote, and every short title
// between it and the real heading then has no window to land in. Upgrade
// path: read the outline's page destinations and restrict each search to
// its page.
func anchorTitles(norm []byte, keys [][]byte) (pos, lens []int) {
	pos = make([]int, len(keys))
	lens = make([]int, len(keys))
	variants := make([][][]byte, len(keys))
	for i, k := range keys {
		pos[i] = -1
		variants[i] = keyVariants(k)
	}
	taken := map[int]bool{}
	place := func(i, at, n int) {
		pos[i], lens[i] = at, n
		taken[at] = true
	}

	cursor := 0
	for i, k := range keys {
		if len(k) < distinctiveTitleLen {
			continue
		}
		if at, n := firstMatch(norm, variants[i], cursor, len(norm)); at >= 0 {
			place(i, at, n)
			cursor = at + n
		}
	}

	for i, k := range keys {
		if pos[i] >= 0 || len(k) < distinctiveTitleLen {
			continue
		}
		for from := 0; from < len(norm); {
			at, n := firstMatch(norm, variants[i], from, len(norm))
			if at < 0 {
				break
			}
			if !taken[at] {
				place(i, at, n)
				break
			}
			from = at + 1
		}
	}

	for i, k := range keys {
		if pos[i] >= 0 || len(k) == 0 {
			continue
		}
		lo, hi := 0, len(norm)
		for p := i - 1; p >= 0; p-- {
			if pos[p] >= 0 {
				lo = pos[p] + lens[p]
				break
			}
		}
		for n := i + 1; n < len(keys); n++ {
			if pos[n] >= 0 {
				hi = pos[n]
				break
			}
		}
		if lo >= hi {
			continue
		}
		if at, n := lastMatch(norm, variants[i], lo, hi); at >= 0 {
			place(i, at, n)
		}
	}
	return pos, lens
}

// headingStart backs up from the first matched byte over letters and control
// bytes, so a heading whose leading ligature was dropped by the font ("The"
// drawn as one glyph) still starts at its first glyph.
func headingStart(text string, at int) int {
	for at > 0 {
		c := text[at-1]
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isControl := c < 0x20 && c != '\n' && c != '\r' && c != '\t'
		if !isLetter && !isControl {
			break
		}
		at--
	}
	return at
}

// fillParentRange gives a section with anchored descendants the line range of
// its whole subtree, so --at resolves through an unanchored parent instead of
// treating it as starting at line 0.
func fillParentRange(s *Section) {
	for _, child := range s.Children {
		fillParentRange(child)
		if child.LineStart == 0 {
			continue
		}
		if s.LineStart == 0 || child.LineStart < s.LineStart {
			s.LineStart = child.LineStart
		}
		if child.LineEnd > s.LineEnd {
			s.LineEnd = child.LineEnd
		}
	}
}

// anchorSections locates every outline title in text and fills Content,
// LineStart, LineEnd and Tokens for the sections it can place. doc.Source is
// set to text with a line break inserted before each anchored heading, so
// line numbers, SourceSpan and --at share one coordinate space and a heading
// never shares a line with the section before it. A section's content runs to
// the next anchored heading in text order, so a parent's own content stops
// where its first child begins. Returns false, leaving the document
// untouched, when fewer than half the titles anchor.
func anchorSections(doc *Document, text string) bool {
	flat := doc.GetAllSections()
	if len(flat) == 0 || text == "" {
		return false
	}

	norm, idx := normalizeForMatch(text)
	keys := make([][]byte, len(flat))
	for i, s := range flat {
		keys[i], _ = normalizeForMatch(s.Title)
	}
	pos, lens := anchorTitles(norm, keys)

	type span struct {
		sec        *Section
		start, end int // raw byte offsets of the heading in text
	}
	var spans []span
	for i, p := range pos {
		if p < 0 {
			continue
		}
		spans = append(spans, span{flat[i], headingStart(text, idx[p]), idx[p+lens[i]-1] + 1})
	}
	if len(spans)*2 < len(flat) {
		return false
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].start < spans[b].start })

	var src strings.Builder
	src.Grow(len(text) + len(spans))
	line := 1
	prev := 0
	starts := make([]int, len(spans)) // line of each heading in src
	for i, sp := range spans {
		chunk := text[prev:sp.start]
		src.WriteString(chunk)
		line += strings.Count(chunk, "\n")
		if sp.start > 0 && text[sp.start-1] != '\n' {
			src.WriteByte('\n')
			line++
		}
		starts[i] = line
		prev = sp.start
	}
	src.WriteString(text[prev:])
	doc.Source = src.String()

	for i, sp := range spans {
		end := len(text)
		if i+1 < len(spans) {
			end = spans[i+1].start
		}
		if sp.end > end {
			sp.end = end
		}
		body := strings.TrimLeft(strings.TrimSpace(text[sp.end:end]), ".:")
		sp.sec.Content = strings.TrimSpace(body)
		sp.sec.Tokens = estimateTokens(sp.sec.Content)
		sp.sec.LineStart = starts[i]
		sp.sec.LineEnd = starts[i] + strings.Count(strings.TrimRight(text[sp.start:end], "\n"), "\n")
	}

	for _, root := range doc.Sections {
		fillParentRange(root)
		calculateCumulativeTokens(root)
	}
	return true
}

// addPageTokens adds token estimates to outlined documents from the whole text
func addPageTokens(doc *Document, allText string) {
	totalTokens := estimateTokens(allText)

	// Distribute tokens across top-level sections proportionally
	if len(doc.Sections) > 0 && totalTokens > 0 {
		tokensPerSection := totalTokens / len(doc.Sections)
		for _, section := range doc.Sections {
			distributeTokens(section, tokensPerSection)
		}
	}
}

// distributeTokens assigns token estimates to a section and its children
func distributeTokens(section *Section, tokens int) {
	if len(section.Children) == 0 {
		section.Tokens = tokens
		return
	}

	// Give parent a portion, distribute rest to children
	childCount := len(section.Children)
	perChild := tokens / (childCount + 1)
	section.Tokens = perChild

	for _, child := range section.Children {
		distributeTokens(child, perChild)
	}

	// Recalculate cumulative tokens
	calculateCumulativeTokens(section)
}

// parseByPage creates a page-based structure for PDFs without outlines
func parseByPage(r *pdf.Reader) []*Section {
	numPages := r.NumPage()
	if numPages == 0 {
		return nil
	}

	var sections []*Section
	var hasContent bool

	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		hasContent = true
		section := &Section{
			Level:     1,
			Title:     fmt.Sprintf("Page %d", i),
			Content:   text,
			Tokens:    estimateTokens(text),
			LineStart: i, // Page number
			LineEnd:   i,
		}
		sections = append(sections, section)
	}

	// If no text content was extracted, return a warning section
	if !hasContent && numPages > 0 {
		return []*Section{{
			Level:     1,
			Title:     fmt.Sprintf("(%d pages - no extractable text)", numPages),
			Content:   "",
			Tokens:    0,
			LineStart: 1,
			LineEnd:   numPages,
		}}
	}

	return sections
}
