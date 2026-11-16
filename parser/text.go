package parser

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// atxHeading spots a .txt file that is really markdown.
var atxHeading = regexp.MustCompile(`(?m)^#{1,6} \S`)

// numberedHeading matches "1 Introduction", "2.3. Methods", "A.1 Proofs".
var numberedHeading = regexp.MustCompile(`^((?:\d+|[A-Z])(?:\.\d+)*)\.?\s+\p{Lu}`)

// maxHeadingLen bounds a plain-text heading line. Longer lines are prose.
const maxHeadingLen = 72

// ParseText parses a plain-text file. Text with markdown ATX headings is
// parsed as markdown. Anything else gets a structure inferred from the
// conventions extracted or hand-written text actually uses, in this order:
//
//   - underlined titles (=== / --- / ~~~ under a line),
//   - numbered headings ("2.3 Methods") and ALL-CAPS lines, each standing
//     alone between blank lines,
//   - form feeds, which pdftotext and friends emit between pages,
//   - failing all of those, one section spanning the file,
//
// so every byte of the file belongs to a section and is reachable by search,
// --expand and --at. Line numbers are real file lines.
func ParseText(content string) *Document {
	if atxHeading.MatchString(content) {
		return Parse(content)
	}
	doc := &Document{Source: content}
	if strings.TrimSpace(content) == "" {
		return doc
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	type mark struct {
		line  int // 0-based index of the title line
		level int
		title string
		skip  int // extra heading lines (an underline) excluded from content
	}
	var marks []mark
	blank := func(i int) bool {
		return i < 0 || i >= len(lines) || strings.TrimSpace(strings.Trim(lines[i], "\f")) == ""
	}
	for i := 0; i < len(lines); i++ {
		title := strings.TrimSpace(strings.Trim(lines[i], "\f"))
		if title == "" || len(title) > maxHeadingLen {
			continue
		}
		if i+1 < len(lines) {
			if lvl := underlineLevel(lines[i+1], title); lvl > 0 && blank(i-1) {
				marks = append(marks, mark{i, lvl, title, 1})
				i++
				continue
			}
		}
		if !blank(i-1) || !blank(i+1) {
			continue
		}
		if m := numberedHeading.FindStringSubmatch(title); m != nil && !endsLikeSentence(title) {
			marks = append(marks, mark{i, 1 + strings.Count(m[1], "."), title, 0})
		} else if isAllCapsHeading(title) {
			marks = append(marks, mark{i, 1, title, 0})
		}
	}

	section := func(level int, title string, from, to, bodyFrom int) *Section {
		for to > from && blank(to) {
			to--
		}
		body := ""
		if bodyFrom <= to {
			body = strings.TrimSpace(strings.ReplaceAll(strings.Join(lines[bodyFrom:to+1], "\n"), "\f", ""))
		}
		return &Section{
			Level: level, Title: title, Content: body,
			Tokens:    estimateTokens(title) + estimateTokens(body),
			KeyTerms:  extractKeyTerms(body),
			LineStart: from + 1, LineEnd: to + 1,
		}
	}

	var all []*Section
	switch {
	case len(marks) > 0:
		if start := firstNonBlank(lines); start < marks[0].line {
			all = append(all, section(1, "(preamble)", start, marks[0].line-1, start))
		}
		for k, m := range marks {
			end := len(lines) - 1
			if k+1 < len(marks) {
				end = marks[k+1].line - 1
			}
			all = append(all, section(m.level, m.title, m.line, end, m.line+1+m.skip))
		}
	case strings.Contains(content, "\f"):
		page, start := 1, 0
		flush := func(end int) {
			if s := section(1, fmt.Sprintf("Page %d", page), start, end, start); s.Content != "" {
				all = append(all, s)
			}
		}
		for i, l := range lines {
			// A form feed ends the page. pdftotext puts it at the start of
			// the next page's first line.
			for n := strings.Count(l, "\f"); n > 0; n-- {
				if strings.HasPrefix(l, "\f") {
					flush(i - 1)
					start = i
				} else {
					flush(i)
					start = i + 1
				}
				page++
				l = strings.Replace(l, "\f", "", 1)
			}
		}
		flush(len(lines) - 1)
	default:
		start := firstNonBlank(lines)
		all = append(all, section(1, "(full text)", start, len(lines)-1, start))
	}

	doc.Sections = buildTree(all)
	for _, r := range doc.Sections {
		extendSectionRange(r)
	}
	for _, s := range all {
		doc.TotalTokens += s.Tokens
	}
	return doc
}

func firstNonBlank(lines []string) int {
	for i, l := range lines {
		if strings.TrimSpace(strings.Trim(l, "\f")) != "" {
			return i
		}
	}
	return 0
}

// underlineLevel returns 1 for a === underline, 2 for --- or ~~~, when the
// line under title is a run of one such character about as long as the title.
func underlineLevel(under, title string) int {
	under = strings.TrimSpace(under)
	if len(under) < 3 || len(under)+2 < len(title) {
		return 0
	}
	c := under[0]
	if strings.Trim(under, string(c)) != "" {
		return 0
	}
	switch c {
	case '=':
		return 1
	case '-', '~':
		return 2
	}
	return 0
}

func endsLikeSentence(s string) bool {
	return strings.HasSuffix(s, ".") || strings.HasSuffix(s, ",") || strings.HasSuffix(s, ";")
}

// isAllCapsHeading reports whether s reads as a shouted heading: at least
// four letters, none lowercase, mostly letters, and not sentence-terminated.
func isAllCapsHeading(s string) bool {
	if endsLikeSentence(s) {
		return false
	}
	letters, other := 0, 0
	for _, r := range s {
		switch {
		case unicode.IsLower(r):
			return false
		case unicode.IsUpper(r):
			letters++
		case !unicode.IsSpace(r):
			other++
		}
	}
	return letters >= 4 && letters > other*2
}
