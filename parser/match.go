package parser

import (
	"strings"
	"unicode/utf8"
)

// ContainsText reports whether query occurs in text the way a reader would
// say it does: case-insensitively, with any run of whitespace (including a
// line wrap) matching a single space, and seeing through end-of-line
// hyphenation ("under-\nmined" contains "undermined"). query must already be
// lowercase.
//
// Wrapped prose is the norm in PDFs, plain text and hard-wrapped markdown,
// and a phrase search that fails because the phrase straddles a line break is
// a wrong answer, not a miss.
func ContainsText(text, query string) bool {
	if query == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, query) {
		return true
	}
	if !strings.ContainsAny(lower, "\n\t") && !strings.Contains(lower, "  ") {
		return false
	}
	q := strings.Join(strings.Fields(query), " ")
	if q == "" {
		return false
	}
	if strings.Contains(strings.Join(strings.Fields(lower), " "), q) {
		return true
	}
	if !strings.Contains(lower, "-\n") {
		return false
	}
	// Join words split at a line end, only for this comparison: the text
	// itself keeps its hyphen because "key-\nvalue" may be a real compound,
	// so both readings are tried.
	for _, keepHyphen := range []bool{false, true} {
		joined := strings.Join(strings.Fields(dehyphenate(lower, keepHyphen)), " ")
		if strings.Contains(joined, q) {
			return true
		}
	}
	return false
}

// dehyphenate joins a word split by a hyphen at a line end, dropping the
// line break and any indentation after it, and the hyphen too unless
// keepHyphen.
func dehyphenate(s string, keepHyphen bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '-' && i > 0 && isWordByte(s[i-1]) {
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\r') {
				j++
			}
			if j < len(s) && s[j] == '\n' {
				j++
				for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
					j++
				}
				if j < len(s) && isWordByte(s[j]) {
					if keepHyphen {
						b.WriteByte('-')
					}
					i = j - 1
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// LineHit is one source line that matches a search query.
type LineHit struct {
	Line int    `json:"line"`
	Text string `json:"text"` // the line, trimmed to a snippet around the match
}

const (
	// MaxHitsPerSection is how many matching lines SectionLineHits returns.
	MaxHitsPerSection = 3
	snippetWidth      = 110
)

// SectionLineHits is the line-level matcher behind search: the first few
// lines of s's own text (not its subsections') that contain query, which
// must be lowercase, and how many such lines there are in all. A phrase
// that wraps onto the next line, or a word hyphenated across the break, is
// reported at the line where it starts. Documents without line-addressable
// source return nothing.
func SectionLineHits(doc *Document, s *Section, query string) ([]LineHit, int) {
	if doc.Source == "" || s.LineStart <= 0 || query == "" {
		return nil, 0
	}
	// Own lines stop where the first subsection starts. YAML and PDF parents
	// that only span their children have none.
	end := s.LineEnd
	for _, c := range s.Children {
		if c.LineStart > 0 && c.LineStart-1 < end {
			end = c.LineStart - 1
		}
	}
	if end < s.LineStart {
		return nil, 0
	}
	lines := strings.Split(doc.SourceSpan(s.LineStart, end), "\n")
	var hits []LineHit
	count := 0
	wrapped := false // the previous hit already covered this line's half of a wrapped match
	for i, line := range lines {
		at := strings.Index(strings.ToLower(line), query)
		if at < 0 && !wrapped && i+1 < len(lines) && straddles(line, lines[i+1], query) {
			// Starts on this line, finishes on the next.
			at = len(line)
			wrapped = true
		} else {
			wrapped = false
		}
		if at < 0 {
			continue
		}
		count++
		if len(hits) < MaxHitsPerSection {
			hits = append(hits, LineHit{Line: s.LineStart + i, Text: snippet(line, at)})
		}
	}
	return hits, count
}

// straddles reports whether query matches across the break between line
// and next without matching inside either one alone. Only the end of line
// and the start of next can take part, so only those are compared.
func straddles(line, next, query string) bool {
	tail := line
	if cut := len(tail) - len(query) - 2; cut > 0 {
		for cut < len(tail) && !utf8.RuneStart(tail[cut]) {
			cut++
		}
		tail = tail[cut:]
	}
	head := strings.TrimLeft(next, " \t")
	if cut := len(query); cut < len(head) {
		for cut < len(head) && !utf8.RuneStart(head[cut]) {
			cut++
		}
		head = head[:cut]
	}
	return ContainsText(tail+"\n"+head, query) && !ContainsText(head, query)
}

// snippet trims line to about snippetWidth characters around the match at
// byte offset at, marking cut ends with an ellipsis.
func snippet(line string, at int) string {
	line = strings.TrimRight(line, " \t\r")
	lead := len(line) - len(strings.TrimLeft(line, " \t"))
	line = line[lead:]
	at -= lead
	if at < 0 {
		at = 0
	}
	if at > len(line) {
		at = len(line)
	}
	r := []rune(line)
	if len(r) <= snippetWidth {
		return line
	}
	pos := len([]rune(line[:at]))
	start := pos - snippetWidth/3
	if start < 0 {
		start = 0
	}
	stop := start + snippetWidth
	if stop > len(r) {
		stop = len(r)
		start = stop - snippetWidth
	}
	out := string(r[start:stop])
	if start > 0 {
		out = "…" + out
	}
	if stop < len(r) {
		out += "…"
	}
	return out
}
