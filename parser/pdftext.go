package parser

import (
	"bytes"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// This file is docmap's own PDF text extractor. It walks a page's content
// stream with the library's lexer but does its own decoding and layout,
// because the library's GetPlainText:
//
//   - concatenates glyphs with no regard for position, so PDFs that space
//     words by moving the pen (every TeX document) come out as one unbroken
//     string of letters;
//   - ignores a font's ToUnicode map whenever the font also has an Encoding
//     dictionary, and ignores that dictionary's BaseEncoding, which turns
//     ligature glyphs and WinAnsi punctuation (en dash = 0x96) into raw
//     control bytes (#34).
//
// The extractor tracks the text matrix, turns horizontal gaps into spaces and
// baseline changes into line breaks, decodes through ToUnicode first, and
// never lets an undecodable glyph vanish: it becomes U+FFFD so the damage is
// visible instead of silently fusing its neighbours.

// replacement marks a glyph whose Unicode value could not be determined.
const replacement = "�"

// cp1252High maps bytes 0x80..0x9F in Windows-1252 (WinAnsiEncoding). The
// rest of the encoding is Latin-1.
var cp1252High = [32]rune{
	0x20AC, 0xFFFD, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0xFFFD, 0x017D, 0xFFFD,
	0xFFFD, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0xFFFD, 0x017E, 0x0178,
}

// ligatureText expands Unicode presentation-form ligatures, which PDF fonts
// use for a single glyph, into the letters a reader would search for.
var ligatureText = map[rune]string{
	0xFB00: "ff", 0xFB01: "fi", 0xFB02: "fl", 0xFB03: "ffi", 0xFB04: "ffl",
	0xFB05: "st", 0xFB06: "st",
}

// cleanGlyphText normalizes one decoded glyph: ligatures are expanded,
// Windows-1252 bytes that leaked through as C1 controls are mapped to the
// punctuation they stand for, and any other control character becomes
// U+FFFD. Soft hyphens and zero-width characters are dropped.
func cleanGlyphText(s string) string {
	clean := true
	for _, r := range s {
		if r < 0x20 || (r >= 0x7F && r < 0xA0) || r == 0xAD || (r >= 0xFB00 && r <= 0xFB06) ||
			r == 0x200B || r == 0xFEFF || r == utf8.RuneError {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r >= 0xFB00 && r <= 0xFB06:
			b.WriteString(ligatureText[r])
		case r >= 0x80 && r < 0xA0:
			b.WriteRune(cp1252High[r-0x80])
		case r < 0x20 || r == 0x7F || r == utf8.RuneError:
			b.WriteString(replacement)
		case r == 0xAD || r == 0x200B || r == 0xFEFF:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// glyphNameText resolves an Adobe glyph name the library's table does not
// know, following the Adobe Glyph List specification: a ".suffix" is dropped,
// "_" joins ligature components, and uniXXXX / uXXXX[XX] spell code points.
// Components must be single letters or digits ("T_h", "f_f_i") or uni names.
// Returns "" when the name cannot be resolved.
func glyphNameText(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, part := range strings.Split(name, "_") {
		switch {
		case len(part) == 1 && (part[0] >= 'A' && part[0] <= 'Z' || part[0] >= 'a' && part[0] <= 'z'):
			b.WriteString(part)
		case strings.HasPrefix(part, "uni") && len(part) >= 7 && (len(part)-3)%4 == 0:
			for i := 3; i < len(part); i += 4 {
				n, err := strconv.ParseUint(part[i:i+4], 16, 32)
				if err != nil {
					return ""
				}
				b.WriteRune(rune(n))
			}
		case part[0] == 'u' && len(part) >= 5 && len(part) <= 7:
			n, err := strconv.ParseUint(part[1:], 16, 32)
			if err != nil {
				return ""
			}
			b.WriteRune(rune(n))
		default:
			if s, ok := extraGlyphNames[part]; ok {
				b.WriteString(s)
			} else {
				return ""
			}
		}
	}
	return b.String()
}

// extraGlyphNames are names seen in real fonts that are not in the Adobe
// Glyph List proper (TeX and Type 1 spellings of ligatures and spaces).
var extraGlyphNames = map[string]string{
	"ff": "ff", "fi": "fi", "fl": "fl", "ffi": "ffi", "ffl": "ffl",
	"thinspace": " ", "nbspace": " ", "visiblespace": " ", "space": " ",
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4",
	"five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9",
	"hyphen": "-", "period": ".", "comma": ",", "endash": "–", "emdash": "—",
}

// toUnicodeMap is a parsed ToUnicode CMap.
type toUnicodeMap struct {
	chars  map[string]string
	ranges []cmapRange
}

type cmapRange struct {
	lo, hi string
	dst    string   // UTF-16BE of the first destination, or
	arr    []string // one UTF-16BE destination per code
}

func utf16BE(s string) string {
	if len(s) < 2 {
		return ""
	}
	u := make([]uint16, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		u = append(u, uint16(s[i])<<8|uint16(s[i+1]))
	}
	return string(utf16.Decode(u))
}

// parseToUnicode reads a ToUnicode CMap stream. It returns nil when the
// stream defines no mappings.
func parseToUnicode(strm pdf.Value) *toUnicodeMap {
	rd := strm.Reader()
	defer rd.Close()
	data, err := io.ReadAll(io.LimitReader(rd, 8<<20))
	if err != nil && len(data) == 0 {
		return nil
	}
	return parseCMap(data)
}

// parseCMap scans CMap source for bfchar and bfrange blocks. A CMap is
// PostScript, but the mappings are flat runs of hex strings between
// begin/end keywords, so a token scan reads them without an interpreter
// (the library's interpreter rejects the standard "findresource begin"
// prologue unless the caller can fabricate a dictionary).
func parseCMap(data []byte) *toUnicodeMap {
	m := &toUnicodeMap{chars: map[string]string{}}
	var (
		mode  string   // "char", "range" or ""
		items []string // decoded hex strings; "[" and "]" delimit an array
	)
	flush := func() {
		switch mode {
		case "char":
			for i := 0; i+1 < len(items); i += 2 {
				m.chars[items[i]] = utf16BE(items[i+1])
			}
		case "range":
			for i := 0; i+2 < len(items); {
				r := cmapRange{lo: items[i], hi: items[i+1]}
				i += 2
				if items[i] == "[" {
					for i++; i < len(items) && items[i] != "]"; i++ {
						r.arr = append(r.arr, utf16BE(items[i]))
					}
					i++
				} else {
					r.dst = items[i]
					i++
				}
				if len(r.lo) > 0 && len(r.lo) == len(r.hi) {
					m.ranges = append(m.ranges, r)
				}
			}
		}
		mode, items = "", nil
	}
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == '%':
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
		case c == '<' && i+1 < len(data) && data[i+1] != '<':
			j := bytes.IndexByte(data[i:], '>')
			if j < 0 {
				i = len(data)
				break
			}
			if mode != "" {
				items = append(items, unhex(data[i+1:i+j]))
			}
			i += j + 1
		case c == '[' || c == ']':
			if mode == "range" {
				items = append(items, string(c))
			}
			i++
		case c >= 'a' && c <= 'z':
			j := i
			for j < len(data) && (data[j] >= 'a' && data[j] <= 'z') {
				j++
			}
			switch string(data[i:j]) {
			case "beginbfchar":
				flush()
				mode = "char"
			case "beginbfrange":
				flush()
				mode = "range"
			case "endbfchar", "endbfrange":
				flush()
			}
			i = j
		default:
			i++
		}
	}
	flush()
	if len(m.chars) == 0 && len(m.ranges) == 0 {
		return nil
	}
	return m
}

// unhex decodes a PDF hex string body, ignoring whitespace. An odd final
// digit is padded with zero, as the PDF specification requires.
func unhex(h []byte) string {
	out := make([]byte, 0, len(h)/2)
	var cur byte
	n := 0
	for _, c := range h {
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			continue
		}
		cur = cur<<4 | v
		if n++; n%2 == 0 {
			out = append(out, cur)
			cur = 0
		}
	}
	if n%2 == 1 {
		out = append(out, cur<<4)
	}
	return string(out)
}

// lookup returns the text for one character code.
func (m *toUnicodeMap) lookup(code string) (string, bool) {
	if s, ok := m.chars[code]; ok {
		return s, true
	}
	for _, r := range m.ranges {
		if len(r.lo) != len(code) || code < r.lo || code > r.hi {
			continue
		}
		last := len(code) - 1
		off := int(code[last]) - int(r.lo[last])
		if len(code) > 1 {
			off += (int(code[last-1]) - int(r.lo[last-1])) << 8
		}
		if off < 0 {
			continue
		}
		if r.arr != nil {
			if off < len(r.arr) {
				return r.arr[off], true
			}
			continue
		}
		if len(r.dst) < 2 {
			continue
		}
		// Increment the last UTF-16 unit of the destination by the offset.
		b := []byte(r.dst)
		n := int(b[len(b)-2])<<8 | int(b[len(b)-1])
		n += off
		b[len(b)-2], b[len(b)-1] = byte(n>>8), byte(n)
		return utf16BE(string(b)), true
	}
	return "", false
}

// textFont is everything the extractor needs to know about one font.
type textFont struct {
	twoByte  bool
	toUni    *toUnicodeMap
	fallback pdf.TextEncoding // the library's decoder, for simple fonts
	names    map[byte]string  // Differences glyph names (Encoding dictionary)
	winAnsi  bool             // undeclared high bytes are Windows-1252
	widths   [256]float64     // simple font, 1/1000 em
	cidW     map[int]float64  // composite font
	defaultW float64
	cache    map[string]string
}

func newTextFont(f pdf.Font) *textFont {
	tf := &textFont{cache: map[string]string{}, defaultW: 500}
	v := f.V
	tf.twoByte = v.Key("Subtype").Name() == "Type0"
	if tu := v.Key("ToUnicode"); tu.Kind() == pdf.Stream {
		tf.toUni = parseToUnicode(tu)
	}
	if tf.twoByte {
		tf.defaultW = 1000
		desc := v.Key("DescendantFonts").Index(0)
		if dw := desc.Key("DW"); dw.Kind() == pdf.Integer || dw.Kind() == pdf.Real {
			tf.defaultW = dw.Float64()
		}
		tf.cidW = map[int]float64{}
		w := desc.Key("W")
		for i := 0; i < w.Len(); {
			first := int(w.Index(i).Int64())
			next := w.Index(i + 1)
			if next.Kind() == pdf.Array {
				for k := 0; k < next.Len(); k++ {
					tf.cidW[first+k] = next.Index(k).Float64()
				}
				i += 2
				continue
			}
			last, width := int(next.Int64()), w.Index(i+2).Float64()
			if last-first > 1<<16 {
				last = first + 1<<16
			}
			for c := first; c <= last; c++ {
				tf.cidW[c] = width
			}
			i += 3
		}
		return tf
	}

	tf.fallback = f.Encoder()
	if mw := v.Key("FontDescriptor").Key("MissingWidth"); mw.Kind() == pdf.Integer || mw.Kind() == pdf.Real {
		tf.defaultW = mw.Float64()
	}
	for i := range tf.widths {
		tf.widths[i] = tf.defaultW
	}
	first := int(v.Key("FirstChar").Int64())
	ws := v.Key("Widths")
	for i := 0; i < ws.Len() && first+i < 256; i++ {
		if first+i >= 0 {
			tf.widths[first+i] = ws.Index(i).Float64()
		}
	}

	enc := v.Key("Encoding")
	switch enc.Kind() {
	case pdf.Name:
		tf.winAnsi = enc.Name() == "WinAnsiEncoding"
	case pdf.Dict:
		base := enc.Key("BaseEncoding").Name()
		tf.winAnsi = base == "" || base == "WinAnsiEncoding"
		tf.names = map[byte]string{}
		diff := enc.Key("Differences")
		code := -1
		for i := 0; i < diff.Len(); i++ {
			switch x := diff.Index(i); x.Kind() {
			case pdf.Integer:
				code = int(x.Int64())
			case pdf.Name:
				if code >= 0 && code < 256 {
					tf.names[byte(code)] = x.Name()
				}
				code++
			}
		}
	}
	return tf
}

// codeLen returns the byte length of the character code at the start of raw.
func (tf *textFont) codeLen(raw string) int {
	if tf.twoByte && len(raw) >= 2 {
		return 2
	}
	return 1
}

// text returns the cleaned Unicode text for one character code.
func (tf *textFont) text(code string) string {
	if s, ok := tf.cache[code]; ok {
		return s
	}
	s := tf.decode(code)
	tf.cache[code] = s
	return s
}

func (tf *textFont) decode(code string) string {
	if tf.toUni != nil {
		if s, ok := tf.toUni.lookup(code); ok && s != "" && s != "\x00" {
			return cleanGlyphText(s)
		}
	}
	if tf.twoByte || len(code) != 1 {
		return replacement
	}
	c := code[0]
	if name, ok := tf.names[c]; ok {
		// The library answers from the Adobe Glyph List and returns the code
		// unchanged for a name it does not know. A printable ASCII code that
		// comes back unchanged is ambiguous (/parenleft at 0x28 is known and
		// maps to itself), so our resolver for ligature and uniXXXX names is
		// asked before the identity is accepted.
		if s := tf.fallback.Decode(code); s != code {
			return cleanGlyphText(s)
		}
		if s := glyphNameText(name); s != "" {
			return cleanGlyphText(s)
		}
		if c >= 0x20 && c < 0x7F {
			return code
		}
		return replacement
	}
	if tf.names != nil {
		// Encoding dictionary, code not in Differences: the base encoding
		// applies. The library passes the byte through untouched.
		if c >= 0x80 && c < 0xA0 && tf.winAnsi {
			return cleanGlyphText(string(cp1252High[c-0x80]))
		}
		if c >= 0xA0 || (c >= 0x20 && c < 0x7F) {
			return cleanGlyphText(string(rune(c)))
		}
		return replacement
	}
	if tf.fallback != nil {
		return cleanGlyphText(tf.fallback.Decode(code))
	}
	return cleanGlyphText(code)
}

func (tf *textFont) width(code string) float64 {
	if tf.twoByte {
		cid := 0
		for i := 0; i < len(code); i++ {
			cid = cid<<8 | int(code[i])
		}
		if w, ok := tf.cidW[cid]; ok {
			return w
		}
		return tf.defaultW
	}
	return tf.widths[code[0]]
}

// mat is a PDF transformation matrix [a b c d e f].
type mat [6]float64

var identity = mat{1, 0, 0, 1, 0, 0}

// mul returns m × n (apply m, then n).
func (m mat) mul(n mat) mat {
	return mat{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

// Layout thresholds, as fractions of the font size.
const (
	spaceGap     = 0.17 // horizontal gap that separates words
	sameLineDrop = 0.75 // baseline shift still on the same line (scripts)
	scriptShift  = 0.12 // baseline shift that makes a glyph a script
	paragraphGap = 1.9  // baseline distance that separates paragraphs
)

// lineWriter assembles positioned glyphs into lines of text.
type lineWriter struct {
	b        strings.Builder
	started  bool
	dirX     float64 // unit baseline direction of the current line
	dirY     float64
	endU     float64 // pen position along the baseline after the last glyph
	baseV    float64 // baseline of the line's body text, across the baseline
	size     float64 // font size of the line's body text
	lastRune rune
	inScript bool
}

func (w *lineWriter) write(s string) {
	if s == "" {
		return
	}
	w.b.WriteString(s)
	w.lastRune, _ = utf8.DecodeLastRuneInString(s)
}

func (w *lineWriter) newline(blank bool) {
	if w.b.Len() == 0 {
		return
	}
	if w.lastRune != '\n' {
		w.b.WriteByte('\n')
	}
	if blank && !strings.HasSuffix(w.b.String(), "\n\n") {
		w.b.WriteByte('\n')
	}
	w.lastRune = '\n'
	w.inScript = false
}

// glyph places one decoded glyph. (x, y) is its origin in device space,
// (dx, dy) the unit direction of its baseline, size its font size in device
// units and adv its advance along the baseline.
func (w *lineWriter) glyph(text string, x, y, dx, dy, size, adv float64) {
	if size <= 0 {
		size = 1
	}
	u := x*dx + y*dy
	v := -x*dy + y*dx
	defer func() { w.endU = u + adv }()

	if !w.started || math.Abs(dx-w.dirX) > 0.01 || math.Abs(dy-w.dirY) > 0.01 {
		if w.started {
			w.newline(false)
		}
		w.started, w.dirX, w.dirY, w.baseV, w.size = true, dx, dy, v, size
		w.write(text)
		return
	}

	ref := math.Max(w.size, size)
	shift := v - w.baseV
	gap := u - w.endU
	if math.Abs(shift) > sameLineDrop*ref || gap < -2*ref {
		// A new line: the pen moved to another baseline, or jumped far
		// back on this one (next table row, next column).
		w.newline(math.Abs(shift) > paragraphGap*ref && gap < 0)
		w.baseV, w.size = v, size
		w.write(text)
		return
	}

	isScript := math.Abs(shift) > scriptShift*w.size && size < 0.95*w.size
	switch {
	case isScript && !w.inScript:
		// Entering a superscript or subscript. Next to a digit it must not
		// fuse: 1.58×10 followed by a raised 7 is 10^7, never 107 (#34).
		if isDigit(w.lastRune) && startsNumeric(text) {
			if shift > 0 {
				w.write("^")
			} else {
				w.write("_")
			}
		} else if gap > spaceGap*ref && w.lastRune != ' ' {
			w.write(" ")
		}
		w.inScript = true
	case isScript:
		if gap > spaceGap*ref && w.lastRune != ' ' {
			w.write(" ")
		}
	default:
		if math.Abs(shift) > scriptShift*size && size > 1.05*w.size {
			// The line opened with a script (a footnote mark); this larger
			// glyph is the body text and sets the real baseline.
			w.baseV, w.size = v, size
		} else if !w.inScript && math.Abs(shift) <= scriptShift*w.size {
			w.baseV, w.size = v, size
		}
		if gap > spaceGap*ref && w.lastRune != ' ' && !strings.HasPrefix(text, " ") {
			w.write(" ")
		} else if w.inScript && isDigit(w.lastRune) && startsNumeric(text) && w.lastRune != ' ' {
			// Leaving a numeric script straight into more digits.
			w.write(" ")
		}
		w.inScript = false
	}
	if text == " " && (w.lastRune == ' ' || w.lastRune == '\n') {
		return
	}
	w.write(text)
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func startsNumeric(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return isDigit(r) || r == '-' || r == '+' || r == '−'
}

// maxFormDepth bounds recursion into nested form XObjects.
const maxFormDepth = 4

// pageText extracts the text of one page in content-stream order.
func pageText(p pdf.Page) (text string) {
	w := &lineWriter{}
	defer func() {
		// A malformed stream keeps the text read before the error.
		_ = recover()
		text = tidyPageText(w.b.String())
	}()
	if p.V.IsNull() {
		return ""
	}
	x := &textExtractor{w: w, fonts: map[string]*textFont{}}
	x.run(p.V.Key("Contents"), p.Resources(), identity, 0)
	return ""
}

type textExtractor struct {
	w     *lineWriter
	fonts map[string]*textFont // keyed by resource scope + name
	scope int
}

type textState struct {
	ctm        mat
	tm, tlm    mat
	font       *textFont
	size       float64
	tc, tw, th float64
	tl, rise   float64
}

func (x *textExtractor) run(strm, resources pdf.Value, ctm mat, depth int) {
	if strm.Kind() == pdf.Null {
		return
	}
	x.scope++
	scope := x.scope
	g := textState{ctm: ctm, tm: identity, tlm: identity, th: 1}
	var stack []textState

	show := func(raw string) {
		if g.font == nil {
			return
		}
		for len(raw) > 0 {
			n := g.font.codeLen(raw)
			code := raw[:n]
			raw = raw[n:]
			trm := mat{g.size * g.th, 0, 0, g.size, 0, g.rise}.mul(g.tm).mul(g.ctm)
			scale := math.Hypot(trm[0], trm[1])
			w0 := g.font.width(code) / 1000
			adv := w0*g.size + g.tc
			if n == 1 && code[0] == ' ' {
				adv += g.tw
			}
			adv *= g.th
			if scale > 0 {
				// Rise is part of the glyph's position but not of the
				// line's baseline direction.
				devAdv := w0 * scale
				x.w.glyph(g.font.text(code), trm[4], trm[5], trm[0]/scale, trm[1]/scale,
					math.Hypot(trm[2], trm[3]), devAdv)
			}
			g.tm = mat{1, 0, 0, 1, adv, 0}.mul(g.tm)
		}
	}
	nextLine := func() {
		g.tlm = mat{1, 0, 0, 1, 0, -g.tl}.mul(g.tlm)
		g.tm = g.tlm
	}

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}
		num := func(i int) float64 {
			if i < len(args) {
				return args[i].Float64()
			}
			return 0
		}
		switch op {
		case "q":
			stack = append(stack, g)
		case "Q":
			if k := len(stack); k > 0 {
				g = stack[k-1]
				stack = stack[:k-1]
			}
		case "cm":
			if len(args) == 6 {
				g.ctm = mat{num(0), num(1), num(2), num(3), num(4), num(5)}.mul(g.ctm)
			}
		case "BT":
			g.tm, g.tlm = identity, identity
		case "Tf":
			if len(args) == 2 {
				name := args[0].Name()
				key := strconv.Itoa(scope) + "/" + name
				tf, ok := x.fonts[key]
				if !ok {
					if fv := resources.Key("Font").Key(name); fv.Kind() == pdf.Dict {
						tf = newTextFont(pdf.Font{V: fv})
					}
					x.fonts[key] = tf
				}
				g.font, g.size = tf, num(1)
			}
		case "Tc":
			g.tc = num(0)
		case "Tw":
			g.tw = num(0)
		case "Tz":
			g.th = num(0) / 100
		case "TL":
			g.tl = num(0)
		case "Ts":
			g.rise = num(0)
		case "Td", "TD":
			if len(args) == 2 {
				if op == "TD" {
					g.tl = -num(1)
				}
				g.tlm = mat{1, 0, 0, 1, num(0), num(1)}.mul(g.tlm)
				g.tm = g.tlm
			}
		case "Tm":
			if len(args) == 6 {
				g.tm = mat{num(0), num(1), num(2), num(3), num(4), num(5)}
				g.tlm = g.tm
			}
		case "T*":
			nextLine()
		case "Tj":
			if len(args) == 1 {
				show(args[0].RawString())
			}
		case "'":
			if len(args) == 1 {
				nextLine()
				show(args[0].RawString())
			}
		case "\"":
			if len(args) == 3 {
				g.tw, g.tc = num(0), num(1)
				nextLine()
				show(args[2].RawString())
			}
		case "TJ":
			if len(args) == 1 {
				v := args[0]
				for i := 0; i < v.Len(); i++ {
					if e := v.Index(i); e.Kind() == pdf.String {
						show(e.RawString())
					} else {
						g.tm = mat{1, 0, 0, 1, -e.Float64() / 1000 * g.size * g.th, 0}.mul(g.tm)
					}
				}
			}
		case "Do":
			if len(args) != 1 || depth >= maxFormDepth {
				return
			}
			form := resources.Key("XObject").Key(args[0].Name())
			if form.Kind() != pdf.Stream || form.Key("Subtype").Name() != "Form" {
				return
			}
			fm := identity
			if m := form.Key("Matrix"); m.Len() == 6 {
				for i := range fm {
					fm[i] = m.Index(i).Float64()
				}
			}
			res := form.Key("Resources")
			if res.Kind() != pdf.Dict {
				res = resources
			}
			x.run(form, res, fm.mul(g.ctm), depth+1)
		}
	})
}

// tidyPageText trims trailing spaces from every line and collapses runs of
// blank lines, leaving the page's lines otherwise as laid out.
func tidyPageText(s string) string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " ")
		if l == "" {
			blank++
			if blank > 1 || len(out) == 0 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
