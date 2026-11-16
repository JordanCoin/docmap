package parser

import (
	"os"
	"path/filepath"
	"strings"
)

// ParseFile reads path, optionally consults the on-disk mtime parse cache
// in the user cache dir, parses, and stores a fresh entry. Set
// DOCMAP_NO_CACHE=1 to skip the cache entirely.
// Cache I/O errors are ignored.
func ParseFile(path string) (*Document, error) {
	lower := strings.ToLower(path)
	isPDF := strings.HasSuffix(lower, ".pdf")
	// An external PDF text command changes the result without changing the
	// file, so its output is never cached.
	if isPDF && os.Getenv(PDFTextCommandEnv) != "" {
		return ParsePDF(path)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	mtimeNS := info.ModTime().UnixNano()
	size := info.Size()

	if doc, ok := cacheGet(abs, mtimeNS, size); ok {
		return doc, nil
	}

	if isPDF {
		doc, err := ParsePDF(path)
		if err != nil {
			return nil, err
		}
		cachePut(abs, mtimeNS, size, doc)
		return doc, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Re-stat after read so the cache key matches what we parsed.
	if info2, err := os.Stat(path); err == nil {
		mtimeNS = info2.ModTime().UnixNano()
		size = info2.Size()
	}

	var doc *Document
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		doc, err = ParseYAML(string(content))
		if err != nil {
			return nil, err
		}
	} else if strings.HasSuffix(lower, ".txt") {
		doc = ParseText(string(content))
	} else {
		doc = Parse(string(content))
	}

	cachePut(abs, mtimeNS, size, doc)
	return doc, nil
}
