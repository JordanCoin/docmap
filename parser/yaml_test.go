package parser

import (
	"testing"
)

func TestParseYAMLSimpleMapping(t *testing.T) {
	content := `name: myapp
version: 1.0.0
description: A sample application`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(doc.Sections))
	}

	if doc.Sections[0].Title != "name" {
		t.Errorf("expected title 'name', got '%s'", doc.Sections[0].Title)
	}
	if doc.Sections[0].Content != "myapp" {
		t.Errorf("expected content 'myapp', got '%s'", doc.Sections[0].Content)
	}
	if doc.Sections[1].Title != "version" {
		t.Errorf("expected title 'version', got '%s'", doc.Sections[1].Title)
	}
}

func TestParseYAMLNestedMapping(t *testing.T) {
	content := `server:
  host: localhost
  port: 8080
  ssl:
    enabled: true
    cert: /path/to/cert.pem`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 top-level section, got %d", len(doc.Sections))
	}

	server := doc.Sections[0]
	if server.Title != "server" {
		t.Errorf("expected title 'server', got '%s'", server.Title)
	}
	if server.Level != 1 {
		t.Errorf("expected level 1, got %d", server.Level)
	}

	if len(server.Children) != 3 {
		t.Fatalf("expected 3 children (host, port, ssl), got %d", len(server.Children))
	}

	// Check nested ssl
	ssl := server.Children[2]
	if ssl.Title != "ssl" {
		t.Errorf("expected title 'ssl', got '%s'", ssl.Title)
	}
	if len(ssl.Children) != 2 {
		t.Errorf("expected 2 children under ssl, got %d", len(ssl.Children))
	}
	if ssl.Level != 2 {
		t.Errorf("expected ssl level 2, got %d", ssl.Level)
	}
	if ssl.Children[0].Level != 3 {
		t.Errorf("expected ssl child level 3, got %d", ssl.Children[0].Level)
	}
}

func TestParseYAMLSimpleSequence(t *testing.T) {
	content := `items:
  - apple
  - banana
  - cherry`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(doc.Sections))
	}

	items := doc.Sections[0]
	if len(items.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(items.Children))
	}

	if items.Children[0].Title != "apple" {
		t.Errorf("expected 'apple', got '%s'", items.Children[0].Title)
	}
	if items.Children[1].Title != "banana" {
		t.Errorf("expected 'banana', got '%s'", items.Children[1].Title)
	}
}

func TestParseYAMLSequenceOfMapsWithName(t *testing.T) {
	content := `services:
  - name: web
    port: 80
    image: nginx:latest
  - name: api
    port: 3000
    image: node:18
  - name: db
    port: 5432
    image: postgres:15`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	services := doc.Sections[0]
	if len(services.Children) != 3 {
		t.Fatalf("expected 3 service children, got %d", len(services.Children))
	}

	// Should use 'name' field as title
	if services.Children[0].Title != "web" {
		t.Errorf("expected 'web', got '%s'", services.Children[0].Title)
	}
	if services.Children[1].Title != "api" {
		t.Errorf("expected 'api', got '%s'", services.Children[1].Title)
	}
	if services.Children[2].Title != "db" {
		t.Errorf("expected 'db', got '%s'", services.Children[2].Title)
	}
}

func TestParseYAMLSequenceOfMapsFallback(t *testing.T) {
	content := `steps:
  - run: echo hello
    shell: bash
  - run: make test
    shell: bash`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := doc.Sections[0]
	if len(steps.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(steps.Children))
	}

	// Should fall back to "firstKey: firstValue"
	if steps.Children[0].Title != "run: echo hello" {
		t.Errorf("expected 'run: echo hello', got '%s'", steps.Children[0].Title)
	}
}

func TestParseYAMLDeeplyNested(t *testing.T) {
	content := `a:
  b:
    c:
      d:
        e:
          value: deep`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Walk down 5 levels
	current := doc.Sections[0] // a
	if current.Title != "a" {
		t.Errorf("level 1: expected 'a', got '%s'", current.Title)
	}
	current = current.Children[0] // b
	if current.Title != "b" {
		t.Errorf("level 2: expected 'b', got '%s'", current.Title)
	}
	current = current.Children[0] // c
	current = current.Children[0] // d
	current = current.Children[0] // e
	if current.Title != "e" {
		t.Errorf("level 5: expected 'e', got '%s'", current.Title)
	}
	if current.Level != 5 {
		t.Errorf("expected level 5, got %d", current.Level)
	}
}

func TestParseYAMLEmptyFile(t *testing.T) {
	doc, err := ParseYAML("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(doc.Sections))
	}

	doc, err = ParseYAML("   \n\n  ")
	if err != nil {
		t.Fatalf("unexpected error for whitespace: %v", err)
	}
	if len(doc.Sections) != 0 {
		t.Errorf("expected 0 sections for whitespace, got %d", len(doc.Sections))
	}
}

