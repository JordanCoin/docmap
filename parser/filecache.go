package parser

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
)

const cacheVersion = 1

// cachePayload is the on-disk gob form under .docmap/cache.
// Keyed by absolute path; validated against mtime (unix nano) + size.
// Markdown entries store Nodes (acyclic); Sections/References are rebuilt
// on load. YAML entries store Sections (Parents cleared) when Nodes is empty.
type cachePayload struct {
	Version     int
	AbsPath     string
	MtimeNS     int64
	Size        int64
	Source      string
	TotalTokens int
	Nodes       []Node
	Sections    []*Section
	References  []Reference
}

var gobOnce sync.Once

func registerGobTypes() {
	gobOnce.Do(func() {
		gob.Register(&Frontmatter{})
		gob.Register(&Heading{})
		gob.Register(&Paragraph{})
		gob.Register(&Blockquote{})
		gob.Register(&Callout{})
		gob.Register(&List{})
		gob.Register(&ListItem{})
		gob.Register(&TaskItem{})
		gob.Register(&Table{})
		gob.Register(&TableRow{})
		gob.Register(&TableCell{})
		gob.Register(&CodeBlock{})
		gob.Register(&MathBlock{})
		gob.Register(&ThematicBreak{})
		gob.Register(&HTMLBlock{})
		gob.Register(&DefinitionList{})
		gob.Register(&DefinitionTerm{})
		gob.Register(&Definition{})
		gob.Register(&FootnoteDef{})
		gob.Register(&LinkRefDef{})
		gob.Register(&Text{})
		gob.Register(&Emphasis{})
		gob.Register(&Strong{})
		gob.Register(&Delete{})
		gob.Register(&InlineCode{})
		gob.Register(&Link{})
		gob.Register(&AutoLink{})
		gob.Register(&Image{})
		gob.Register(&WikiLink{})
		gob.Register(&WikiEmbed{})
		gob.Register(&FootnoteRef{})
		gob.Register(&Mention{})
		gob.Register(&IssueRef{})
		gob.Register(&CommitRef{})
		gob.Register(&Emoji{})
		gob.Register(&LineBreak{})
		gob.Register(&Entity{})
		gob.Register(&InlineMath{})
		gob.Register(&InlineHTML{})
	})
}

func cacheEnabled() bool {
	return os.Getenv("DOCMAP_NO_CACHE") != "1"
}

// cacheDirForTest overrides .docmap/cache location in tests. Empty = default.
var cacheDirForTest string

// cacheDirFor returns the on-disk cache directory for absFile.
// Prefer <git-root>/.docmap/cache (walk for .git), else beside the file.
// Not process cwd — scanning repo B from repo A must not write into A.
func cacheDirFor(absFile string) string {
	if cacheDirForTest != "" {
		return cacheDirForTest
	}
	dir := filepath.Dir(absFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Join(dir, ".docmap", "cache")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join(filepath.Dir(absFile), ".docmap", "cache")
}

func cacheFileName(absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(sum[:16]) + ".gob"
}

// cacheGet returns a Document when a valid mtime/size entry exists.
// Errors are ignored (cache miss).
func cacheGet(absPath string, mtimeNS, size int64) (*Document, bool) {
	if !cacheEnabled() {
		return nil, false
	}
	dir := cacheDirFor(absPath)
	if dir == "" {
		return nil, false
	}
	registerGobTypes()

	f, err := os.Open(filepath.Join(dir, cacheFileName(absPath)))
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var p cachePayload
	if err := gob.NewDecoder(f).Decode(&p); err != nil {
		return nil, false
	}
	if p.Version != cacheVersion || p.AbsPath != absPath || p.MtimeNS != mtimeNS || p.Size != size {
		return nil, false
	}

	doc := &Document{
		Source:      p.Source,
		TotalTokens: p.TotalTokens,
		Nodes:       p.Nodes,
		References:  p.References,
	}
	if len(p.Nodes) > 0 {
		doc.Sections, doc.TotalTokens = sectionsFromNodes(p.Nodes)
		doc.References = referencesFromNodes(p.Nodes)
	} else {
		doc.Sections = p.Sections
		rewireSectionParents(doc.Sections)
	}
	return doc, true
}

// cachePut stores a Document. Errors are ignored.
func cachePut(absPath string, mtimeNS, size int64, doc *Document) {
	if !cacheEnabled() || doc == nil {
		return
	}
	dir := cacheDirFor(absPath)
	if dir == "" {
		return
	}
	registerGobTypes()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	p := cachePayload{
		Version:     cacheVersion,
		AbsPath:     absPath,
		MtimeNS:     mtimeNS,
		Size:        size,
		Source:      doc.Source,
		TotalTokens: doc.TotalTokens,
		Nodes:       doc.Nodes,
		References:  doc.References,
	}
	if len(doc.Nodes) == 0 {
		p.Sections = cloneSectionsForCache(doc.Sections)
	}

	tmp, err := os.CreateTemp(dir, "tmp-*.gob")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	enc := gob.NewEncoder(tmp)
	err = enc.Encode(&p)
	closeErr := tmp.Close()
	if err != nil || closeErr != nil {
		os.Remove(tmpName)
		return
	}
	final := filepath.Join(dir, cacheFileName(absPath))
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
	}
}

// cloneSectionsForCache deep-copies the section tree with Parent nilled
// so gob encoding stays acyclic. Notables are omitted (YAML has none).
func cloneSectionsForCache(sections []*Section) []*Section {
	if sections == nil {
		return nil
	}
	out := make([]*Section, len(sections))
	for i, s := range sections {
		cp := &Section{
			Level:     s.Level,
			Title:     s.Title,
			Content:   s.Content,
			Tokens:    s.Tokens,
			KeyTerms:  append([]string(nil), s.KeyTerms...),
			LineStart: s.LineStart,
			LineEnd:   s.LineEnd,
			Stats:     s.Stats,
			Children:  cloneSectionsForCache(s.Children),
		}
		out[i] = cp
	}
	return out
}

func rewireSectionParents(sections []*Section) {
	for _, s := range sections {
		for _, c := range s.Children {
			c.Parent = s
		}
		rewireSectionParents(s.Children)
	}
}
