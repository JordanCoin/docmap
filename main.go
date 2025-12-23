package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordanCoin/docmap/parser"
	"github.com/JordanCoin/docmap/render"
)

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
			fmt.Println("No markdown files found")
			os.Exit(1)
		}
		if showRefs {
			render.RefsTree(docs, target)
		} else {
			render.MultiTree(docs, target)
		}
	} else {
		// Single file mode
		content, err := os.ReadFile(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		doc := parser.Parse(string(content))
		parts := strings.Split(target, "/")
		doc.Filename = parts[len(parts)-1]

		if expandSection != "" {
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
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		// Skip hidden files and common non-doc files
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		doc := parser.Parse(string(content))
		// Get relative path from dir
		relPath, _ := filepath.Rel(dir, path)
		doc.Filename = relPath

		docs = append(docs, doc)
		return nil
	})

	return docs
}

func printUsage() {
	fmt.Println(`docmap - instant documentation structure for LLMs and humans

Usage:
  docmap <file.md|dir> [flags]

Examples:
  docmap .                          # All markdown files in directory
  docmap README.md                  # Single file deep dive
  docmap docs/                      # Specific folder
  docmap README.md --section "API"  # Filter to section
  docmap README.md --expand "API"   # Show section content
  docmap . --refs                   # Show cross-references between docs

Flags:
  -s, --section <name>   Filter to a specific section
  -e, --expand <name>    Show full content of a section
  -r, --refs             Show cross-references between markdown files
  -v, --version          Print version
  -h, --help             Show this help

More info: https://github.com/JordanCoin/docmap`)
}
