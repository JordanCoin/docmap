package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	Since       string         `json:"since,omitempty"`
	TotalTokens int            `json:"total_tokens"`
	TotalDocs   int            `json:"total_docs"`
	Documents   []JSONDocument `json:"documents"`
}

type JSONDocument struct {
	Filename     string        `json:"filename"`
	Change       string        `json:"change,omitempty"`
	OldPath      string        `json:"old_path,omitempty"`
	ChangedLines []int         `json:"changed_lines,omitempty"`
	Tokens       int           `json:"tokens"`
	Summary      JSONSummary   `json:"summary"`
	Sections     []JSONSection `json:"sections"`
	Nodes        []JSONNode    `json:"nodes,omitempty"`
	References   []JSONRef     `json:"references,omitempty"`
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
	var walkAll bool
	var target string
	var targets []string
	var termsFile string
	var compact bool
	var brief bool
	var mentionPaths []string
	var mentionsFlag bool

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
			if i+1 >= len(os.Args) || strings.HasPrefix(os.Args[i+1], "--") {
				fmt.Fprintln(os.Stderr, "Error: --search requires a query")
				os.Exit(1)
			}
			searchQuery = os.Args[i+1]
			i++
		case "--terms-file":
			if i+1 >= len(os.Args) || strings.HasPrefix(os.Args[i+1], "--") {
				fmt.Fprintln(os.Stderr, "Error: --terms-file requires a path")
				os.Exit(1)
			}
			termsFile = os.Args[i+1]
			i++
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
			if i+1 >= len(os.Args) || strings.HasPrefix(os.Args[i+1], "--") {
				fmt.Fprintln(os.Stderr, "Error: --since requires a git ref")
				os.Exit(1)
			}
			sinceRef = os.Args[i+1]
			i++
		case "--refs", "-r":
			showRefs = true
		case "--json", "-j":
			jsonMode = true
		case "--stdin":
			stdinMode = true
		case "--compact":
			compact = true
		case "--all":
			walkAll = true
		case "--brief":
			brief = true
		case "--mentions":
			mentionsFlag = true
			if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "--") {
				mentionPaths = append(mentionPaths, splitMentionArg(os.Args[i+1])...)
				i++
			}
		default:
			targets = append(targets, os.Args[i])
		}
	}
	if len(targets) > 0 {
		target = targets[0]
	}
	if compact && searchQuery == "" && termsFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --compact requires --search or --terms-file")
		os.Exit(1)
	}
	if mentionsFlag && len(mentionPaths) == 0 && sinceRef == "" {
		if stdinIsPipe() {
			mentionPaths = append(mentionPaths, readMentionLines(os.Stdin)...)
		}
	}
	if mentionsFlag && len(mentionPaths) == 0 && sinceRef == "" {
		fmt.Fprintln(os.Stderr, "Error: --mentions requires a path, piped names, or --since <ref>")
		os.Exit(1)
	}
	terms, err := readTerms(searchQuery, termsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(targets) > 1 && len(terms) == 0 {
		fmt.Fprintln(os.Stderr, "Error: multiple paths are supported only with --search or --terms-file")
		os.Exit(1)
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

		if len(terms) > 0 {
			outputSearch(docs, terms, jsonMode, compact)
		} else if jsonMode {
			outputJSON(docs, manifest.Root)
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

	if len(targets) > 1 {
		var docs []*parser.Document
		for _, path := range targets {
			parsed, err := parsePath(path, walkAll)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			docs = append(docs, parsed...)
		}
		if len(docs) == 0 {
			fmt.Println("No markdown, PDF, or YAML files found")
			os.Exit(1)
		}
		outputSearch(docs, terms, jsonMode, compact)
		return
	}

	// Check if target is a directory
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if mentionsFlag && len(mentionPaths) == 0 && sinceRef != "" {
		mentionPaths = mentionPathsFromGit(target, sinceRef)
		if len(mentionPaths) == 0 {
			fmt.Println("No changed documentation paths since " + sinceRef)
			return
		}
	}

	if info.IsDir() {
		// Multi-file mode: find all .md files
		docs := parseDirectoryOpts(target, walkAll)
		if len(docs) == 0 {
			fmt.Println("No markdown, PDF, or YAML files found")
			os.Exit(1)
		}
		if len(terms) > 0 {
			outputSearch(docs, terms, jsonMode, compact)
		} else if sinceRef != "" && jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSONSince(docs, absPath, target, sinceRef)
		} else if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON(docs, absPath)
		} else if brief {
			outputBrief(docs, target)
		} else if mentionsFlag {
			outputMentions(docs, mentionPaths)
		} else if sinceRef != "" {
			outputChangedSince(docs, target, sinceRef)
		} else if expandSection != "" {
			outputExpand(docs, expandSection)
		} else if sectionFilter != "" {
			outputSection(docs, sectionFilter)
		} else if showRefs {
			render.RefsTree(docs, target)
		} else {
			render.MultiTree(docs, target)
		}
	} else {
		// Single file mode
		doc, err := parseSingleFile(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing file: %v\n", err)
			os.Exit(1)
		}
		doc.Filename = filepath.Base(target)

		if len(terms) > 0 {
			outputSearch([]*parser.Document{doc}, terms, jsonMode, compact)
		} else if sinceRef != "" && jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSONSince([]*parser.Document{doc}, absPath, filepath.Dir(target), sinceRef)
		} else if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON([]*parser.Document{doc}, absPath)
		} else if brief {
			outputBrief([]*parser.Document{doc}, filepath.Dir(target))
		} else if mentionsFlag {
			outputMentions([]*parser.Document{doc}, mentionPaths)
		} else if sinceRef != "" {
			changed, err := parser.ChangedLines(target, sinceRef)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			render.ChangedSince(doc, changed, sinceRef)
		} else if atLine > 0 && expandSection != "" {
			render.ExpandAtLine(doc, atLine)
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

// skipDirs are dependency and build caches that never hold project docs.
// Walking node_modules alone turns a 90-file repo into 1,700 "docs" (#4).
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	".next":        true,
	".cache":       true,
}

