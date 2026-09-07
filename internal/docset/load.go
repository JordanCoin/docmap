package docset

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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

// LoadDir walks dir for markdown, PDF and YAML documents. Unless all is true
// it skips dependency directories and honors .gitignore.
// Collection uses charlievieth/fastwalk (parallel WalkDir); parsing uses a
// worker pool bounded by GOMAXPROCS. Results are sorted by Filename.
func LoadDir(dir string, all bool) []*parser.Document {
	var tracked map[string]bool
	if !all {
		tracked = gitTrackedSet(dir)
	}

	var (
		pathMu sync.Mutex
		paths  []string
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
		if !isMd && !isPdf && !isYaml {
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
	return docs
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
