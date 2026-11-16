package parser

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildPDF assembles a PDF from object bodies; objs[i] is object i+1 and
// object 1 must be the catalog.
func buildPDF(objs []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func stream(s string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
}

const testToUnicode = `/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CMapName /Adobe-Identity-UCS def
1 begincodespacerange
<00> <FF>
endcodespacerange
2 beginbfchar
<01> <00660069>
<02> <FB02>
endbfchar
1 beginbfrange
<41> <5A> <0041>
endbfrange
endcmap
end
end`

const testToUnicode2 = `begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 beginbfrange
<0001> <0002> [<0048> <00690021>]
endbfrange
endcmap`

// fidelityPDF is a two-page PDF that reproduces, with fonts built for the
// purpose, every defect in #34 plus the missing word spacing.
func fidelityPDF() []byte {
	page1 := strings.Join([]string{
		// Words spaced by pen movement only; a kern must not split a word.
		`BT /F1 10 Tf 72 700 Td [(Introduction)] TJ ET`,
		`BT /F1 10 Tf 72 686 Td [(calibration) -333 (range) -333 (of) -333 (0.625\226125) -333 (nM) -333 (A) 80 (V)] TJ ET`,
		// Differences names the library does not know, and an unknown glyph.
		`BT /F1 10 Tf 72 672 Td (puri\001cation \003e \002rst x\005y) Tj ET`,
		// Raised exponents, then a raised citation mark after a word.
		`BT /F1 10 Tf 72 658 Td (1.58\32710) Tj /F1 7 Tf 4 Ts (7) Tj 0 Ts /F1 10 Tf ( \261 4.06\32710) Tj /F1 7 Tf 4 Ts (6) Tj 0 Ts /F1 10 Tf ( cells) Tj /F1 7 Tf 4 Ts (1,2) Tj ET`,
	}, "\n")
	page2 := strings.Join([]string{
		`BT /F2 10 Tf 72 700 Td (CALIBRATION METHODS) Tj ET`,
		// ToUnicode must win over the Encoding dictionary on the same font.
		`BT /F2 10 Tf 72 686 Td (quanti\001cation \002ow) Tj ET`,
		// Composite font, two-byte codes, bfrange with an array destination.
		`BT /F3 10 Tf 72 672 Td <00010002> Tj ET`,
		// A table row: the pen jumps far to the right on the same baseline.
		`BT /F2 10 Tf 72 658 Td (LEFT) Tj 200 0 Td (RIGHT) Tj ET`,
	}, "\n")
	fonts := `/Font << /F1 7 0 R /F2 8 0 R /F3 10 0 R >>`
	return buildPDF([]string{
		`<< /Type /Catalog /Pages 2 0 R /Outlines 13 0 R >>`,                                                     // 1
		`<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>`,                                                        // 2
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << ` + fonts + ` >> >>`, // 3
		stream(page1), // 4
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 6 0 R /Resources << ` + fonts + ` >> >>`, // 5
		stream(page2), // 6
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [1 /fi /f_i /T_h 5 /g123] >> >>`, // 7
		`<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman /Encoding << /Type /Encoding /Differences [1 /g1 /g2] >> /ToUnicode 9 0 R >>`,                           // 8
		stream(testToUnicode), // 9
		`<< /Type /Font /Subtype /Type0 /BaseFont /Fake /Encoding /Identity-H /DescendantFonts [11 0 R] /ToUnicode 12 0 R >>`, // 10
		`<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Fake /DW 600 /W [1 [500 500]] >>`,                                   // 11
		stream(testToUnicode2), // 12
		`<< /Type /Outlines /First 14 0 R /Last 15 0 R /Count 2 >>`,                         // 13
		`<< /Title (Introduction) /Parent 13 0 R /Next 15 0 R /Dest [3 0 R /Fit] >>`,        // 14
		`<< /Title (Calibration Methods) /Parent 13 0 R /Prev 14 0 R /Dest [5 0 R /Fit] >>`, // 15
	})
}

func writeFidelityPDF(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fidelity.pdf")
	if err := os.WriteFile(path, fidelityPDF(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParsePDF_TextFidelity(t *testing.T) {
	t.Setenv(PDFTextCommandEnv, "")
	doc, err := ParsePDF(writeFidelityPDF(t))
	if err != nil {
		t.Fatal(err)
	}
	src := doc.Source
	for _, want := range []string{
		"calibration range of 0.625–125 nM AV", // spaces from gaps; 0x96 is an en dash, not a fused number
		"purification The first",               // /fi, /T_h, /f_i resolved
		"x�y",                                  // unknown glyph stays visible
		"1.58×10^7 ± 4.06×10^6 cells1,2",       // exponents kept apart from mantissas
		"quantification flow",                  // ToUnicode beats Differences; U+FB02 expanded
		"Hi!",                                  // composite font
		"LEFT RIGHT",                           // same-baseline jump is a space
	} {
		if !strings.Contains(src, want) {
			t.Errorf("source missing %q\n--- source ---\n%s", want, src)
		}
	}
	for _, bad := range []string{"0.625125", "107", "106", "\x01", "\u0096", "ﬁ", "ﬂ"} {
		if strings.Contains(src, bad) {
			t.Errorf("source must not contain %q\n--- source ---\n%s", bad, src)
		}
	}

	intro := doc.GetSection("Introduction")
	methods := doc.GetSection("Calibration Methods")
	if intro == nil || methods == nil {
		t.Fatalf("outline sections missing: %v", titles(doc))
	}
	if !strings.Contains(intro.Content, "purification") || strings.Contains(intro.Content, "quantification") {
		t.Errorf("Introduction content = %q", intro.Content)
	}
	if !strings.HasPrefix(methods.Content, "quantification flow") {
		t.Errorf("Calibration Methods content = %q", methods.Content)
	}
	if got := doc.SourceSpan(methods.LineStart, methods.LineStart); got != "CALIBRATION METHODS" {
		t.Errorf("methods heading line = %q", got)
	}
}

func TestParsePDF_PageModeHasRealLines(t *testing.T) {
	t.Setenv(PDFTextCommandEnv, "")
	// Same PDF without its outline.
	raw := bytes.Replace(fidelityPDF(), []byte(" /Outlines 13 0 R"), []byte("                 "), 1)
	path := filepath.Join(t.TempDir(), "pages.pdf")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := ParsePDF(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(titles(doc), "|"); got != "Page 1|Page 2" {
		t.Fatalf("titles = %q", got)
	}
	p2 := doc.Sections[1]
	if p2.LineStart <= doc.Sections[0].LineEnd {
		t.Fatalf("page 2 lines %d-%d overlap page 1 (ends %d)", p2.LineStart, p2.LineEnd, doc.Sections[0].LineEnd)
	}
	if got := doc.SourceSpan(p2.LineStart, p2.LineStart); got != "CALIBRATION METHODS" {
		t.Fatalf("first line of page 2 = %q", got)
	}
	if got := doc.SourceSpan(p2.LineStart, p2.LineEnd); strings.TrimSpace(got) != p2.Content {
		t.Fatalf("page 2 span %q != content %q", got, p2.Content)
	}
}

func TestParsePDF_ExternalTextCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "extract.sh")
	body := "#!/bin/sh\n[ -f \"$1\" ] || exit 3\nprintf 'Introduction\\nexternal words here\\n\\fCalibration Methods\\nquantification from outside\\n\\f'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PDFTextCommandEnv, script+" {}")
	doc, err := ParsePDF(writeFidelityPDF(t))
	if err != nil {
		t.Fatal(err)
	}
	// docmap's outline structure, the external tool's text.
	methods := doc.GetSection("Calibration Methods")
	if methods == nil || methods.Content != "quantification from outside" {
		t.Fatalf("methods = %+v\nsource:\n%s", methods, doc.Source)
	}
	if strings.Contains(doc.Source, "purification") {
		t.Fatal("built-in text leaked into external-backend output")
	}

	// A failing command falls back to the built-in extractor.
	t.Setenv(PDFTextCommandEnv, filepath.Join(dir, "missing-tool"))
	doc, err = ParsePDF(writeFidelityPDF(t))
	if err != nil || !strings.Contains(doc.Source, "purification") {
		t.Fatalf("fallback failed: err=%v source=%q", err, doc.Source)
	}
}

func TestCleanGlyphText(t *testing.T) {
	cases := map[string]string{
		"plain":         "plain",
		"eﬃcient":       "efficient",
		"\u0096":        "–",
		"\u0093q\u0094": "“q”",
		"a\x1fb":        "a�b",
		"soft\u00ADhy":  "softhy",
	}
	for in, want := range cases {
		if got := cleanGlyphText(in); got != want {
			t.Errorf("cleanGlyphText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGlyphNameText(t *testing.T) {
	cases := map[string]string{
		"f_i": "fi", "T_h": "Th", "f_f_i": "ffi", "f_i.alt": "fi",
		"uni2009": " ", "uni00660069": "fi", "u1F600": "\U0001F600",
		"g123": "", "": "", "Omegainv": "",
	}
	for in, want := range cases {
		if got := glyphNameText(in); got != want {
			t.Errorf("glyphNameText(%q) = %q, want %q", in, got, want)
		}
	}
}
