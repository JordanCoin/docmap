package parser

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// charInfo holds per-character data extracted from PDF
type charInfo struct {
	text     string
	x        float64
	y        float64
	fontSize float64
	isBold   bool
	fontName string
}

// textLine represents a line of text extracted from PDF with font metadata
type textLine struct {
	Text     string
	Y        float64
	FontSize float64
	IsBold   bool
	FontName string
}

// headingInfo represents a detected heading with its position and level
type headingInfo struct {
	LineIdx int
	Level   int
}

// ParseURL fetches a URL via headless Chrome, converts to PDF, and extracts sections.
func ParseURL(url string) (*Document, error) {
	chromePath, err := findChrome()
	if err != nil {
		return nil, fmt.Errorf("chrome not found: %w\n\nInstall Chrome or set CHROME_PATH environment variable", err)
	}

	tmpDir, err := os.MkdirTemp("", "docmap-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	pdfPath := filepath.Join(tmpDir, "page.pdf")
	if err := urlToPDF(chromePath, url, pdfPath); err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	lines, err := extractSpatialText(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	if len(lines) == 0 {
		return &Document{Sections: []*Section{{
			Level: 1,
			Title: "(no extractable text)",
		}}}, nil
	}

	headings := detectHeadings(lines)
	doc := &Document{}
	doc.Sections = buildSectionsFromLines(lines, headings)

	for _, s := range doc.GetAllSections() {
		doc.TotalTokens += s.Tokens
	}

	return doc, nil
}

// findChrome locates the Chrome/Chromium binary on the system.
func findChrome() (string, error) {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("CHROME_PATH set but not found: %s", p)
	}

	switch runtime.GOOS {
	case "darwin":
		paths := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	case "linux":
		names := []string{"google-chrome", "google-chrome-stable", "chromium-browser", "chromium"}
		for _, name := range names {
			if p, err := exec.LookPath(name); err == nil {
				return p, nil
			}
		}
	case "windows":
		paths := []string{
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("Chrome/Chromium not found")
}

// urlToPDF uses headless Chrome to render a URL to PDF.
func urlToPDF(chromePath, url, outPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, chromePath,
		"--headless",
		"--disable-gpu",
		"--no-pdf-header-footer",
		"--print-to-pdf="+outPath,
		url,
	)
	cmd.Stderr = nil
	cmd.Stdout = nil

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out after 30s rendering %s", url)
		}
		return fmt.Errorf("chrome failed: %w", err)
	}

	if _, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("PDF was not generated")
	}

	return nil
}

// extractSpatialText extracts text lines with font metadata from a PDF.
// Uses pdftotext (poppler) for clean text when available, with go-pdfium providing
// font size data for heading detection. Falls back to go-pdfium rects if pdftotext
// is not installed.
func extractSpatialText(pdfPath string) ([]textLine, error) {
	// Try pdftotext + go-pdfium hybrid first
	plainLines, ptErr := extractWithPdftotext(pdfPath)
	if ptErr == nil {
		// Get heading candidates from go-pdfium rect data
		headingCandidates, bodySize, err := extractHeadingCandidates(pdfPath)
		if err == nil && len(headingCandidates) > 0 {
			return buildLinesWithHeadings(plainLines, headingCandidates, bodySize), nil
		}
		// If pdfium fails, return pdftotext lines without heading info
		var result []textLine
		for _, line := range plainLines {
			result = append(result, textLine{Text: line, FontSize: 12})
		}
		return result, nil
	}

	// Fall back to go-pdfium rect-based extraction
	return extractWithPdfium(pdfPath)
}

// headingCandidate is a heading detected from go-pdfium with its normalized text and font size.
type headingCandidate struct {
	normalizedText string
	fontSize       float64
}

