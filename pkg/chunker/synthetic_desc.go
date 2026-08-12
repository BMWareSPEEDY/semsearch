package chunker

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GenerateSyntheticDescription produces a concise plain-English functional summary
// for a given code chunk. The chunk carries a structural meta header followed by
// the definition; thanks to doc-comment lookback in the AST chunker, the leading
// lines of the body are the developer's own doc comment for the symbol. That is
// the richest semantic signal for conceptual queries, so it is preferred over
// templates. The repo-relative module path is always included (at the front, so
// it survives token truncation) because module names — hugolib/site_render.go,
// minifiers/minifiers.go — frequently carry the exact terms users search for.
func GenerateSyntheticDescription(content, symbolName, kind, filePath string) string {
	pathHint := modulePathHint(filePath)

	if comment := leadingDocComment(content); comment != "" {
		return pathHint + " — " + comment
	}

	if symbolName != "" {
		words := splitCamelCase(symbolName)
		cleaned := strings.Join(words, " ")
		if cleaned == "" {
			cleaned = symbolName
		}

		switch kind {
		case "function_declaration", "function_definition":
			return fmt.Sprintf("Function %s in %s handles %s processing and operational execution.", symbolName, pathHint, cleaned)
		case "method_declaration":
			return fmt.Sprintf("Method %s in %s executes %s logic on struct instance.", symbolName, pathHint, cleaned)
		case "type_spec", "struct_definition", "class_definition":
			return fmt.Sprintf("Data type and struct %s defined in %s representing %s data model.", symbolName, pathHint, cleaned)
		}
	}

	return fmt.Sprintf("Code block in %s implementing application logic.", pathHint)
}

// modulePathHint returns a repo-agnostic module-style path (last 3 segments)
// such as "hugolib/site_render.go" or "clap_builder/src/builder/arg.rs", which
// preserves the module and sub-module names used in search queries.
func modulePathHint(filePath string) string {
	parts := strings.Split(filepath.ToSlash(filePath), "/")
	n := len(parts)
	if n <= 3 {
		return filepath.ToSlash(filePath)
	}
	return strings.Join(parts[n-3:], "/")
}

// ChunkCodeText returns the chunk content used for code-vector embedding: the
// structural meta header plus the definition body, with any leading doc comment
// removed. Doc comments are embedded separately as the description vector, so
// they never crowd the code signature out of the model's context window — the
// case for Rust crates whose `///` docs run to dozens of lines. Markdown chunks
// (no meta header) are returned unchanged.
func ChunkCodeText(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "// [") {
		return content
	}
	i := 1
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			i++
			continue
		}
		if isDocCommentLine(t) {
			i++
			continue
		}
		break
	}
	return joinLines(lines[:1]) + "\n" + joinLines(lines[i:])
}

// leadingDocComment extracts the run of comment lines that open the chunk body,
// immediately after the meta header, and returns them joined as plain English.
// It returns "" when the body starts with code (e.g. markdown chunks).
func leadingDocComment(content string) string {
	lines := strings.Split(content, "\n")
	// Skip the structural meta header line.
	start := 0
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "// [Package:") || t == "" {
			continue
		}
		start = i
		break
	}

	var comments []string
	for i := start; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			if len(comments) > 0 {
				// Blank line ends the doc block.
				break
			}
			continue
		}
		if isDocCommentLine(t) {
			comments = append(comments, stripCommentMarker(t))
			continue
		}
		break
	}

	joined := strings.Join(comments, " ")
	if joined == "" {
		return ""
	}
	return truncateWords(joined, 60)
}

func stripCommentMarker(l string) string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "///")
	l = strings.TrimPrefix(l, "//")
	l = strings.TrimPrefix(l, "/*")
	l = strings.TrimPrefix(l, "*/")
	l = strings.TrimPrefix(l, "#")
	l = strings.TrimSpace(l)
	l = strings.Trim(l, "*")
	return strings.TrimSpace(l)
}

func truncateWords(s string, max int) string {
	words := strings.Fields(s)
	if len(words) <= max {
		return s
	}
	return strings.Join(words[:max], " ")
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
