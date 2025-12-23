package main

import (
	"testing"
)

func TestParseDirectory(t *testing.T) {
	// Test with current directory (should find README.md at minimum)
	docs := parseDirectory(".")

	if len(docs) == 0 {
		t.Error("expected to find at least one markdown file")
	}

	// Check that README.md was found
	found := false
	for _, doc := range docs {
		if doc.Filename == "README.md" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find README.md")
	}
}
