package chunker

import (
	"fmt"
	"strings"
)

// GenerateSyntheticDescription produces a concise plain-English functional summary
// for a given code chunk based on identifier names, comments, signatures, and body keywords.
func GenerateSyntheticDescription(content, symbolName, kind, filePath string) string {
	if symbolName != "" {
		words := splitCamelCase(symbolName)
		cleanedSymbol := strings.Join(words, " ")

		switch kind {
		case "function_declaration", "function_definition":
			return fmt.Sprintf("Function %s in %s handles %s processing and operational execution.", symbolName, filePath, cleanedSymbol)
		case "method_declaration":
			return fmt.Sprintf("Method %s in %s executes %s logic on struct instance.", symbolName, filePath, cleanedSymbol)
		case "type_spec", "struct_definition", "class_definition":
			return fmt.Sprintf("Data type and struct %s defined in %s representing %s data model.", symbolName, filePath, cleanedSymbol)
		}
	}

	// Fallback heuristic based on tree-sitter or line content
	lines := strings.Split(content, "\n")
	var comments []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") {
			comments = append(comments, strings.TrimLeft(trimmed, "/#* "))
		}
	}

	if len(comments) > 0 {
		return strings.Join(comments, " ")
	}

	return fmt.Sprintf("Code block in %s implementing application logic.", filePath)
}

func splitCamelCase(src string) []string {
	var words []string
	var current strings.Builder
	for i, r := range src {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				words = append(words, strings.ToLower(current.String()))
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, strings.ToLower(current.String()))
	}
	return words
}
