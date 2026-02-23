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
	var searchQuery string
	var outputFile string
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
		case "--search":
			if i+1 < len(os.Args) {
				searchQuery = os.Args[i+1]
				i++
			}
		case "--output", "-o":
			if i+1 < len(os.Args) {
				outputFile = os.Args[i+1]
				i++
			}
		case "--refs", "-r":
			showRefs = true
		case "--json", "-j":
			jsonMode = true
		}
	}

	// Check if target is a URL
	isURL := strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")
	if isURL {
		doc, err := parser.ParseURL(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Use the URL's last path segment as filename
		parts := strings.Split(strings.TrimRight(target, "/"), "/")
		doc.Filename = parts[len(parts)-1]
		if doc.Filename == "" {
			doc.Filename = target
		}

		if outputFile != "" {
			yamlContent, err := parser.ExportYAML(doc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting YAML: %v\n", err)
				os.Exit(1)
			}
			if err := os.WriteFile(outputFile, []byte(yamlContent), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Saved to %s\n", outputFile)
		}

		if jsonMode {
			outputJSON([]*parser.Document{doc}, target)
		} else if searchQuery != "" {
			render.SearchResults([]*parser.Document{doc}, searchQuery)
		} else if expandSection != "" {
			render.ExpandSection(doc, expandSection)
		} else if sectionFilter != "" {
			render.FilteredTree(doc, sectionFilter)
		} else if outputFile == "" {
			render.Tree(doc)
		}
		return
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
			fmt.Println("No markdown, PDF, or YAML files found")
			os.Exit(1)
		}
		if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON(docs, absPath)
		} else if searchQuery != "" {
			render.SearchResults(docs, searchQuery)
		} else if showRefs {
			render.RefsTree(docs, target)
		} else {
			render.MultiTree(docs, target)
		}
	} else {
		// Single file mode
		var doc *parser.Document

		lower := strings.ToLower(target)
		if strings.HasSuffix(lower, ".pdf") {
			// PDF file
			var err error
			doc, err = parser.ParsePDF(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing PDF: %v\n", err)
				os.Exit(1)
			}
		} else if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
			// YAML file
			content, err := os.ReadFile(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
				os.Exit(1)
			}
			doc, err = parser.ParseYAML(string(content))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
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

		if outputFile != "" {
			yamlContent, err := parser.ExportYAML(doc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting YAML: %v\n", err)
				os.Exit(1)
			}
			if err := os.WriteFile(outputFile, []byte(yamlContent), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Saved to %s\n", outputFile)
		}

		if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON([]*parser.Document{doc}, absPath)
		} else if searchQuery != "" {
			render.SearchResults([]*parser.Document{doc}, searchQuery)
		} else if expandSection != "" {
			render.ExpandSection(doc, expandSection)
		} else if sectionFilter != "" {
			render.FilteredTree(doc, sectionFilter)
		} else if outputFile == "" {
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
		isYaml := strings.HasSuffix(lowerPath, ".yaml") || strings.HasSuffix(lowerPath, ".yml")

		if !isMd && !isPdf && !isYaml {
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
		} else if isYaml {
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			doc, err = parser.ParseYAML(string(content))
			if err != nil {
				// Skip YAML files that can't be parsed
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
  docmap <file.md|file.pdf|file.yaml|url|dir> [flags]

Examples:
  docmap .                          # All markdown, PDF, and YAML files
  docmap README.md                  # Single markdown file deep dive
  docmap report.pdf                 # Single PDF file structure
  docmap config.yaml                # Single YAML file structure
  docmap docs/                      # Specific folder
  docmap README.md --section "API"  # Filter to section
  docmap README.md --expand "API"   # Show section content
  docmap . --refs                   # Show cross-references between docs
  docmap docs/ --search "auth"      # Search across all files

URL Support:
  docmap https://example.com/docs                     # Map a web page
  docmap https://example.com/docs --search "auth"     # Search sections
  docmap https://example.com/docs -o docs.yaml        # Save as YAML
  docmap docs.yaml --search "auth"                    # Fast local access

Flags:
  --search <query>       Search sections across all files
  -s, --section <name>   Filter to a specific section
  -e, --expand <name>    Show full content of a section
  -o, --output <file>    Export structure as YAML file
  -r, --refs             Show cross-references between markdown files
  -j, --json             Output JSON format
  -v, --version          Print version
  -h, --help             Show this help

PDF Support:
  PDFs with outlines show document structure; tokens are estimated.
  PDFs without outlines fall back to page-by-page structure.

YAML Support:
  Maps keys to sections with nested children. Sequences use name/id/title
  fields for titles when available, falling back to key: value or [N].

URL Support:
  Uses headless Chrome to render web pages, then extracts heading structure
  from font sizes. Requires Chrome/Chromium installed, or set CHROME_PATH.

More info: https://github.com/JordanCoin/docmap`)
}