// extractHeadingCandidates runs go-pdfium rect extraction and heading detection,
// returning normalized heading text + font sizes and the body font size.
func extractHeadingCandidates(pdfPath string) ([]headingCandidate, float64, error) {
	rectLines, err := extractWithPdfium(pdfPath)
	if err != nil {
		return nil, 0, err
	}

	headings := detectHeadings(rectLines)
	if len(headings) == 0 {
		return nil, 0, nil
	}

	// Determine body size
	sizeCount := make(map[float64]int)
	for _, l := range rectLines {
		rounded := math.Round(l.FontSize*2) / 2
		sizeCount[rounded] += len(l.Text)
	}
	var bodySize float64
	var maxCount int
	for size, count := range sizeCount {
		if count > maxCount {
			maxCount = count
			bodySize = size
		}
	}

	var candidates []headingCandidate
	for _, h := range headings {
		norm := normalizeForMatch(rectLines[h.LineIdx].Text)
		// Skip very short heading candidates — they cause false matches
		if len(norm) < 4 {
			continue
		}
		candidates = append(candidates, headingCandidate{
			normalizedText: norm,
			fontSize:       rectLines[h.LineIdx].FontSize,
		})
	}

	return candidates, bodySize, nil
}

// normalizeForMatch strips spaces/punctuation and lowercases for fuzzy matching.
func normalizeForMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// buildLinesWithHeadings assigns heading font sizes to pdftotext lines that match
// heading candidates extracted from go-pdfium. Uses character-overlap matching
// since go-pdfium text may have stutter prefixes (e.g. "VeVersioning" matches "Versioning").
func buildLinesWithHeadings(plainLines []string, candidates []headingCandidate, bodySize float64) []textLine {
	var result []textLine

	candidateIdx := 0
	for _, line := range plainLines {
		fontSize := bodySize
		normalized := normalizeForMatch(line)

		// Check if this line matches the next heading candidate
		if candidateIdx < len(candidates) && len(normalized) > 0 {
			candidate := candidates[candidateIdx]
			if isHeadingMatch(normalized, candidate.normalizedText) {
				fontSize = candidate.fontSize
				candidateIdx++
			}
		}

		result = append(result, textLine{
			Text:     line,
			FontSize: fontSize,
		})
	}

	return result
}

// isHeadingMatch checks if a clean pdftotext line matches a garbled go-pdfium heading.
// The pdfium text may have stutter prefixes ("veversioning" for "versioning"), so we check
// if the clean text is contained within the garbled text.
func isHeadingMatch(cleanNorm, garbledNorm string) bool {
	if len(cleanNorm) < 4 || len(garbledNorm) < 4 {
		return false
	}
	if cleanNorm == garbledNorm {
		return true
	}
	// Clean text should be a substring of garbled (garbled has extra stutter chars)
	if strings.Contains(garbledNorm, cleanNorm) {
		return true
	}
	// Or garbled is a substring of clean
	if strings.Contains(cleanNorm, garbledNorm) {
		return true
	}
	// Check character overlap ratio — require high overlap
	overlap := charOverlap(cleanNorm, garbledNorm)
	shorter := len(cleanNorm)
	if len(garbledNorm) < shorter {
		shorter = len(garbledNorm)
	}
	return float64(overlap)/float64(shorter) > 0.8
}

// charOverlap counts matching characters between two strings using a simple LCS-like approach.
func charOverlap(a, b string) int {
	count := 0
	bIdx := 0
	for _, ch := range a {
		for bIdx < len(b) {
			if rune(b[bIdx]) == ch {
				count++
				bIdx++
				break
			}
			bIdx++
		}
	}
	return count
}

// extractWithPdftotext shells out to pdftotext for clean text extraction.
func extractWithPdftotext(pdfPath string) ([]string, error) {
	pdftotextPath, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, fmt.Errorf("pdftotext not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pdftotextPath, pdfPath, "-")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pdftotext failed: %w", err)
	}

	var lines []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}


