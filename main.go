package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JordanCoin/docmap/parser"
	"github.com/JordanCoin/docmap/render"
)

// StdinManifest represents the JSON manifest read from stdin
type StdinManifest struct {
	Root  string         `json:"root"`
	Files []ManifestFile `json:"files"`
}

// ManifestFile represents a single file in the stdin manifest
type ManifestFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

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
	Summary    JSONSummary   `json:"summary"`
	Sections   []JSONSection `json:"sections"`
	Nodes      []JSONNode    `json:"nodes,omitempty"`
	References []JSONRef     `json:"references,omitempty"`
}

// JSONSummary mirrors parser.ContentSummary for JSON consumers.
type JSONSummary struct {
	Callouts     int `json:"callouts,omitempty"`
	Tables       int `json:"tables,omitempty"`
	CodeBlocks   int `json:"code_blocks,omitempty"`
	MathBlocks   int `json:"math_blocks,omitempty"`
	HTMLBlocks   int `json:"html_blocks,omitempty"`
	Footnotes    int `json:"footnotes,omitempty"`
	DefLists     int `json:"definition_lists,omitempty"`
	LinkRefDefs  int `json:"link_ref_defs,omitempty"`
	Tasks        int `json:"tasks,omitempty"`
	TasksChecked int `json:"tasks_checked,omitempty"`
	WikiLinks    int `json:"wiki_links,omitempty"`
	WikiEmbeds   int `json:"wiki_embeds,omitempty"`
	Mentions     int `json:"mentions,omitempty"`
	IssueRefs    int `json:"issue_refs,omitempty"`
	CommitRefs   int `json:"commit_refs,omitempty"`
	Emojis       int `json:"emojis,omitempty"`
}

type JSONSection struct {
	Level     int           `json:"level"`
	Title     string        `json:"title"`
	Tokens    int           `json:"tokens"`
	LineStart int           `json:"line_start,omitempty"`
	LineEnd   int           `json:"line_end,omitempty"`
	KeyTerms  []string      `json:"key_terms,omitempty"`
	Notables  []JSONNode    `json:"notables,omitempty"`
	Children  []JSONSection `json:"children,omitempty"`
}