// gitTrackedSet returns the set of files git considers part of the project
// under dir (tracked plus untracked-but-not-ignored), keyed by path relative
// to dir. It returns nil when dir is not inside a git work tree or git is not
// available, in which case callers fall back to skipDirs only.
func gitTrackedSet(dir string) map[string]bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	if eval, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = eval
	}
	inside, err := exec.Command("git", "-C", abs, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return nil
	}
	cmd := exec.Command("git", "-C", abs, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	set := make(map[string]bool)
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel != "" {
			set[filepath.ToSlash(rel)] = true
		}
	}
	return set
}

func parseDirectory(dir string) []*parser.Document {
	return parseDirectoryOpts(dir, false)
}

// parseDirectoryOpts walks dir for markdown, PDF and YAML documents. Unless
// all is true it skips dependency directories and honors .gitignore.
func parseDirectoryOpts(dir string, all bool) []*parser.Document {
	var docs []*parser.Document

	var tracked map[string]bool
	if !all {
		tracked = gitTrackedSet(dir)
	}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if !all && path != dir && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
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

		// Honor .gitignore: inside a git work tree only project files count.
		if tracked != nil {
			rel, relErr := filepath.Rel(dir, path)
			if relErr == nil && !tracked[filepath.ToSlash(rel)] {
				return nil
			}
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

func outputChangedSince(docs []*parser.Document, root, ref string) {
	if err := parser.EnsureRef(root, ref); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	shown := 0
	seen := map[string]bool{}
	for _, doc := range docs {
		path := filepath.Join(root, doc.Filename)
		changed, err := parser.ChangedLines(path, ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "docmap: %s: %v\n", doc.Filename, err)
			continue
		}
		if len(changed) == 0 {
			continue
		}
		render.ChangedSince(doc, changed, ref)
		seen[filepath.ToSlash(doc.Filename)] = true
		shown++
	}
	for _, c := range deletedDocPaths(root, ref) {
		fmt.Printf("deleted: %s\n", c)
		shown++
	}
	for _, c := range renamedDocPaths(root, ref) {
		if seen[c.Path] {
			continue
		}
		fmt.Printf("renamed: %s → %s\n", c.OldPath, c.Path)
		shown++
	}
	if shown == 0 {
		render.ChangedSince(&parser.Document{Filename: root}, map[int]bool{}, ref)
	}
}

func deletedDocPaths(root, ref string) []string {
	changes, err := parser.ChangedPaths(root, ref)
	if err != nil {
		return nil
	}
	var out []string
	for _, c := range changes {
		if c.Status == "D" {
			out = append(out, c.Path)
		}
	}
	sort.Strings(out)
	return out
}

func renamedDocPaths(root, ref string) []parser.PathChange {
	changes, err := parser.ChangedPaths(root, ref)
	if err != nil {
		return nil
	}
	var out []parser.PathChange
	for _, c := range changes {
		if (c.Status == "R" || c.Status == "C") && c.OldPath != "" {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func changeStatusByPath(root, ref string) map[string]parser.PathChange {
	changes, err := parser.ChangedPaths(root, ref)
	if err != nil {
		return nil
	}
	out := map[string]parser.PathChange{}
	for _, c := range changes {
		out[c.Path] = c
	}
	return out
}

func mentionPathsFromGit(root, ref string) []string {
	changes, err := parser.ChangedPaths(root, ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, c := range changes {
		add(c.Path)
		add(c.OldPath)
	}
	return out
}

func outputBrief(docs []*parser.Document, root string) {
	totalSections := 0
	totalTokens := 0
	md := 0
	for _, d := range docs {
		totalTokens += d.TotalTokens
		totalSections += len(d.GetAllSections())
		if strings.HasSuffix(strings.ToLower(d.Filename), ".md") {
			md++
		}
	}
	fmt.Printf("%d files (%d md) · %d sections · ~%s tokens\n",
		len(docs), md, totalSections, briefTokens(totalTokens))

	recent := parser.RecentFiles(root, 5)
	if len(recent) == 0 {
		recent = recentByMtime(docs, root, 5)
	}
	if len(recent) > 0 {
		fmt.Printf("recent: %s\n", strings.Join(recent, ", "))
	}
}

func briefTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000.0)
	}
	return strconv.Itoa(n)
}

func recentByMtime(docs []*parser.Document, root string, limit int) []string {
	type rec struct {
		name string
		mod  int64
	}
	var rows []rec
	for _, d := range docs {
		info, err := os.Stat(filepath.Join(root, d.Filename))
		if err != nil {
			continue
		}
		rows = append(rows, rec{d.Filename, info.ModTime().UnixNano()})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].mod > rows[j].mod })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.name
	}
	return out
}