func TestParseYAMLInvalid(t *testing.T) {
	_, err := ParseYAML(":\n  - :\n  invalid: [yaml: {bad")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParseYAMLMixedTypes(t *testing.T) {
	content := `string_val: hello
list_val:
  - one
  - two
map_val:
  key1: val1
  key2: val2`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(doc.Sections))
	}

	// string_val: scalar content, no children
	if doc.Sections[0].Content != "hello" {
		t.Errorf("expected content 'hello', got '%s'", doc.Sections[0].Content)
	}
	if len(doc.Sections[0].Children) != 0 {
		t.Errorf("expected 0 children for scalar, got %d", len(doc.Sections[0].Children))
	}

	// list_val: has children
	if len(doc.Sections[1].Children) != 2 {
		t.Errorf("expected 2 children for list, got %d", len(doc.Sections[1].Children))
	}

	// map_val: has children
	if len(doc.Sections[2].Children) != 2 {
		t.Errorf("expected 2 children for map, got %d", len(doc.Sections[2].Children))
	}
}

func TestParseYAMLGetSection(t *testing.T) {
	content := `database:
  host: localhost
  port: 5432
  credentials:
    username: admin
    password: secret
cache:
  redis:
    host: localhost
    port: 6379`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find top-level section
	section := doc.GetSection("database")
	if section == nil {
		t.Fatal("expected to find 'database' section")
	}
	if section.Title != "database" {
		t.Errorf("expected title 'database', got '%s'", section.Title)
	}

	// Find nested section
	section = doc.GetSection("credentials")
	if section == nil {
		t.Fatal("expected to find 'credentials' section")
	}

	// Find deeply nested
	section = doc.GetSection("redis")
	if section == nil {
		t.Fatal("expected to find 'redis' section")
	}

	// Not found
	section = doc.GetSection("nonexistent")
	if section != nil {
		t.Error("expected nil for nonexistent section")
	}
}

func TestParseYAMLLineNumbers(t *testing.T) {
	content := `name: test
server:
  host: localhost
  port: 8080`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc.Sections[0].LineStart != 1 {
		t.Errorf("expected line 1, got %d", doc.Sections[0].LineStart)
	}

	server := doc.Sections[1]
	if server.LineStart != 2 {
		t.Errorf("expected server at line 2, got %d", server.LineStart)
	}
	if server.LineEnd < 4 {
		t.Errorf("expected server LineEnd >= 4, got %d", server.LineEnd)
	}
}

func TestParseYAMLBooleanNumericScalars(t *testing.T) {
	content := `enabled: true
count: 42
ratio: 3.14
disabled: false`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Sections) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(doc.Sections))
	}

	if doc.Sections[0].Content != "true" {
		t.Errorf("expected content 'true', got '%s'", doc.Sections[0].Content)
	}
	if doc.Sections[1].Content != "42" {
		t.Errorf("expected content '42', got '%s'", doc.Sections[1].Content)
	}
	if doc.Sections[2].Content != "3.14" {
		t.Errorf("expected content '3.14', got '%s'", doc.Sections[2].Content)
	}
}

func TestParseYAMLCumulativeTokens(t *testing.T) {
	content := `parent:
  child1: some content here
  child2: more content here too`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parent := doc.Sections[0]
	child1Tokens := parent.Children[0].Tokens
	child2Tokens := parent.Children[1].Tokens

	if child1Tokens == 0 || child2Tokens == 0 {
		t.Error("expected non-zero tokens for children")
	}

	// Parent cumulative tokens should include children
	if parent.Tokens < child1Tokens+child2Tokens {
		t.Errorf("parent tokens (%d) should be >= sum of children (%d + %d)",
			parent.Tokens, child1Tokens, child2Tokens)
	}
}

func TestParseYAMLTotalTokens(t *testing.T) {
	content := `key1: value one
key2: value two
key3: value three`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc.TotalTokens == 0 {
		t.Error("expected non-zero total tokens")
	}
}

func TestParseYAMLNullValues(t *testing.T) {
	content := `present: value
empty:
also_null: ~`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(doc.Sections))
	}
}

func TestParseYAMLFlowStyle(t *testing.T) {
	content := `flow_map: {a: 1, b: 2}
flow_list: [x, y, z]`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(doc.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(doc.Sections))
	}

	// flow_map should have children
	if len(doc.Sections[0].Children) != 2 {
		t.Errorf("expected 2 children for flow_map, got %d", len(doc.Sections[0].Children))
	}

	// flow_list should have children
	if len(doc.Sections[1].Children) != 3 {
		t.Errorf("expected 3 children for flow_list, got %d", len(doc.Sections[1].Children))
	}
}

func TestExtractYAMLKeyTerms(t *testing.T) {
	terms := extractYAMLKeyTerms("nginx:latest https://example.com user@host.com /path/to/file")

	if len(terms) == 0 {
		t.Fatal("expected key terms to be extracted")
	}

	found := make(map[string]bool)
	for _, term := range terms {
		found[term] = true
	}

	if !found["nginx:latest"] {
		t.Error("expected 'nginx:latest' in key terms")
	}
	if !found["https://example.com"] {
		t.Error("expected 'https://example.com' in key terms")
	}
}

func TestParseYAMLParentPointers(t *testing.T) {
	content := `root:
  child:
    grandchild: value`

	doc, err := ParseYAML(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root := doc.Sections[0]
	child := root.Children[0]
	grandchild := child.Children[0]

	if child.Parent != root {
		t.Error("child's parent should be root")
	}
	if grandchild.Parent != child {
		t.Error("grandchild's parent should be child")
	}
}