// JSONNode is the typed-AST-aware serialization format. `Kind` identifies
// the node type (e.g. "code_block", "callout"); remaining fields are
// populated per kind. Agents can switch on Kind to deserialize.
type JSONNode struct {
	Kind      string     `json:"kind"`
	LineStart int        `json:"line_start,omitempty"`
	LineEnd   int        `json:"line_end,omitempty"`
	Tokens    int        `json:"tokens,omitempty"`
	Title     string     `json:"title,omitempty"`    // Heading
	Level     int        `json:"level,omitempty"`    // Heading
	Language  string     `json:"language,omitempty"` // CodeBlock
	Code      string     `json:"code,omitempty"`     // CodeBlock
	Variant   string     `json:"variant,omitempty"`  // Callout
	Headers   []string   `json:"headers,omitempty"`  // Table
	Aligns    []string   `json:"aligns,omitempty"`   // Table
	TeX       string     `json:"tex,omitempty"`      // MathBlock / InlineMath
	ID        string     `json:"id,omitempty"`       // FootnoteDef
	Label     string     `json:"label,omitempty"`    // LinkRefDef
	URL       string     `json:"url,omitempty"`      // LinkRefDef / Link
	Checked   *bool      `json:"checked,omitempty"`  // TaskItem
	Raw       string     `json:"raw,omitempty"`      // HTMLBlock / Frontmatter
	Format    string     `json:"format,omitempty"`   // Frontmatter
	Target    string     `json:"target,omitempty"`   // WikiLink / WikiEmbed
	Children  []JSONNode `json:"children,omitempty"`
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

	// Check for help/version flags first (before full parse)
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

	// Parse flags (scan all args for flags first)
	var sectionFilter string
	var expandSection string
	var searchQuery string
	var typeFilter string
	var langFilter string
	var kindFilter string
	var atLine int
	var sinceRef string
	var showRefs bool
	var jsonMode bool
	var stdinMode bool
	var target string

	for i := 1; i < len(os.Args); i++ {
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
		case "--type", "-t":
			if i+1 < len(os.Args) {
				typeFilter = os.Args[i+1]
				i++
			}
		case "--lang":
			if i+1 < len(os.Args) {
				langFilter = os.Args[i+1]
				i++
			}
		case "--kind":
			if i+1 < len(os.Args) {
				kindFilter = os.Args[i+1]
				i++
			}
		case "--at":
			if i+1 < len(os.Args) {
				n, err := strconv.Atoi(os.Args[i+1])
				if err == nil {
					atLine = n
				}
				i++
			}
		case "--since":
			if i+1 < len(os.Args) {
				sinceRef = os.Args[i+1]
				i++
			}
		case "--refs", "-r":
			showRefs = true
		case "--json", "-j":
			jsonMode = true
		case "--stdin":
			stdinMode = true
		default:
			if target == "" {
				target = os.Args[i]
			}
		}
	}

	// Handle --stdin mode
	if stdinMode {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}

		var manifest StdinManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing JSON manifest: %v\n", err)
			os.Exit(1)
		}

		if manifest.Root == "" {
			fmt.Fprintf(os.Stderr, "Error: manifest missing 'root' field\n")
			os.Exit(1)
		}

		// Create temp directory and write files
		tmpDir, err := os.MkdirTemp("", "docmap-stdin-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temp directory: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tmpDir)

		for _, f := range manifest.Files {
			destPath := filepath.Join(tmpDir, f.Path)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory for %s: %v\n", f.Path, err)
				os.Exit(1)
			}
			if err := os.WriteFile(destPath, []byte(f.Content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", f.Path, err)
				os.Exit(1)
			}
		}

		// Parse the temp directory
		docs := parseDirectory(tmpDir)
		if len(docs) == 0 {
			fmt.Println("No markdown, PDF, or YAML files found")
			os.Exit(1)
		}

		if jsonMode {
			outputJSON(docs, manifest.Root)
		} else if searchQuery != "" {
			render.SearchResults(docs, searchQuery)
		} else if showRefs {
			render.RefsTree(docs, manifest.Root)
		} else {
			render.MultiTree(docs, manifest.Root)
		}
		return
	}

	if target == "" {
		printUsage()
		os.Exit(1)
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

		if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON([]*parser.Document{doc}, absPath)
		} else if searchQuery != "" {
			render.SearchResults([]*parser.Document{doc}, searchQuery)
		} else if sinceRef != "" {
			changed, _ := parser.ChangedLines(target, sinceRef)
			render.ChangedSince(doc, changed, sinceRef)
		} else if atLine > 0 {
			render.AtLine(doc, atLine)
		} else if typeFilter != "" {
			render.TypeFilterFiltered(doc, typeFilter, langFilter, kindFilter)
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
			Summary:  convertSummary(doc.Summary()),
			Sections: convertSections(doc.Sections),
			Nodes:    convertNodeList(doc.Nodes),
		}

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

func convertSummary(s parser.ContentSummary) JSONSummary {
	return JSONSummary{
		Callouts:     s.Callouts,
		Tables:       s.Tables,
		CodeBlocks:   s.CodeBlocks,
		MathBlocks:   s.MathBlocks,
		HTMLBlocks:   s.HTMLBlocks,
		Footnotes:    s.Footnotes,
		DefLists:     s.DefLists,
		LinkRefDefs:  s.LinkRefDefs,
		Tasks:        s.Tasks,
		TasksChecked: s.TasksChecked,
		WikiLinks:    s.WikiLinks,
		WikiEmbeds:   s.WikiEmbeds,
		Mentions:     s.Mentions,
		IssueRefs:    s.IssueRefs,
		CommitRefs:   s.CommitRefs,
		Emojis:       s.Emojis,
	}
}

