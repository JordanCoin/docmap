package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/JordanCoin/docmap/internal/docset"
	docmapmcp "github.com/JordanCoin/docmap/mcp"
	"github.com/JordanCoin/docmap/parser"
	"github.com/JordanCoin/docmap/render"
	"github.com/JordanCoin/docmap/stale"
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

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	if os.Args[1] == "mcp" {
		docmapmcp.Version = version
		if err := docmapmcp.Run(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
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
	var staleMode bool
	var staleDays int
	var checkFlags bool
	var staleRemote bool
	var allowDomains []string

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
		case "--stale":
			staleMode = true
		case "--days":
			if i+1 >= len(os.Args) || strings.HasPrefix(os.Args[i+1], "--") {
				fmt.Fprintln(os.Stderr, "Error: --days requires a number")
				os.Exit(1)
			}
			n, err := strconv.Atoi(os.Args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "Error: --days requires a positive integer")
				os.Exit(1)
			}
			staleDays = n
			i++
		case "--check-flags":
			checkFlags = true
		case "--remote":
			staleRemote = true
		case "--allow-domains":
			if i+1 >= len(os.Args) || strings.HasPrefix(os.Args[i+1], "--") {
				fmt.Fprintln(os.Stderr, "Error: --allow-domains requires a comma-separated list")
				os.Exit(1)
			}
			for _, d := range strings.Split(os.Args[i+1], ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					allowDomains = append(allowDomains, d)
				}
			}
			i++
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
	if staleDays != 0 && !staleMode && !brief {
		fmt.Fprintln(os.Stderr, "Error: --days requires --stale (or --brief)")
		os.Exit(1)
	}
	if (staleRemote || checkFlags || len(allowDomains) > 0) && !staleMode {
		fmt.Fprintln(os.Stderr, "Error: --remote/--check-flags/--allow-domains require --stale")
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
		docs := docset.LoadDir(tmpDir, false)
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
			parsed, err := docset.LoadPath(path, walkAll)
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
		docs := docset.LoadDir(target, walkAll)
		if len(docs) == 0 {
			fmt.Println("No markdown, PDF, or YAML files found")
			os.Exit(1)
		}
		if len(terms) > 0 {
			outputSearch(docs, terms, jsonMode, compact)
		} else if brief {
			outputBrief(docs, target, staleDays, staleMode)
		} else if staleMode {
			outputStale(docs, target, staleDays, checkFlags, staleRemote, allowDomains, jsonMode)
		} else if sinceRef != "" && jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSONSince(docs, absPath, target, sinceRef)
		} else if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON(docs, absPath)
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
		doc, err := parser.ParseFile(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing file: %v\n", err)
			os.Exit(1)
		}
		doc.Filename = filepath.Base(target)

		if len(terms) > 0 {
			outputSearch([]*parser.Document{doc}, terms, jsonMode, compact)
		} else if brief {
			outputBrief([]*parser.Document{doc}, filepath.Dir(target), staleDays, staleMode)
		} else if staleMode {
			outputStale([]*parser.Document{doc}, filepath.Dir(target), staleDays, checkFlags, staleRemote, allowDomains, jsonMode)
		} else if sinceRef != "" && jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSONSince([]*parser.Document{doc}, absPath, filepath.Dir(target), sinceRef)
		} else if jsonMode {
			absPath, _ := filepath.Abs(target)
			outputJSON([]*parser.Document{doc}, absPath)
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

func outputChangedSince(docs []*parser.Document, root, ref string) {
	if err := parser.EnsureRef(root, ref); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	paths := make([]string, len(docs))
	for i, doc := range docs {
		paths[i] = filepath.Join(root, doc.Filename)
	}
	batch, err := parser.ChangedLinesBatch(root, ref, paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	shown := 0
	seen := map[string]bool{}
	for _, doc := range docs {
		path := filepath.Join(root, doc.Filename)
		changed := batch[path]
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
	changes, err := parser.ChangedPathsAll(root, ref)
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

func outputBrief(docs []*parser.Document, root string, days int, runStale bool) {
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
		recent = docset.RecentByMtime(docs, root, 5)
	}
	if len(recent) > 0 {
		fmt.Printf("recent: %s\n", strings.Join(recent, ", "))
	}
	if !runStale {
		fmt.Println("stale: run with --brief --stale")
		return
	}
	findings := stale.Check(docs, stale.Options{Root: root, Days: days})
	if len(findings) == 0 {
		fmt.Println("stale: none")
	} else {
		fmt.Printf("stale: %d (run docmap %s --stale)\n", len(findings), root)
	}
}

func outputStale(docs []*parser.Document, root string, days int, checkFlags, remote bool, allow []string, jsonMode bool) {
	findings := stale.Check(docs, stale.Options{
		Root:         root,
		Days:         days,
		CheckFlags:   checkFlags,
		Remote:       remote,
		AllowDomains: allow,
	})
	if jsonMode {
		if findings == nil {
			findings = []stale.Finding{}
		}
		json.NewEncoder(os.Stdout).Encode(findings)
		return
	}
	for _, line := range stale.FormatLines(findings) {
		fmt.Println(line)
	}
	fmt.Printf("%d stale claim%s\n", len(findings), pluralClaims(len(findings)))
}

func pluralClaims(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func briefTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000.0)
	}
	return strconv.Itoa(n)
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
	json.NewEncoder(os.Stdout).Encode(docset.DocumentsJSON(root, docs))
}

func outputJSONSince(docs []*parser.Document, absRoot, root, ref string) {
	if err := parser.EnsureRef(root, ref); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	status := changeStatusByPath(root, ref)
	output := docset.JSONOutput{
		Root:  absRoot,
		Since: ref,
	}
	paths := make([]string, len(docs))
	for i, doc := range docs {
		paths[i] = filepath.Join(root, doc.Filename)
	}
	batch, err := parser.ChangedLinesBatch(root, ref, paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	seen := map[string]bool{}
	for _, doc := range docs {
		path := filepath.Join(root, doc.Filename)
		changed := batch[path]
		if len(changed) == 0 {
			continue
		}
		lines := sortedChangedLines(changed)
		ch := "M"
		oldPath := ""
		if pc, ok := status[filepath.ToSlash(doc.Filename)]; ok {
			ch = pc.Status
			oldPath = pc.OldPath
		}
		jsonDoc := docset.JSONDocument{
			Filename:     doc.Filename,
			Change:       ch,
			OldPath:      oldPath,
			ChangedLines: lines,
			Tokens:       doc.TotalTokens,
			Summary:      docset.ConvertSummary(doc.Summary()),
			Sections:     docset.ConvertSectionsSince(doc.Sections, changed),
			Nodes:        docset.ConvertNodesSince(doc.Nodes, changed),
		}
		output.Documents = append(output.Documents, jsonDoc)
		output.TotalTokens += doc.TotalTokens
		seen[filepath.ToSlash(doc.Filename)] = true
	}
	for _, name := range deletedDocPaths(root, ref) {
		output.Documents = append(output.Documents, docset.JSONDocument{
			Filename: name,
			Change:   "D",
		})
	}
	for _, c := range renamedDocPaths(root, ref) {
		if seen[c.Path] {
			continue
		}
		output.Documents = append(output.Documents, docset.JSONDocument{
			Filename: c.Path,
			OldPath:  c.OldPath,
			Change:   c.Status,
		})
	}
	output.TotalDocs = len(output.Documents)
	if output.Documents == nil {
		output.Documents = []docset.JSONDocument{}
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

func printUsage() {
	fmt.Println(`docmap - instant documentation structure for LLMs and humans

Usage:
  docmap <file.md|file.pdf|file.yaml|dir> [flags]
  docmap <dirA> <dirB> (--search <query> | --terms-file <path>) [flags]
  docmap --stdin [flags] < manifest.json
  docmap mcp                         # stdio MCP server

Examples:
  docmap .                          # All markdown, PDF, and YAML files
  docmap README.md                  # Single markdown file deep dive
  docmap report.pdf                 # Single PDF file structure
  docmap config.yaml                # Single YAML file structure
  docmap docs/                      # Specific folder
  docmap README.md --section "API"  # Filter to section
  docmap . --brief                   # Session start: counts + recent docs
  docmap . --brief --stale           # Brief plus stale claim count
  docmap . --stale                   # Flag stale path/binary/date/env claims
  docmap . --stale --remote          # Also HEAD-check URLs and compare config values
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
  --brief                Session-start digest: counts + recent docs (add --stale for count)
  --stale                Flag sections with missing paths/binaries/env keys or old dates
  --days N               With --stale/--brief --stale: status dates older than N days (default 90)
  --check-flags          With --stale: verify backticked --flags against binary --help
  --remote               With --stale: HEAD-check URLs, compare versions and config values
  --allow-domains list   With --stale --remote: only check these domains (comma-separated)
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
