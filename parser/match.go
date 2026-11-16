package parser

import "strings"

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