func convertSections(sections []*parser.Section) []JSONSection {
	var result []JSONSection
	for _, s := range sections {
		js := JSONSection{
			Level:     s.Level,
			Title:     s.Title,
			Tokens:    s.Tokens,
			LineStart: s.LineStart,
			LineEnd:   s.LineEnd,
			KeyTerms:  s.KeyTerms,
			Notables:  convertNodeList(s.Notables),
			Children:  convertSections(s.Children),
		}
		result = append(result, js)
	}
	return result
}

func convertNodeList(nodes []parser.Node) []JSONNode {
	var out []JSONNode
	for _, n := range nodes {
		out = append(out, convertNode(n))
	}
	return out
}

// convertNode serializes one typed AST node into JSON-friendly form.
// Only fields relevant to the kind are populated; omitempty keeps the
// output compact.
func convertNode(n parser.Node) JSONNode {
	j := JSONNode{
		Kind:      string(n.Kind()),
		LineStart: n.LineStart(),
		LineEnd:   n.LineEnd(),
		Tokens:    n.Tokens(),
	}
	switch v := n.(type) {
	case *parser.Heading:
		j.Title = v.Title
		j.Level = v.Level
	case *parser.CodeBlock:
		j.Language = v.Language
		j.Code = v.Code
	case *parser.Callout:
		j.Variant = string(v.Variant)
	case *parser.Table:
		j.Headers = v.Headers
		for _, a := range v.Aligns {
			j.Aligns = append(j.Aligns, string(a))
		}
	case *parser.MathBlock:
		j.TeX = v.TeX
	case *parser.InlineMath:
		j.TeX = v.TeX
	case *parser.FootnoteDef:
		j.ID = v.ID
	case *parser.LinkRefDef:
		j.Label = v.Label
		j.URL = v.URL
	case *parser.Link:
		j.URL = v.URL
		j.Title = v.Text
	case *parser.TaskItem:
		checked := v.Checked
		j.Checked = &checked
	case *parser.HTMLBlock:
		j.Raw = v.Raw
	case *parser.Frontmatter:
		j.Raw = v.Raw
		j.Format = string(v.Format)
	case *parser.WikiLink:
		j.Target = v.Target
	case *parser.WikiEmbed:
		j.Target = v.Target
	}
	// Recurse into children for container nodes so the JSON tree mirrors
	// the in-memory AST.
	for _, c := range n.Children() {
		j.Children = append(j.Children, convertNode(c))
	}
	return j
}

func printUsage() {
	fmt.Println(`docmap - instant documentation structure for LLMs and humans

Usage:
  docmap <file.md|file.pdf|file.yaml|dir> [flags]
  docmap --stdin [flags] < manifest.json

Examples:
  docmap .                          # All markdown, PDF, and YAML files
  docmap README.md                  # Single markdown file deep dive
  docmap report.pdf                 # Single PDF file structure
  docmap config.yaml                # Single YAML file structure
  docmap docs/                      # Specific folder
  docmap README.md --section "API"  # Filter to section
  docmap README.md --expand "API"   # Show section content
  docmap . --refs                   # Show cross-references between docs
  docmap docs/ --search "auth"     # Search across all files
  docmap --stdin --json < manifest.json  # Parse files from JSON manifest

Flags:
  --stdin                Read JSON file manifest from stdin (no filesystem access needed)
  --search <query>       Search sections across all files
  -s, --section <name>   Filter to a specific section
  -e, --expand <name>    Show full content of a section
  -t, --type <kind>      Drill into one construct: code, callout, table, math,
                         footnote, deflist, linkref, html, task, wiki, embed,
                         mention, issue, sha, emoji
  --lang <name>          Sub-filter for --type code (e.g. --type code --lang python)
  --kind <name>          Sub-filter for --type callout (e.g. --kind warning)
  --at <line>            Show what construct lives at a specific line number
  --since <ref>          Show constructs on lines changed since a git ref
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

More info: https://github.com/JordanCoin/docmap`)
}