// extractWithPdfium falls back to go-pdfium rect-based extraction when pdftotext is unavailable.
func extractWithPdfium(pdfPath string) ([]textLine, error) {
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle: 1, MaxIdle: 1, MaxTotal: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init pdfium: %w", err)
	}
	defer pool.Close()

	instance, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to get pdfium instance: %w", err)
	}

	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &pdfData})
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}

	var allLines []textLine
	for i := 0; i < pageCount.PageCount; i++ {
		textResult, err := instance.GetPageTextStructured(&requests.GetPageTextStructured{
			Page: requests.Page{
				ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i},
			},
			Mode:                   requests.GetPageTextStructuredModeRects,
			CollectFontInformation: true,
		})
		if err != nil {
			continue
		}
		pageLines := groupRectsIntoLines(textResult.Rects)
		allLines = append(allLines, pageLines...)
	}
	return allLines, nil
}

// rectInfo holds per-rect data extracted from PDF structured text
type rectInfo struct {
	text     string
	x        float64
	right    float64
	y        float64
	fontSize float64
	isBold   bool
	fontName string
}

// groupRectsIntoLines groups pre-segmented text rects by Y-coordinate into lines.
func groupRectsIntoLines(rects []*responses.GetPageTextStructuredRect) []textLine {
	if len(rects) == 0 {
		return nil
	}

	var infos []rectInfo
	for _, r := range rects {
		if strings.TrimSpace(r.Text) == "" {
			continue
		}

		ri := rectInfo{
			text:  r.Text, // keep trailing spaces — they encode word boundaries
			x:     r.PointPosition.Left,
			right: r.PointPosition.Right,
			y:     r.PointPosition.Top,
		}

		if r.FontInformation != nil {
			ri.fontSize = r.FontInformation.Size
			ri.fontName = r.FontInformation.Name
			ri.isBold = r.FontInformation.Flags&(1<<18) != 0 ||
				strings.Contains(strings.ToLower(r.FontInformation.Name), "bold")
		}

		infos = append(infos, ri)
	}

	if len(infos) == 0 {
		return nil
	}

	// Sort by Y (top-to-bottom), then X (left-to-right).
	// Use generous Y tolerance since character baselines vary within a line.
	sort.Slice(infos, func(i, j int) bool {
		avgSize := (infos[i].fontSize + infos[j].fontSize) / 2
		yTol := avgSize * 0.4
		if yTol < 2.0 {
			yTol = 2.0
		}
		if math.Abs(infos[i].y-infos[j].y) <= yTol {
			return infos[i].x < infos[j].x
		}
		return infos[i].y < infos[j].y
	})

	// Remove overlapping duplicate rects
	infos = deduplicateOverlappingRects(infos)

	// Group rects into lines by Y-proximity.
	// Use wider tolerance since individual character baselines vary (ascenders/descenders).
	var lines []textLine
	var currentGroup []rectInfo
	var groupMinY, groupMaxY float64

	for _, ri := range infos {
		lineHeight := ri.fontSize * 0.5
		if lineHeight < 3.0 {
			lineHeight = 3.0
		}

		if len(currentGroup) > 0 {
			// Check if this rect belongs to the current line
			withinLine := ri.y >= groupMinY-lineHeight && ri.y <= groupMaxY+lineHeight
			if !withinLine {
				lines = append(lines, mergeRectsToLine(currentGroup))
				currentGroup = nil
			}
		}

		currentGroup = append(currentGroup, ri)
		if len(currentGroup) == 1 {
			groupMinY = ri.y
			groupMaxY = ri.y
		} else {
			if ri.y < groupMinY {
				groupMinY = ri.y
			}
			if ri.y > groupMaxY {
				groupMaxY = ri.y
			}
		}
	}

	if len(currentGroup) > 0 {
		lines = append(lines, mergeRectsToLine(currentGroup))
	}

	return lines
}

