package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tocPDF has a table of contents on page 1 that repeats every heading, and a
// figure label on page 2 that repeats one of them above the real heading.
// Text matching alone anchors both sections on the contents page. One
// outline entry uses a direct destination, the other a named destination
// resolved through the /Names tree, as LaTeX hyperref writes them.
func tocPDF(withDests bool) []byte {
	page1 := strings.Join([]string{
		`BT /F1 10 Tf 72 720 Td (Contents) Tj ET`,
		`BT /F1 10 Tf 72 700 Td (Introduction . . . . . 2) Tj ET`,
		`BT /F1 10 Tf 72 686 Td (Calibration Methods . . . . . 2) Tj ET`,
	}, "\n")
	page2 := strings.Join([]string{
		`BT /F1 8 Tf 72 750 Td (Calibration Methods) Tj ET`, // figure label
		`BT /F1 12 Tf 72 700 Td (1 Introduction) Tj ET`,
		`BT /F1 10 Tf 72 686 Td (Opening words of the paper.) Tj ET`,
		`BT /F1 12 Tf 72 600 Td (1.2 Calibration Methods) Tj ET`,
		`BT /F1 10 Tf 72 586 Td (Standards were diluted serially.) Tj ET`,
	}, "\n")
	intro, methods := ``, ``
	names := ``
	if withDests {
		intro = ` /Dest [5 0 R /XYZ 72 714 null]`
		methods = ` /A << /S /GoTo /D (sec.methods) >>`
		names = ` /Names << /Dests 11 0 R >>`
	}
	res := `/Resources << /Font << /F1 7 0 R >> >>`
	return buildPDF([]string{
		`<< /Type /Catalog /Pages 2 0 R /Outlines 8 0 R` + names + ` >>`,                      // 1
		`<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>`,                                     // 2
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R ` + res + ` >>`, // 3
		stream(page1), // 4
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 6 0 R ` + res + ` >>`, // 5
		stream(page2), // 6
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>`, // 7
		`<< /Type /Outlines /First 9 0 R /Last 10 0 R /Count 2 >>`,                          // 8
		`<< /Title (Introduction) /Parent 8 0 R /Next 10 0 R` + intro + ` >>`,               // 9
		`<< /Title (Calibration Methods) /Parent 8 0 R /Prev 9 0 R` + methods + ` >>`,       // 10
		`<< /Kids [12 0 R] >>`, // 11
		`<< /Limits [(sec.methods) (sec.methods)] /Names [(sec.methods) << /D [5 0 R /XYZ 72 614 0] >>] >>`, // 12
	})
}

func parseBytes(t *testing.T, raw []byte) *Document {
	t.Helper()
	t.Setenv(PDFTextCommandEnv, "")
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := ParsePDF(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestParsePDF_AnchorsByOutlineDestination(t *testing.T) {
	doc := parseBytes(t, tocPDF(true))
	intro, methods := doc.GetSection("Introduction"), doc.GetSection("Calibration Methods")
	if intro == nil || methods == nil {
		t.Fatalf("sections = %v", titles(doc))
	}
	if intro.Content != "Opening words of the paper." {
		t.Errorf("Introduction content = %q", intro.Content)
	}
	if methods.Content != "Standards were diluted serially." {
		t.Errorf("Calibration Methods content = %q", methods.Content)
	}
	// The heading keeps its section number, and its line is the real heading.
	if got := doc.SourceSpan(methods.LineStart, methods.LineStart); got != "1.2 Calibration Methods" {
		t.Errorf("methods heading line = %q", got)
	}
	if got := doc.SourceSpan(intro.LineStart, intro.LineStart); got != "1 Introduction" {
		t.Errorf("intro heading line = %q", got)
	}
}

func TestParsePDF_WithoutDestinationsStillAnchors(t *testing.T) {
	// Same document, outline without destinations: the text-only passes
	// run, and every title is still placed somewhere.
	doc := parseBytes(t, tocPDF(false))
	for _, s := range doc.GetAllSections() {
		if s.LineStart == 0 {
			t.Errorf("%q was not anchored", s.Title)
		}
	}
}

func TestReadOutlineDestinations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(path, tocPDF(true), 0o644); err != nil {
		t.Fatal(err)
	}
	f, r, err := openPDF(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sections, dests := readOutline(r)
	if len(sections) != 2 || len(dests) != 2 {
		t.Fatalf("sections=%d dests=%d", len(sections), len(dests))
	}
	want := []outlineDest{{page: 1, top: 714, hasTop: true}, {page: 1, top: 614, hasTop: true}}
	for i, d := range dests {
		if d != want[i] {
			t.Errorf("dest[%d] = %+v, want %+v", i, d, want[i])
		}
	}
}

func TestParseFile_CachesPDF(t *testing.T) {
	dir := t.TempDir()
	cacheDirForTest = filepath.Join(dir, "cache")
	t.Cleanup(func() { cacheDirForTest = "" })
	t.Setenv("DOCMAP_NO_CACHE", "")
	t.Setenv(PDFTextCommandEnv, "")
	path := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(path, tocPDF(true), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(cacheDirForTest); len(entries) != 1 {
		t.Fatalf("expected one cache entry, got %d", len(entries))
	}
	second, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.Source != first.Source || second.TotalTokens != first.TotalTokens {
		t.Fatal("cached PDF differs from the fresh parse")
	}
	m := second.GetSection("Calibration Methods")
	if m == nil || m.Content != "Standards were diluted serially." || m.LineStart == 0 {
		t.Fatalf("cached section = %+v", m)
	}

	// An external text command must bypass the cache in both directions.
	t.Setenv(PDFTextCommandEnv, filepath.Join(dir, "missing-tool"))
	if _, err := ParseFile(path); err != nil {
		t.Fatal(err)
	}
}