func outputMentions(docs []*parser.Document, paths []string) {
	seen := map[string]bool{}
	for _, p := range paths {
		needles := mentionNeedles(p)
		if len(needles) == 0 {
			continue
		}
		for _, doc := range docs {
			if mentionIsSelf(doc.Filename, p) {
				continue
			}
			for _, sec := range doc.GetAllSections() {
				if !sectionMentions(doc, sec, needles) {
					continue
				}
				line := doc.Filename + " > " + sec.Title
				if seen[line] {
					continue
				}
				seen[line] = true
				fmt.Println(line)
			}
		}
	}
	if len(seen) == 0 {
		fmt.Println("No sections mention those paths")
	}
}

func mentionIsSelf(filename, changed string) bool {
	return strings.EqualFold(filepath.ToSlash(filename), filepath.ToSlash(changed)) ||
		strings.EqualFold(filepath.Base(filename), filepath.Base(changed))
}

func mentionNeedles(p string) []string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if p == "" {
		return nil
	}
	var out []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if len(s) < 3 {
			return
		}
		for _, e := range out {
			if e == s {
				return
			}
		}
		out = append(out, s)
	}
	add(p)
	add(filepath.Base(p))
	return out
}

func sectionMentions(doc *parser.Document, sec *parser.Section, needles []string) bool {
	blob := strings.ToLower(sec.Title + "\n" + sec.Content)
	for _, n := range needles {
		if strings.Contains(blob, n) {
			return true
		}
	}
	for _, ref := range doc.References {
		if ref.Line < sec.LineStart || ref.Line > sec.LineEnd {
			continue
		}
		tgt := strings.ToLower(filepath.ToSlash(ref.Target))
		for _, n := range needles {
			if strings.Contains(tgt, n) {
				return true
			}
		}
	}
	return false
}

func outputExpand(docs []*parser.Document, name string) {
	found := false
	for _, doc := range docs {
		if doc.GetSection(name) == nil {
			continue
		}
		render.ExpandSection(doc, name)
		found = true
	}
	if !found {
		fmt.Printf("Section '%s' not found\n", name)
	}
}