// deduplicateOverlappingRects removes shorter rects that overlap with longer ones.
// Chrome sometimes renders text twice: a short prefix then the full word at the same position.
func deduplicateOverlappingRects(rects []rectInfo) []rectInfo {
	if len(rects) <= 1 {
		return rects
	}

	var result []rectInfo

	for i := 0; i < len(rects); i++ {
		if i+1 < len(rects) {
			curr := rects[i]
			next := rects[i+1]
			// If next rect starts inside current rect and has longer text, skip current
			if next.x >= curr.x-1 && next.x < curr.right &&
				len(strings.TrimSpace(next.text)) > len(strings.TrimSpace(curr.text)) {
				continue
			}
		}
		result = append(result, rects[i])
	}

	return result
}

// mergeRectsToLine merges a group of text rects on the same line into a single textLine.
// Rect text already includes trailing spaces for word boundaries, so we just concatenate.
func mergeRectsToLine(rects []rectInfo) textLine {
	var text strings.Builder
	var totalSize float64
	var boldCount int
	var fontName string

	for _, r := range rects {
		text.WriteString(r.text)
		totalSize += r.fontSize
		if r.isBold {
			boldCount++
		}
		if fontName == "" && r.fontName != "" {
			fontName = r.fontName
		}
	}

	avgSize := totalSize / float64(len(rects))

	return textLine{
		Text:     strings.TrimSpace(text.String()),
		Y:        rects[0].y,
		FontSize: avgSize,
		IsBold:   boldCount > len(rects)/2,
		FontName: fontName,
	}
}

// groupCharsIntoLines groups characters by Y-coordinate into text lines.
func groupCharsIntoLines(chars []*responses.GetPageTextStructuredChar) []textLine {
	if len(chars) == 0 {
		return nil
	}

	var infos []charInfo
	for _, c := range chars {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}

		ci := charInfo{
			text: c.Text,
			x:    c.PointPosition.Left,
			y:    c.PointPosition.Top,
		}

		if c.FontInformation != nil {
			ci.fontSize = c.FontInformation.Size
			ci.fontName = c.FontInformation.Name
			// PDF spec 1.7 Section 5.7.1: bit 19 (0-indexed bit 18) = ForceBold
			ci.isBold = c.FontInformation.Flags&(1<<18) != 0 ||
				strings.Contains(strings.ToLower(c.FontInformation.Name), "bold")
		}

		infos = append(infos, ci)
	}

	if len(infos) == 0 {
		return nil
	}

	// Sort by Y (top-to-bottom), then X (left-to-right)
	sort.Slice(infos, func(i, j int) bool {
		if math.Abs(infos[i].y-infos[j].y) < 1.0 {
			return infos[i].x < infos[j].x
		}
		return infos[i].y < infos[j].y
	})

	// Group into lines by Y-proximity
	var lines []textLine
	var currentLine []charInfo
	currentY := infos[0].y

	for _, ci := range infos {
		tolerance := ci.fontSize * 0.3
		if tolerance < 1.0 {
			tolerance = 1.0
		}

		if math.Abs(ci.y-currentY) > tolerance && len(currentLine) > 0 {
			lines = append(lines, mergeCharsToLine(currentLine))
			currentLine = nil
			currentY = ci.y
		}

		currentLine = append(currentLine, ci)
		if len(currentLine) == 1 {
			currentY = ci.y
		}
	}

	if len(currentLine) > 0 {
		lines = append(lines, mergeCharsToLine(currentLine))
	}

	return lines
}

