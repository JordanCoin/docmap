package parser

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/ledongthuc/pdf"
)

// ParsePDF parses a PDF file into a Document structure. Outlined PDFs get
// their bookmark tree anchored to the text layer; PDFs without an outline get
// one section per page. Either way doc.Source holds the extracted text and
// section line numbers index into it, so --expand, --at and --search work.
func ParsePDF(filepath string) (*Document, error) {
	f, r, err := openPDF(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	doc := &Document{}
	pages := pdfPageTexts(filepath, r)

	// Try to extract outline (bookmarks) first
	sections, dests := readOutline(r)
	if len(sections) == 0 {
		// Our reader gave up on a malformed outline; take the library's.
		if outline := r.Outline(); hasOutline(outline) {
			sections, dests = parseOutline(outline), nil
		}
	}
	if len(sections) > 0 {
		doc.Sections = sections
		text, layout := joinPages(pages)
		if !anchorSections(doc, text, layout, dests) {
			// Too few titles found in the text layer (scanned PDF, or an
			// outline that does not match the body): keep the tree, estimate tokens.
			addPageTokens(doc, text)
		}
	} else {
		// Fall back to page-based structure
		doc.Sections, doc.Source = pageSections(pages)
	}

	// Calculate total tokens
	for _, s := range doc.GetAllSections() {
		doc.TotalTokens += s.Tokens
	}

	return doc, nil
}

func openPDF(path string) (*os.File, *pdf.Reader, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	return f, r, nil
}

// PDFTextCommandEnv names an external text extractor. When set, its value is
// a command line that receives the PDF path (substituted for "{}", or
// appended when there is no "{}") and writes plain text to stdout, with a
// form feed between pages if it wants page structure preserved:
//
//	DOCMAP_PDF_TEXT_CMD="pdftotext -layout {} -"
//
// docmap keeps its own outline structure and anchors it to that text (#34).
// The value is split on whitespace and run directly, never through a shell.
// If the command fails, docmap says so on stderr and uses its built-in
// extractor.
const PDFTextCommandEnv = "DOCMAP_PDF_TEXT_CMD"

var warnExternalOnce sync.Once

// pdfPageTexts returns the text of every page, index 0 being page 1.
func pdfPageTexts(path string, r *pdf.Reader) []pdfPage {
	if cmdline := strings.Fields(os.Getenv(PDFTextCommandEnv)); len(cmdline) > 0 {
		pages, err := externalPageTexts(cmdline, path)
		if err == nil {
			return pages
		}
		warnExternalOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "docmap: %s failed (%v); using the built-in PDF text extractor\n", PDFTextCommandEnv, err)
		})
	}
	pages := make([]pdfPage, r.NumPage())
	for i := range pages {
		pages[i] = pageText(r.Page(i + 1))
	}
	return pages
}

func externalPageTexts(cmdline []string, path string) ([]pdfPage, error) {
	args := make([]string, 0, len(cmdline))
	substituted := false
	for _, a := range cmdline[1:] {
		if strings.Contains(a, "{}") {
			a = strings.ReplaceAll(a, "{}", path)
			substituted = true
		}
		args = append(args, a)
	}
	if !substituted {
		args = append(args, path)
	}
	out, err := exec.Command(cmdline[0], args...).Output()
	if err != nil {
		return nil, err
	}
	text := strings.ToValidUTF8(strings.ReplaceAll(string(out), "\r\n", "\n"), replacement)
	parts := strings.Split(text, "\f")
	if n := len(parts); n > 1 && strings.TrimSpace(parts[n-1]) == "" {
		parts = parts[:n-1]
	}
	pages := make([]pdfPage, len(parts))
	for i, p := range parts {
		pages[i] = pdfPage{text: strings.Trim(p, "\n")}
	}
	return pages, nil
}

// pdfLayout describes where the lines of a joined PDF text came from.
type pdfLayout struct {
	pageLo    []int     // byte offset where each page's text starts; one extra entry = len(text)
	lineStart []int     // byte offset of each line
	lineY     []float64 // baseline y of each line, NaN when unknown
}

// lineAt returns the index of the line containing byte offset off.
func (l *pdfLayout) lineAt(off int) int {
	return sort.Search(len(l.lineStart), func(i int) bool { return l.lineStart[i] > off }) - 1
}

// pageAt returns the 0-based page containing byte offset off.
func (l *pdfLayout) pageAt(off int) int {
	return sort.Search(len(l.pageLo)-1, func(i int) bool { return l.pageLo[i+1] > off })
}

