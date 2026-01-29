package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordanCoin/docmap/parser"
	"github.com/JordanCoin/docmap/render"
)

// JSON output structures
type JSONOutput struct {
	Root        string         `json:"root"`
	TotalTokens int            `json:"total_tokens"`
	TotalDocs   int            `json:"total_docs"`
	Documents   []JSONDocument `json:"documents"`
}

type JSONDocument struct {
	Filename   string        `json:"filename"`
	Tokens     int           `json:"tokens"`
	Sections   []JSONSection `json:"sections"`
	References []JSONRef     `json:"references,omitempty"`
}

type JSONSection struct {
	Level    int           `json:"level"`
	Title    string        `json:"title"`
	Tokens   int           `json:"tokens"`
	KeyTerms []string      `json:"key_terms,omitempty"`
	Children []JSONSection `json:"children,omitempty"`
}

type JSONRef struct {
	Text   string `json:"text"`
	Target string `json:"target"`
	Line   int    `json:"line"`
}

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Check for help/version flags first
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--help", "-h":
			printUsage()
			return
		case "--version", "-v":
			fmt.Printf("docmap %s\n", version)
			return
		}
	}

	target := os.Args[1]

	// Parse flags
	var sectionFilter string
	var expandSection string
	var showRefs bool
	var jsonMode bool
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--section", "-s":
			if i+1 < len(os.Args) {
				sectionFilter = os.Args[i+1]
				i++
			}
		case "--expand", "-e":
			if i+1 < len(os.Args) {
				expandSection = os.Args[i+1]
				i++
			}
		case "--refs", "-r":
			showRefs = true
		case "--json", "-j":
			jsonMode = true
		}
	}

	// Check if target is a directory
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		// Multi-file mode: find all .md files
		docs := parseDirectory(target)
		if len(docs) == 0 {
			fmt.Println("No markdown or PDF files found")
			os.Exit(1)
		}
		if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON(docs, absPath)
		} else if showRefs {
			render.RefsTree(docs, target)
		} else {
			render.MultiTree(docs, target)
		}
	} else {
		// Single file mode
		var doc *parser.Document

		if strings.HasSuffix(strings.ToLower(target), ".pdf") {
			// PDF file
			var err error
			doc, err = parser.ParsePDF(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing PDF: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Markdown file
			content, err := os.ReadFile(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
				os.Exit(1)
			}
			doc = parser.Parse(string(content))
		}

		parts := strings.Split(target, "/")
		doc.Filename = parts[len(parts)-1]

		if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON([]*parser.Document{doc}, absPath)
		} else if expandSection != "" {
			render.ExpandSection(doc, expandSection)
		} else if sectionFilter != "" {
			render.FilteredTree(doc, sectionFilter)
		} else {
			render.Tree(doc)
		}
	}
}

func parseDirectory(dir string) []*parser.Document {
	var docs []*parser.Document

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		lowerPath := strings.ToLower(path)
		isMd := strings.HasSuffix(lowerPath, ".md")
		isPdf := strings.HasSuffix(lowerPath, ".pdf")

		if !isMd && !isPdf {
			return nil
		}

		// Skip hidden files
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			return nil
		}

		var doc *parser.Document

		if isPdf {
			var err error
			doc, err = parser.ParsePDF(path)
			if err != nil {
				// Skip PDFs that can't be parsed
				return nil
			}
		} else {
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			doc = parser.Parse(string(content))
		}

		// Get relative path from dir
		relPath, _ := filepath.Rel(dir, path)
		doc.Filename = relPath

		docs = append(docs, doc)
		return nil
	})

	return docs
}

func outputJSON(docs []*parser.Document, root string) {
	output := JSONOutput{
		Root:      root,
		TotalDocs: len(docs),
	}

	for _, doc := range docs {
		jsonDoc := JSONDocument{
			Filename: doc.Filename,
			Tokens:   doc.TotalTokens,
			Sections: convertSections(doc.Sections),
		}

		// Add references
		for _, ref := range doc.References {
			jsonDoc.References = append(jsonDoc.References, JSONRef{
				Text:   ref.Text,
				Target: ref.Target,
				Line:   ref.Line,
			})
		}

		output.Documents = append(output.Documents, jsonDoc)
		output.TotalTokens += doc.TotalTokens
	}

	json.NewEncoder(os.Stdout).Encode(output)
}

func convertSections(sections []*parser.Section) []JSONSection {
	var result []JSONSection
	for _, s := range sections {
		js := JSONSection{
			Level:    s.Level,
			Title:    s.Title,
			Tokens:   s.Tokens,
			KeyTerms: s.KeyTerms,
			Children: convertSections(s.Children),
		}
		result = append(result, js)
	}
	return result
}

func printUsage() {
	fmt.Println(`docmap - instant documentation structure for LLMs and humans

Usage:
  docmap <file.md|file.pdf|dir> [flags]

Examples:
  docmap .                          # All markdown and PDF files in directory
  docmap README.md                  # Single markdown file deep dive
  docmap report.pdf                 # Single PDF file structure
  docmap docs/                      # Specific folder
  docmap README.md --section "API"  # Filter to section
  docmap README.md --expand "API"   # Show section content
  docmap . --refs                   # Show cross-references between docs

Flags:
  -s, --section <name>   Filter to a specific section
  -e, --expand <name>    Show full content of a section
  -r, --refs             Show cross-references between markdown files
  -j, --json             Output JSON format
  -v, --version          Print version
  -h, --help             Show this help

PDF Support:
  PDFs with outlines show document structure; tokens are estimated.
  PDFs without outlines fall back to page-by-page structure.

More info: https://github.com/JordanCoin/docmap`)
}
