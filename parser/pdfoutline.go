package parser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
)

// outlineDest is where an outline entry points: a page and, when the
// destination carries one, the y coordinate of the top of the view in
// default user space (PDF y grows upward).
type outlineDest struct {
	page   int // 0-based; -1 when unknown
	top    float64
	hasTop bool
}

var noDest = outlineDest{page: -1}

// Limits that keep a hostile or cyclic outline from running away.
const (
	maxOutlineItems = 20000
	maxOutlineDepth = 32
	maxNamedDests   = 200000
)

var objRef = regexp.MustCompile(`(\d+) \d+ R`)

// leadingRef returns the object number of the indirect reference that
// starts an array, read from the library's textual form ("[12 0 R /XYZ …]").
// The library resolves references on access and does not expose object
// numbers, but it prints unresolved ones.
func leadingRef(arr pdf.Value) (int, bool) {
	s := arr.String()
	if !strings.HasPrefix(s, "[") {
		return 0, false
	}
	m := objRef.FindStringSubmatchIndex(s)
	if m == nil || m[0] != 1 {
		return 0, false
	}
	n, err := strconv.Atoi(s[m[2]:m[3]])
	return n, err == nil
}

// pageObjects maps each page's object number to its 0-based index.
func pageObjects(r *pdf.Reader) map[int]int {
	out := map[int]int{}
	next := 0
	var walk func(node pdf.Value, depth int)
	walk = func(node pdf.Value, depth int) {
		if depth > maxOutlineDepth {
			return
		}
		kids := node.Key("Kids")
		refs := objRef.FindAllStringSubmatch(kids.String(), -1)
		aligned := len(refs) == kids.Len()
		for i := 0; i < kids.Len(); i++ {
			kid := kids.Index(i)
			if kid.Key("Type").Name() == "Pages" {
				walk(kid, depth+1)
				continue
			}
			if aligned {
				if n, err := strconv.Atoi(refs[i][1]); err == nil {
					out[n] = next
				}
			}
			next++
		}
	}
	walk(r.Trailer().Key("Root").Key("Pages"), 0)
	return out
}

// destResolver turns a destination value into an outlineDest.
type destResolver struct {
	pages map[int]int
	root  pdf.Value
	named map[string]pdf.Value // built on first use
}

func (d *destResolver) loadNamed() {
	d.named = map[string]pdf.Value{}
	// PDF 1.1: a dictionary of names under /Dests.
	old := d.root.Key("Dests")
	for _, k := range old.Keys() {
		d.named[k] = old.Key(k)
	}
	// PDF 1.2+: a name tree under /Names /Dests.
	var walk func(node pdf.Value, depth int)
	walk = func(node pdf.Value, depth int) {
		if depth > maxOutlineDepth || len(d.named) > maxNamedDests {
			return
		}
		names := node.Key("Names")
		for i := 0; i+1 < names.Len(); i += 2 {
			d.named[names.Index(i).RawString()] = names.Index(i + 1)
		}
		kids := node.Key("Kids")
		for i := 0; i < kids.Len(); i++ {
			walk(kids.Index(i), depth+1)
		}
	}
	walk(d.root.Key("Names").Key("Dests"), 0)
}

func (d *destResolver) resolve(v pdf.Value) outlineDest {
	for hops := 0; hops < 4; hops++ {
		switch v.Kind() {
		case pdf.Dict:
			v = v.Key("D")
			continue
		case pdf.String, pdf.Name:
			if d.named == nil {
				d.loadNamed()
			}
			key := v.RawString()
			if v.Kind() == pdf.Name {
				key = v.Name()
			}
			next, ok := d.named[key]
			if !ok {
				return noDest
			}
			v = next
			continue
		case pdf.Array:
			obj, ok := leadingRef(v)
			if !ok {
				return noDest
			}
			page, ok := d.pages[obj]
			if !ok {
				return noDest
			}
			dest := outlineDest{page: page}
			topAt := -1
			switch v.Index(1).Name() {
			case "XYZ":
				topAt = 3
			case "FitH", "FitBH":
				topAt = 2
			case "FitR":
				topAt = 5
			}
			if topAt > 0 {
				if t := v.Index(topAt); t.Kind() == pdf.Integer || t.Kind() == pdf.Real {
					dest.top, dest.hasTop = t.Float64(), true
				}
			}
			return dest
		}
		return noDest
	}
	return noDest
}

// readOutline walks the document outline and returns the section tree plus,
// in the same pre-order as Document.GetAllSections, each entry's destination.
func readOutline(r *pdf.Reader) (sections []*Section, dests []outlineDest) {
	defer func() {
		// A malformed outline keeps what was read.
		_ = recover()
	}()
	root := r.Trailer().Key("Root")
	res := &destResolver{pages: pageObjects(r), root: root}
	count := 0
	var walk func(parent pdf.Value, up *Section, level int) []*Section
	walk = func(parent pdf.Value, up *Section, level int) []*Section {
		var out []*Section
		if level > maxOutlineDepth {
			return nil
		}
		for item := parent.Key("First"); item.Kind() == pdf.Dict; item = item.Key("Next") {
			if count++; count > maxOutlineItems {
				break
			}
			s := &Section{Level: level, Title: strings.TrimSpace(item.Key("Title").Text()), Parent: up}
			dest := item.Key("Dest")
			if dest.IsNull() {
				if a := item.Key("A"); a.Key("S").Name() == "GoTo" {
					dest = a.Key("D")
				}
			}
			dests = append(dests, res.resolve(dest))
			out = append(out, s)
			s.Children = walk(item, s, level+1)
		}
		return out
	}
	sections = walk(root.Key("Outlines"), nil, 1)
	return sections, dests
}