func outputSection(docs []*parser.Document, name string) {
	found := false
	for _, doc := range docs {
		if doc.GetSection(name) == nil {
			continue
		}
		render.FilteredTree(doc, name)
		found = true
	}
	if !found {
		fmt.Printf("Section '%s' not found\n", name)
	}
}

func splitMentionArg(arg string) []string {
	var out []string
	for _, p := range strings.Split(arg, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func readMentionLines(r io.Reader) []string {
	var out []string
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

func stdinIsPipe() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice == 0
}

func parsePath(path string, all bool) ([]*parser.Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return parseDirectoryOpts(path, all), nil
	}
	doc, err := parseSingleFile(path)
	if err != nil {
		return nil, err
	}
	doc.Filename = filepath.Base(path)
	return []*parser.Document{doc}, nil
}

func parseSingleFile(path string) (*parser.Document, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".pdf") {
		return parser.ParsePDF(path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		return parser.ParseYAML(string(content))
	}
	return parser.Parse(string(content)), nil
}

func readTerms(search, filename string) ([]string, error) {
	var terms []string
	if search != "" {
		terms = append(terms, search)
	}
	if filename == "" {
		return terms, nil
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			terms = append(terms, line)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return nil, fmt.Errorf("terms file contains no queries")
	}
	return terms, nil
}

type searchJSON struct {
	Term    string `json:"term"`
	File    string `json:"file"`
	Section string `json:"section"`
	Tokens  int    `json:"tokens"`
}

func outputSearch(docs []*parser.Document, terms []string, jsonMode, compact bool) {
	if jsonMode {
		var out []searchJSON
		for _, term := range terms {
			for _, hit := range render.FindSearchResults(docs, term) {
				out = append(out, searchJSON{term, hit.Filename, hit.Section.Title, hit.Section.Tokens})
			}
		}
		if out == nil {
			out = []searchJSON{}
		}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	for _, term := range terms {
		hits := render.FindSearchResults(docs, term)
		if compact {
			if len(terms) > 1 {
				fmt.Printf("## %s\n", term)
			}
			for _, hit := range hits {
				file := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(hit.Filename)
				section := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(hit.Section.Title)
				fmt.Printf("%s > %s\n", file, section)
			}
		} else {
			render.SearchResults(docs, term)
		}
	}
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

func outputJSONSince(docs []*parser.Document, absRoot, root, ref string) {
	if err := parser.EnsureRef(root, ref); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	status := changeStatusByPath(root, ref)
	output := JSONOutput{
		Root:  absRoot,
		Since: ref,
	}
	seen := map[string]bool{}
	for _, doc := range docs {
		path := filepath.Join(root, doc.Filename)
		changed, err := parser.ChangedLines(path, ref)
		if err != nil || len(changed) == 0 {
			continue
		}
		lines := sortedChangedLines(changed)
		ch := "M"
		oldPath := ""
		if pc, ok := status[filepath.ToSlash(doc.Filename)]; ok {
			ch = pc.Status
			oldPath = pc.OldPath
		}
		jsonDoc := JSONDocument{
			Filename:     doc.Filename,
			Change:       ch,
			OldPath:      oldPath,
			ChangedLines: lines,
			Tokens:       doc.TotalTokens,
			Summary:      convertSummary(doc.Summary()),
			Sections:     convertSectionsSince(doc.Sections, changed),
			Nodes:        convertNodesSince(doc.Nodes, changed),
		}
		output.Documents = append(output.Documents, jsonDoc)
		output.TotalTokens += doc.TotalTokens
		seen[filepath.ToSlash(doc.Filename)] = true
	}
	for _, name := range deletedDocPaths(root, ref) {
		output.Documents = append(output.Documents, JSONDocument{
			Filename: name,
			Change:   "D",
		})
	}
	for _, c := range renamedDocPaths(root, ref) {
		if seen[c.Path] {
			continue
		}
		output.Documents = append(output.Documents, JSONDocument{
			Filename: c.Path,
			OldPath:  c.OldPath,
			Change:   c.Status,
		})
	}
	output.TotalDocs = len(output.Documents)
	if output.Documents == nil {
		output.Documents = []JSONDocument{}
	}
	json.NewEncoder(os.Stdout).Encode(output)
}

func sortedChangedLines(changed map[int]bool) []int {
	lines := make([]int, 0, len(changed))
	for n := range changed {
		lines = append(lines, n)
	}
	sort.Ints(lines)
	return lines
}

func convertSectionsSince(sections []*parser.Section, changed map[int]bool) []JSONSection {
	var result []JSONSection
	for _, s := range sections {
		kids := convertSectionsSince(s.Children, changed)
		hit := false
		for line := s.LineStart; line <= s.LineEnd; line++ {
			if changed[line] {
				hit = true
				break
			}
		}
		if !hit && len(kids) == 0 {
			continue
		}
		js := JSONSection{
			Level:     s.Level,
			Title:     s.Title,
			Tokens:    s.Tokens,
			LineStart: s.LineStart,
			LineEnd:   s.LineEnd,
			KeyTerms:  s.KeyTerms,
			Notables:  convertNodesSince(s.Notables, changed),
			Children:  kids,
		}
		result = append(result, js)
	}
	return result
}

func convertNodesSince(nodes []parser.Node, changed map[int]bool) []JSONNode {
	var out []JSONNode
	for _, n := range nodes {
		end := n.LineEnd()
		if end < n.LineStart() {
			end = n.LineStart()
		}
		hit := false
		for line := n.LineStart(); line <= end; line++ {
			if changed[line] {
				hit = true
				break
			}
		}
		if hit {
			out = append(out, convertNode(n))
		}
	}
	return out
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
  docmap <dirA> <dirB> (--search <query> | --terms-file <path>) [flags]
  docmap --stdin [flags] < manifest.json

Examples:
  docmap .                          # All markdown, PDF, and YAML files
  docmap README.md                  # Single markdown file deep dive
  docmap report.pdf                 # Single PDF file structure
  docmap config.yaml                # Single YAML file structure
  docmap docs/                      # Specific folder
  docmap README.md --section "API"  # Filter to section
  docmap . --brief                   # Ten-line session start: counts + recent docs
  docmap . --mentions parser/git.go  # Sections that mention a changed path
  docmap . --mentions --since HEAD   # Mentions of files git says changed
  docmap . --since HEAD --json       # Changed docs as JSON (includes deletions)
  docmap README.md --expand "API"   # Show section content
  docmap README.md --at 154 --expand "API"  # Dump the section that contains line 154
  docmap . --refs                   # Show cross-references between docs
  docmap docs/ --search "auth"     # Search across all files
  docmap dirA dirB --search "auth" --compact # Search multiple roots
  docmap --stdin --json < manifest.json  # Parse files from JSON manifest

Flags:
  --stdin                Read JSON file manifest from stdin (no filesystem access needed)
  --all                  Walk everything: include node_modules, vendor and
                         .gitignore'd files (skipped by default)
  --search <query>       Search sections across all files
  --terms-file <path>     Run one search query per non-comment, non-blank line
  --compact               Search output as one "file > section" line per hit
  -s, --section <name>   Filter to a specific section
  --brief                Session-start digest: file/section counts and recent docs
  --mentions <path>      Sections that mention a path (repeat or comma-list; stdin OK)
                         With --since and no path, uses git's changed files
  -e, --expand <name>    Show full source of a section (file:L-L, including children)
  -t, --type <kind>      Drill into one construct: code, callout, table, math,
                         footnote, deflist, linkref, html, task, wiki, embed,
                         mention, issue, sha, emoji
  --lang <name>          Sub-filter for --type code (e.g. --type code --lang python)
  --kind <name>          Sub-filter for --type callout (e.g. --kind warning)
  --at <line>            Show what construct lives at a specific line number
  --since <ref>          Show constructs on lines changed since a git ref
                         (runs git from the file's repo; works on a file or dir)
                         Combine with --json for changed_lines + deleted files
  -r, --refs             Show cross-references between markdown files
  -j, --json             Output JSON format
  -v, --version          Print version
  -h, --help             Show this help

Multiple positional paths are supported for search only; plain tree mode takes one path.

PDF Support:
  PDFs with outlines show document structure; tokens are estimated.
  PDFs without outlines fall back to page-by-page structure.

YAML Support:
  Maps keys to sections with nested children. Sequences use name/id/title
  fields for titles when available, falling back to key: value or [N].

More info: https://github.com/JordanCoin/docmap`)
}
