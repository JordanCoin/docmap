package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordanCoin/docmap/internal/docset"
	"github.com/JordanCoin/docmap/parser"
)

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
		docs := docset.LoadDir(abs, false)
		if len(docs) == 0 {
			return nil, fmt.Errorf("no markdown, PDF, or YAML files found in %s", abs)
		}
		return &loaded{Root: abs, Docs: docs}, nil
	}
	docs, err := docset.LoadPath(abs, false)
	if err != nil {
		return nil, err
	}
	return &loaded{Root: filepath.Dir(abs), Docs: docs}, nil
}