// mergeCharsToLine merges a group of characters into a single textLine.
func mergeCharsToLine(chars []charInfo) textLine {
	var text strings.Builder
	var totalSize float64
	var boldCount int
	var fontName string

	for i, c := range chars {
		if i > 0 {
			gap := c.x - chars[i-1].x
			charWidth := chars[i-1].fontSize * 0.6
			if charWidth < 1 {
				charWidth = 5
			}
			if gap > charWidth*1.5 {
				text.WriteString(" ")
			}
		}
		text.WriteString(c.text)
		totalSize += c.fontSize
		if c.isBold {
			boldCount++
		}
		if fontName == "" && c.fontName != "" {
			fontName = c.fontName
		}
	}

	avgSize := totalSize / float64(len(chars))

	return textLine{
		Text:     text.String(),
		Y:        chars[0].y,
		FontSize: avgSize,
		IsBold:   boldCount > len(chars)/2,
		FontName: fontName,
	}
}

// detectHeadings identifies heading lines based on font size distribution.
// The most frequent font size is body text; larger sizes are headings.
func detectHeadings(lines []textLine) []headingInfo {
	if len(lines) == 0 {
		return nil
	}

	// Build font size histogram (round to nearest 0.5pt for clustering)
	sizeCount := make(map[float64]int)
	for _, l := range lines {
		rounded := math.Round(l.FontSize*2) / 2
		sizeCount[rounded] += len(l.Text)
	}

	// Find body size (most frequent by character count)
	var bodySize float64
	var maxCount int
	for size, count := range sizeCount {
		if count > maxCount {
			maxCount = count
			bodySize = size
		}
	}

	// Collect distinct heading sizes (larger than body)
	headingSizeSet := make(map[float64]bool)
	for size := range sizeCount {
		if size > bodySize+0.5 {
			headingSizeSet[size] = true
		}
	}

	if len(headingSizeSet) == 0 {
		return nil
	}

	// Sort heading sizes descending → largest = level 1
	var headingSizes []float64
	for size := range headingSizeSet {
		headingSizes = append(headingSizes, size)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(headingSizes)))

	sizeToLevel := make(map[float64]int)
	for i, size := range headingSizes {
		sizeToLevel[size] = i + 1
	}

	// Map lines to headings
	var headings []headingInfo
	for i, l := range lines {
		rounded := math.Round(l.FontSize*2) / 2
		if level, ok := sizeToLevel[rounded]; ok {
			headings = append(headings, headingInfo{
				LineIdx: i,
				Level:   level,
			})
		}
	}

	return headings
}

// buildSectionsFromLines constructs a section tree from text lines and detected headings.
func buildSectionsFromLines(lines []textLine, headings []headingInfo) []*Section {
	if len(lines) == 0 {
		return nil
	}

	// Build a set of heading line indices for quick lookup
	headingMap := make(map[int]int) // lineIdx → level
	for _, h := range headings {
		headingMap[h.LineIdx] = h.Level
	}

	// If no headings detected, create a single section with all content
	if len(headings) == 0 {
		var content strings.Builder
		for _, l := range lines {
			content.WriteString(l.Text)
			content.WriteString("\n")
		}
		text := strings.TrimSpace(content.String())
		return []*Section{{
			Level:   1,
			Title:   truncateTitle(lines[0].Text),
			Content: text,
			Tokens:  estimateTokens(text),
		}}
	}

	// Walk lines, creating sections at heading boundaries
	var allSections []*Section
	var currentSection *Section
	var contentBuilder strings.Builder

	for i, l := range lines {
		if level, isHeading := headingMap[i]; isHeading {
			// Finalize previous section
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(contentBuilder.String())
				currentSection.Tokens = estimateTokens(currentSection.Content)
			}

			currentSection = &Section{
				Level: level,
				Title: strings.TrimSpace(l.Text),
			}
			allSections = append(allSections, currentSection)
			contentBuilder.Reset()
		} else if currentSection != nil {
			contentBuilder.WriteString(l.Text)
			contentBuilder.WriteString("\n")
		}
		// Lines before the first heading are dropped (usually nav/header chrome)
	}

	// Finalize last section
	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(contentBuilder.String())
		currentSection.Tokens = estimateTokens(currentSection.Content)
	}

	// Build tree and calculate cumulative tokens
	roots := buildTree(allSections)
	return roots
}
