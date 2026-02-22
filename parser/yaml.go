package parser

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseYAML parses YAML content into a Document structure
func ParseYAML(content string) (*Document, error) {
	doc := &Document{}

	if strings.TrimSpace(content) == "" {
		return doc, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// The root node is a DocumentNode wrapping the actual content
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return doc, nil
	}

	topNode := root.Content[0]
	doc.Sections = yamlNodeToSections(topNode, 1)

	// Calculate total tokens
	for _, s := range doc.GetAllSections() {
		doc.TotalTokens += s.Tokens
	}

	return doc, nil
}

// yamlNodeToSections converts a yaml.Node into a slice of Sections
func yamlNodeToSections(node *yaml.Node, level int) []*Section {
	switch node.Kind {
	case yaml.MappingNode:
		return yamlMappingToSections(node, level)
	case yaml.SequenceNode:
		return yamlSequenceToSections(node, level)
	case yaml.ScalarNode:
		// A top-level scalar is unusual but handle it
		section := &Section{
			Level:     level,
			Title:     truncateTitle(node.Value),
			Content:   node.Value,
			Tokens:    estimateTokens(node.Value),
			KeyTerms:  extractYAMLKeyTerms(node.Value),
			LineStart: node.Line,
			LineEnd:   node.Line,
		}
		return []*Section{section}
	case yaml.AliasNode:
		if node.Alias != nil {
			return yamlNodeToSections(node.Alias, level)
		}
		return nil
	default:
		return nil
	}
}

// yamlMappingToSections converts a YAML mapping node into sections (one per key)
func yamlMappingToSections(node *yaml.Node, level int) []*Section {
	var sections []*Section

	// MappingNode Content alternates: key, value, key, value, ...
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		section := &Section{
			Level:     level,
			Title:     keyNode.Value,
			LineStart: keyNode.Line,
			LineEnd:   lastLine(valNode),
		}

		switch valNode.Kind {
		case yaml.MappingNode, yaml.SequenceNode:
			children := yamlNodeToSections(valNode, level+1)
			for _, child := range children {
				child.Parent = section
			}
			section.Children = children
		case yaml.ScalarNode:
			section.Content = valNode.Value
			section.Tokens = estimateTokens(valNode.Value)
			section.KeyTerms = extractYAMLKeyTerms(valNode.Value)
		case yaml.AliasNode:
			if valNode.Alias != nil {
				children := yamlNodeToSections(valNode.Alias, level+1)
				for _, child := range children {
					child.Parent = section
				}
				section.Children = children
			}
		}

		// Calculate cumulative tokens for sections with children
		if len(section.Children) > 0 {
			calculateCumulativeTokens(section)
		}

		sections = append(sections, section)
	}

	return sections
}

// yamlSequenceToSections converts a YAML sequence node into sections (one per element)
func yamlSequenceToSections(node *yaml.Node, level int) []*Section {
	var sections []*Section

	for idx, item := range node.Content {
		var section *Section

		switch item.Kind {
		case yaml.MappingNode:
			title := yamlMapTitle(item, idx)
			section = &Section{
				Level:     level,
				Title:     title,
				LineStart: item.Line,
				LineEnd:   lastLine(item),
			}
			children := yamlMappingToSections(item, level+1)
			for _, child := range children {
				child.Parent = section
			}
			section.Children = children
			calculateCumulativeTokens(section)

		case yaml.SequenceNode:
			section = &Section{
				Level:     level,
				Title:     fmt.Sprintf("[%d]", idx),
				LineStart: item.Line,
				LineEnd:   lastLine(item),
			}
			children := yamlSequenceToSections(item, level+1)
			for _, child := range children {
				child.Parent = section
			}
			section.Children = children
			calculateCumulativeTokens(section)

		case yaml.ScalarNode:
			section = &Section{
				Level:     level,
				Title:     truncateTitle(item.Value),
				Content:   item.Value,
				Tokens:    estimateTokens(item.Value),
				KeyTerms:  extractYAMLKeyTerms(item.Value),
				LineStart: item.Line,
				LineEnd:   item.Line,
			}

		case yaml.AliasNode:
			if item.Alias != nil {
				aliased := yamlNodeToSections(item.Alias, level)
				sections = append(sections, aliased...)
				continue
			}
		}

		if section != nil {
			sections = append(sections, section)
		}
	}

	return sections
}

// yamlMapTitle generates a title for a map within a sequence.
// Prefers name/id/title keys; falls back to "firstKey: firstValue", then "[N]".
func yamlMapTitle(node *yaml.Node, index int) string {
	nameKeys := []string{"name", "id", "title"}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]
		for _, nk := range nameKeys {
			if strings.EqualFold(key, nk) && val.Kind == yaml.ScalarNode && val.Value != "" {
				return val.Value
			}
		}
	}

	// Fallback: use first key: value
	if len(node.Content) >= 2 {
		key := node.Content[0]
		val := node.Content[1]
		if key.Kind == yaml.ScalarNode && val.Kind == yaml.ScalarNode && val.Value != "" {
			title := fmt.Sprintf("%s: %s", key.Value, val.Value)
			return truncateTitle(title)
		}
	}

	return fmt.Sprintf("[%d]", index)
}

// lastLine finds the deepest line number in a yaml.Node subtree
func lastLine(node *yaml.Node) int {
	max := node.Line
	for _, child := range node.Content {
		if cl := lastLine(child); cl > max {
			max = cl
		}
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		if al := lastLine(node.Alias); al > max {
			max = al
		}
	}
	return max
}

// truncateTitle shortens a title if it's too long
func truncateTitle(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

// extractYAMLKeyTerms extracts terms containing special chars (package names, URLs, ports)
func extractYAMLKeyTerms(value string) []string {
	var terms []string
	seen := make(map[string]bool)

	for _, word := range strings.Fields(value) {
		word = strings.Trim(word, "\"',;()[]{}") // strip surrounding punctuation
		if word == "" || seen[word] {
			continue
		}
		// Look for terms with dots, slashes, colons, or @ (package names, URLs, ports, emails)
		if strings.ContainsAny(word, "./:@") && len(word) < 80 {
			terms = append(terms, word)
			seen[word] = true
		}
	}

	if len(terms) > 5 {
		terms = terms[:5]
	}
	return terms
}
