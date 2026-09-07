package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JordanCoin/docmap/parser"
)

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

type loaded struct {
	Root string
	Docs []*parser.Document
}

func loadPath(path string) (*loaded, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, path[2:])
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		docs := parseDirectory(abs)
		if len(docs) == 0 {
			return nil, fmt.Errorf("no markdown, PDF, or YAML files found in %s", abs)
		}
		return &loaded{Root: abs, Docs: docs}, nil
	}
	doc, err := parser.ParseFile(abs)
	if err != nil {
		return nil, err
	}
	doc.Filename = filepath.Base(abs)
	return &loaded{Root: filepath.Dir(abs), Docs: []*parser.Document{doc}}, nil
}

func parseDirectory(dir string) []*parser.Document {
	tracked := gitTrackedSet(dir)
	var docs []*parser.Document
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path != dir && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(path)
		isMd := strings.HasSuffix(lower, ".md")
		isPdf := strings.HasSuffix(lower, ".pdf")
		isYaml := strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
		if !isMd && !isPdf && !isYaml {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		if tracked != nil {
			rel, relErr := filepath.Rel(dir, path)
			if relErr == nil && !tracked[filepath.ToSlash(rel)] {
				return nil
			}
		}
		doc, err := parser.ParseFile(path)
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		doc.Filename = relPath
		docs = append(docs, doc)
		return nil
	})
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Filename < docs[j].Filename
	})
	return docs
}

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
