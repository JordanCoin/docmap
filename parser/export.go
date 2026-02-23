package parser

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// yamlSection is the serializable form of a Section for YAML export.
type yamlSection struct {
	Title    string        `yaml:"title"`
	Content  string        `yaml:"content,omitempty"`
	Tokens   int           `yaml:"tokens"`
	Children []yamlSection `yaml:"children,omitempty"`
}

// yamlDocument is the serializable form of a Document for YAML export.
type yamlDocument struct {
	Docmap   string        `yaml:"docmap"`
	Filename string        `yaml:"filename,omitempty"`
	Tokens   int           `yaml:"tokens"`
	Sections []yamlSection `yaml:"sections"`
}

// ExportYAML serializes a Document to structured YAML.
// The output can be read back by ParseYAML to reconstruct the document.
func ExportYAML(doc *Document) (string, error) {
	yd := yamlDocument{
		Docmap:   "1.0",
		Filename: doc.Filename,
		Tokens:   doc.TotalTokens,
		Sections: convertToYAMLSections(doc.Sections),
	}

	data, err := yaml.Marshal(yd)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}

	return string(data), nil
}

func convertToYAMLSections(sections []*Section) []yamlSection {
	var result []yamlSection
	for _, s := range sections {
		ys := yamlSection{
			Title:    s.Title,
			Tokens:   s.Tokens,
			Children: convertToYAMLSections(s.Children),
		}
		if s.Content != "" {
			ys.Content = s.Content
		}
		result = append(result, ys)
	}
	return result
}
