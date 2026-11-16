package docset

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/JordanCoin/docmap/parser"
	"github.com/charlievieth/fastwalk"
)

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
	".docmap":      true,
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
	out, err := exec.Command("git", "-C", abs, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".").Output()
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

// maxWalkTextSize is the largest .txt a directory walk parses. Bigger text
// files are logs and data dumps far more often than documents.
const maxWalkTextSize = 1 << 20

// nonDocText are .txt names that are machine input, not documents.
var nonDocText = map[string]bool{
	"cmakelists.txt": true, "robots.txt": true,
	// Python packaging metadata (*.egg-info)
	"top_level.txt": true, "sources.txt": true, "entry_points.txt": true,
	"dependency_links.txt": true, "requires.txt": true,
}

// walkableText decides whether a directory walk should parse the .txt file at
// path. Dependency pins, build manifests, oversized files and binary data
// wearing a .txt extension are left out. counted is true when the file was
// left out for a reason worth reporting (name or size), so callers can say
// how many were skipped instead of dropping them silently (#32).
func walkableText(path string, all bool) (ok, counted bool) {
	name := strings.ToLower(filepath.Base(path))
	if !all && (nonDocText[name] || strings.HasPrefix(name, "requirements") || strings.HasPrefix(name, "constraints")) {
		return false, true
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	if !all && info.Size() > maxWalkTextSize {
		return false, true
	}
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()
	head := make([]byte, 8192)
	n, _ := f.Read(head)
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return false, false
	}
	return true, false
}

// LoadDir walks dir for markdown, PDF, plain-text and YAML documents. Unless
// all is true it skips dependency directories and honors .gitignore.
// Collection uses charlievieth/fastwalk (parallel WalkDir); parsing uses a
// worker pool bounded by GOMAXPROCS. Results are sorted by Filename.
func LoadDir(dir string, all bool) []*parser.Document {
	docs, _ := loadDir(dir, all, true)
	return docs
}

// LoadDirSkipped is LoadDir plus the number of .txt files the walk left out
// by name or size (see walkableText).
func LoadDirSkipped(dir string, all bool) ([]*parser.Document, int) {
	return loadDir(dir, all, true)
}

// LoadDocs is LoadDir without YAML. YAML under a tree is config, fixtures and
// CI, not documentation: gumroad has 3,514 spec/support YAML files that would
// otherwise count as docs. Explicit YAML paths and --all still load it.
func LoadDocs(dir string, all bool) []*parser.Document {
	docs, _ := loadDir(dir, all, all)
	return docs
}

func loadDir(dir string, all, yaml bool) ([]*parser.Document, int) {
	var tracked map[string]bool
	if !all {
		tracked = gitTrackedSet(dir)
	}

	var (
		pathMu  sync.Mutex
		paths   []string
		skipped int
	)

	_ = fastwalk.Walk(nil, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if !all && path != dir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		lowerPath := strings.ToLower(path)
		isMd := strings.HasSuffix(lowerPath, ".md")
		isPdf := strings.HasSuffix(lowerPath, ".pdf")
		isYaml := strings.HasSuffix(lowerPath, ".yaml") || strings.HasSuffix(lowerPath, ".yml")
		isTxt := strings.HasSuffix(lowerPath, ".txt")
		if !isMd && !isPdf && !isTxt && !(yaml && isYaml) {
			return nil
		}

		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		if tracked != nil {
			rel, relErr := filepath.Rel(dir, path)
			if relErr == nil && !tracked[filepath.ToSlash(rel)] {
				return nil
			}
		}

		if isTxt {
			ok, counted := walkableText(path, all)
			if !ok {
				if counted {
					pathMu.Lock()
					skipped++
					pathMu.Unlock()
				}
				return nil
			}
		}

		pathMu.Lock()
		paths = append(paths, path)
		pathMu.Unlock()
		return nil
	})

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if n := len(paths); n < workers {
		workers = n
	}

	var (
		docsMu sync.Mutex
		docs   []*parser.Document
		wg     sync.WaitGroup
	)
	jobs := make(chan string)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				doc, err := parser.ParseFile(path)
				if err != nil {
					continue
				}

				relPath, _ := filepath.Rel(dir, path)
				doc.Filename = relPath

				docsMu.Lock()
				docs = append(docs, doc)
				docsMu.Unlock()
			}
		}()
	}

	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	wg.Wait()

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Filename < docs[j].Filename
	})
	return docs, skipped
}

// LoadPath loads a single file or directory of docs. For directories, Filename
// is relative to path; for a file it is the base name.
func LoadPath(path string, all bool) ([]*parser.Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return LoadDir(path, all), nil
	}
	doc, err := parser.ParseFile(path)
	if err != nil {
		return nil, err
	}
	doc.Filename = filepath.Base(path)
	return []*parser.Document{doc}, nil
}

// RecentByMtime returns up to limit filenames sorted by mtime descending.
func RecentByMtime(docs []*parser.Document, root string, limit int) []string {
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

// IsYAML reports whether name has a YAML extension.
func IsYAML(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

// WithoutYAML returns docs minus YAML files, preserving order.
func WithoutYAML(docs []*parser.Document) []*parser.Document {
	var out []*parser.Document
	for _, d := range docs {
		if !IsYAML(d.Filename) {
			out = append(out, d)
		}
	}
	return out
}

// KindBreakdown renders per-format counts for a doc set, e.g.
// "2 md, 1 pdf, 2 yaml". Formats with no files are omitted.
func KindBreakdown(docs []*parser.Document) string {
	order := []string{"md", "pdf", "yaml", "txt"}
	counts := map[string]int{}
	for _, d := range docs {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(d.Filename)), ".")
		switch ext {
		case "yml":
			ext = "yaml"
		case "md", "pdf", "yaml", "txt":
		default:
			ext = "md" // anything else was parsed as markdown
		}
		counts[ext]++
	}
	var parts []string
	for _, k := range order {
		if counts[k] > 0 {
			parts = append(parts, strconv.Itoa(counts[k])+" "+k)
		}
	}
	return strings.Join(parts, ", ")
}
