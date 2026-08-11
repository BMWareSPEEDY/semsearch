package chunker

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
)

type ASTChunker struct {
	defaultChunker *DefaultChunker
}

func NewASTChunker() *ASTChunker {
	return &ASTChunker{
		defaultChunker: NewDefaultChunker(),
	}
}

func (a *ASTChunker) ChunkFile(filePath string, content []byte) ([]Chunk, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	var lang *sitter.Language
	var targetKinds map[string]bool

	switch ext {
	case ".go":
		lang = golang.GetLanguage()
		targetKinds = map[string]bool{
			"function_declaration": true,
			"method_declaration":   true,
			"type_spec":            true,
		}
	case ".py", ".pyi":
		lang = python.GetLanguage()
		targetKinds = map[string]bool{
			"function_definition":       true,
			"async_function_definition": true,
			"class_definition":          true,
		}
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		lang = javascript.GetLanguage()
		targetKinds = map[string]bool{
			"function_declaration": true,
			"method_definition":   true,
			"class_declaration":    true,
			"arrow_function":       true,
		}
	default:
		return a.defaultChunker.ChunkFile(filePath)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil || tree == nil {
		return a.defaultChunker.ChunkFile(filePath)
	}
	defer tree.Close()

	n := tree.RootNode()
	var chunks []Chunk
	lines := splitLines(string(content))

	var inspectNode func(node *sitter.Node)
	inspectNode = func(node *sitter.Node) {
		kind := node.Type()
		if targetKinds[kind] {
			startRow := int(node.StartPoint().Row)
			endRow := int(node.EndPoint().Row)

			if endRow-startRow > 80 {
				endRow = startRow + 80
			}

			if startRow < len(lines) && endRow < len(lines) {
				slice := lines[startRow : endRow+1]
				rawChunkContent := joinLines(slice)

				symbolName := extractSymbolName(node, content)
				pkgName := extractPackageName(lines)

				// Structural Metadata Prefixing: Give the neural embedding model English-language domain context
				metaPrefix := fmt.Sprintf("// [Package: %s | File: %s | Symbol: %s | Kind: %s]\n", pkgName, filepath.Base(filePath), symbolName, kind)
				chunkContent := metaPrefix + rawChunkContent

				chunk := createChunk(filePath, symbolName, kind, startRow+1, endRow+1, chunkContent)
				chunks = append(chunks, chunk)
			}
		}

		for i := 0; i < int(node.NamedChildCount()); i++ {
			inspectNode(node.NamedChild(i))
		}
	}

	inspectNode(n)

	if len(chunks) == 0 {
		return a.defaultChunker.ChunkFile(filePath)
	}

	return chunks, nil
}

func extractPackageName(lines []string) string {
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "package ") {
			parts := strings.Fields(l)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return "main"
}

func extractSymbolName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" || child.Type() == "type_identifier" {
			return child.Content(src)
		}
	}
	return ""
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	res := ""
	for i, l := range lines {
		if i > 0 {
			res += "\n"
		}
		res += l
	}
	return res
}