// joinPages concatenates page texts, one page per line at minimum, and
// records which bytes and lines belong to which page.
func joinPages(pages []pdfPage) (string, *pdfLayout) {
	var b strings.Builder
	lay := &pdfLayout{}
	for _, pg := range pages {
		lay.pageLo = append(lay.pageLo, b.Len())
		if pg.text == "" {
			continue
		}
		for i, line := range strings.Split(pg.text, "\n") {
			lay.lineStart = append(lay.lineStart, b.Len())
			y := math.NaN()
			if i < len(pg.ys) {
				y = pg.ys[i]
			}
			lay.lineY = append(lay.lineY, y)
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	lay.pageLo = append(lay.pageLo, b.Len())
	return b.String(), lay
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
	for i := range pos {
		pos[i] = -1
	}
	anchorRemaining(norm, keys, pos, lens)
	return pos, lens
}

// anchorRemaining runs the three text-matching passes for every title that
// pos does not already place (pos[i] < 0), treating the ones it does as
// fixed points.
func anchorRemaining(norm []byte, keys [][]byte, pos, lens []int) {
	variants := make([][][]byte, len(keys))
	taken := map[int]bool{}
	for i, k := range keys {
		variants[i] = keyVariants(k)
		if pos[i] >= 0 {
			taken[pos[i]] = true
		}
	}
	place := func(i, at, n int) {
		pos[i], lens[i] = at, n
		taken[at] = true
	}

	cursor := 0
	for i, k := range keys {
		if pos[i] >= 0 {
			// Only move forward: outlines list appendices and figure
			// captions out of reading order.
			if pos[i]+lens[i] > cursor {
				cursor = pos[i] + lens[i]
			}
			continue
		}
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
}

// destTolerance is how far above its destination (in points) a heading's
// baseline may sit and still be the heading the destination points at.
const destTolerance = 4

// anchorByDest places titles using the outline's own destinations: each
// title is searched for only on the page it points to (and the next, since a
// destination can sit at the very bottom of a page), and when the destination
// carries a y coordinate the occurrence nearest below it wins. This is what
// keeps a section off the table-of-contents line, running header or figure
// label that happens to repeat its title.
func anchorByDest(text string, norm []byte, idx []int, keys [][]byte, lay *pdfLayout, dests []outlineDest, pos, lens []int) {
	if lay == nil || len(dests) != len(keys) || len(lay.pageLo) < 2 {
		return
	}
	// normAt maps a raw byte offset to the first normalized byte at or after it.
	normAt := func(off int) int {
		return sort.Search(len(idx), func(i int) bool { return idx[i] >= off })
	}
	pages := len(lay.pageLo) - 1
	taken := map[int]bool{}
	for i, d := range dests {
		if d.page < 0 || d.page >= pages || len(keys[i]) == 0 {
			continue
		}
		last := d.page + 1
		if last >= pages {
			last = pages - 1
		}
		lo, hi := normAt(lay.pageLo[d.page]), normAt(lay.pageLo[last+1])
		best, bestLen, bestCost := -1, 0, math.Inf(1)
		for _, v := range keyVariants(keys[i]) {
			for from := lo; from < hi; {
				j := bytes.Index(norm[from:hi], v)
				if j < 0 {
					break
				}
				at := from + j
				from = at + 1
				if taken[at] {
					continue
				}
				raw := idx[at]
				line := lay.lineAt(raw)
				cost := float64(at-lo) / float64(hi-lo+1) // earlier wins a tie
				if lay.pageAt(raw) != d.page {
					cost += 5000
				}
				// A heading starts its line, give or take a section number.
				lineStart := lay.lineStart[line]
				if strings.TrimLeft(text[lineStart:raw], "0123456789. \tABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
					cost += 300
				}
				if y := lay.lineY[line]; d.hasTop && !math.IsNaN(y) {
					below := d.top - y
					if below < -destTolerance {
						cost += 1000 - below // above the destination: almost never the heading
					} else {
						cost += below
					}
				}
				if cost < bestCost {
					best, bestLen, bestCost = at, len(v), cost
				}
			}
			if best >= 0 {
				break // longest variant found; stripped ones are a fallback
			}
		}
		if best >= 0 {
			pos[i], lens[i] = best, bestLen
			taken[best] = true
		}
	}
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
	// "3.2.1 Scaled Dot-Product Attention": the outline title has no number
	// but the heading does. Take the number with the heading rather than
	// leaving it as the last line of the previous section.
	ls := strings.LastIndexByte(text[:at], '\n') + 1
	if prefix := text[ls:at]; len(prefix) > 0 && len(prefix) <= 12 &&
		strings.Trim(prefix, "0123456789. ") == "" && strings.ContainsAny(prefix, "0123456789") {
		return ls
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
func anchorSections(doc *Document, text string, lay *pdfLayout, dests []outlineDest) bool {
	flat := doc.GetAllSections()
	if len(flat) == 0 || text == "" {
		return false
	}

	norm, idx := normalizeForMatch(text)
	keys := make([][]byte, len(flat))
	for i, s := range flat {
		keys[i], _ = normalizeForMatch(s.Title)
	}
	pos := make([]int, len(keys))
	lens := make([]int, len(keys))
	for i := range pos {
		pos[i] = -1
	}
	anchorByDest(text, norm, idx, keys, lay, dests, pos, lens)
	anchorRemaining(norm, keys, pos, lens)

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

// pageSections builds the page-by-page structure for PDFs without an
// outline. It returns the sections and the source text they index into: page
// texts joined by a blank line, with each section's LineStart and LineEnd
// pointing at its page's lines.
func pageSections(pages []pdfPage) ([]*Section, string) {
	if len(pages) == 0 {
		return nil, ""
	}
	var sections []*Section
	var src strings.Builder
	line := 1
	for i, pg := range pages {
		text := strings.TrimSpace(pg.text)
		if text == "" {
			continue
		}
		if src.Len() > 0 {
			src.WriteString("\n")
			line++
		}
		n := strings.Count(text, "\n") + 1
		sections = append(sections, &Section{
			Level:     1,
			Title:     fmt.Sprintf("Page %d", i+1),
			Content:   text,
			Tokens:    estimateTokens(text),
			LineStart: line,
			LineEnd:   line + n - 1,
		})
		src.WriteString(text)
		src.WriteString("\n")
		line += n
	}

	// If no text content was extracted, return a warning section
	if len(sections) == 0 {
		return []*Section{{
			Level:     1,
			Title:     fmt.Sprintf("(%d pages - no extractable text)", len(pages)),
			LineStart: 1,
			LineEnd:   len(pages),
		}}, ""
	}
	return sections, src.String()
}
