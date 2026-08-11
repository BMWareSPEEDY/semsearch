package chunker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_csharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type Chunk struct {
	ID          int    `json:"id"`
	FilePath    string `json:"file_path"`
	SymbolName  string `json:"symbol_name,omitempty"`
	Kind        string `json:"kind"` // "function", "struct", "class", "method", "markdown_section", "text_block"
	LineStart   int    `json:"line_start"`
	LineEnd     int    `json:"line_end"`
	ContentHash string `json:"content_hash"`
	Content     string `json:"content"`
	Preview     string `json:"preview"`
}

type Chunker interface {
	ChunkFile(filePath string) ([]Chunk, error)
}

type TreeSitterChunker struct {
	TargetTokens int
	Overlap      int
}

func NewTreeSitterChunker() *TreeSitterChunker {
	return &TreeSitterChunker{
		TargetTokens: 400,
		Overlap:      50,
	}
}

func (c *TreeSitterChunker) ChunkFile(filePath string) ([]Chunk, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if len(content) == 0 {
		return nil, nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	lang := languageFromExtension(ext)

	if lang != nil {
		chunks, parseErr := c.chunkWithTreeSitter(filePath, string(content), lang)
		if parseErr == nil && len(chunks) > 0 {
			return chunks, nil
		}
	}

	return c.chunkSlidingWindow(filePath, string(content))
}

func languageFromExtension(ext string) *sitter.Language {
	switch ext {
	case ".go":
		return sitter.NewLanguage(tree_sitter_go.Language())
	case ".py":
		return sitter.NewLanguage(tree_sitter_python.Language())
	case ".js", ".jsx", ".ts", ".tsx":
		return sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	case ".java":
		return sitter.NewLanguage(tree_sitter_java.Language())
	case ".rs":
		return sitter.NewLanguage(tree_sitter_rust.Language())
	case ".cpp", ".cc", ".cxx", ".hpp", ".h":
		return sitter.NewLanguage(tree_sitter_cpp.Language())
	case ".c":
		return sitter.NewLanguage(tree_sitter_c.Language())
	case ".cs":
		return sitter.NewLanguage(tree_sitter_csharp.Language())
	case ".php":
		return sitter.NewLanguage(tree_sitter_php.LanguagePHP())
	case ".rb":
		return sitter.NewLanguage(tree_sitter_ruby.Language())
	default:
		return nil
	}
}

func (c *TreeSitterChunker) chunkWithTreeSitter(filePath, content string, lang *sitter.Language) ([]Chunk, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(lang); err != nil {
		return nil, err
	}

	tree := parser.Parse([]byte(content), nil)
	if tree == nil {
		return nil, nil
	}
	defer tree.Close()

	rootNode := tree.RootNode()
	if rootNode == nil {
		return nil, nil
	}

	var chunks []Chunk
	lines := strings.Split(content, "\n")

	c.walkAST(filePath, rootNode, lines, &chunks)

	if len(chunks) == 0 {
		return c.chunkSlidingWindow(filePath, content)
	}

	return chunks, nil
}

// symbolKindForNode maps a tree-sitter node type to a chunk kind.
// Keys cover the 11 Tier-1 grammars (Go, Python, JS/TS, Java, Rust, C/C++,
// C#, PHP, Ruby) plus a few generic aliases. "type_spec" is handled
// separately in walkAST because its kind depends on its type child.
func symbolKindForNode(nodeType string) string {
	switch nodeType {
	case "function_declaration", "function_definition", "method_declaration",
		"method_definition", "function_item", "function_signature",
		"func_declaration", "method":
		return "function"
	case "struct_declaration", "struct_specifier", "struct_type",
		"struct_item", "type_definition", "class_struct",
		"record_declaration", "class_declaration":
		return "struct"
	case "class_definition", "class_specifier", "class_item",
		"class", "module":
		return "class"
	case "interface_declaration", "interface_type", "interface_item":
		return "interface"
	case "enum_declaration", "enum_specifier", "enum_item",
		"enum_definition":
		return "enum"
	case "trait_declaration", "trait_definition", "trait_item":
		return "trait"
	case "impl_declaration", "impl_block", "impl_item":
		return "impl"
	default:
		return ""
	}
}

func (c *TreeSitterChunker) walkAST(filePath string, node *sitter.Node, lines []string, chunks *[]Chunk) {
	nodeType := node.Kind()
	kind := symbolKindForNode(nodeType)

	// Special handling for Go's type_spec - its kind depends on the type child
	if kind == "" && nodeType == "type_spec" {
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child != nil && child.Kind() == "struct_type" {
				kind = "struct"
				break
			}
		}
	}

	if kind != "" {
		startPoint := node.StartPosition()
		endPoint := node.EndPosition()
		startLine := int(startPoint.Row) + 1
		endLine := int(endPoint.Row) + 1

		if startLine <= len(lines) && endLine <= len(lines) {
			if endLine > len(lines) {
				endLine = len(lines)
			}
			if startLine < 1 {
				startLine = 1
			}

			content := strings.Join(lines[startLine-1:endLine], "\n")
			if strings.TrimSpace(content) != "" {
				chunk := createChunk(filePath, c.extractSymbolName(node, lines), kind, startLine, endLine, content)
				*chunks = append(*chunks, chunk)
			}
		}
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil {
			c.walkAST(filePath, child, lines, chunks)
		}
	}
}

func (c *TreeSitterChunker) extractSymbolName(node *sitter.Node, lines []string) string {
	// Prefer the grammar-standard "name" field when present.
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		if s := nodeText(nameNode, lines); s != "" {
			return s
		}
	}

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Kind()
		if childType == "identifier" || childType == "type_identifier" ||
			childType == "name" || childType == "property_identifier" ||
			childType == "field_identifier" || childType == "method_name" ||
			childType == "function_name" {
			if s := nodeText(child, lines); s != "" {
				return s
			}
		}
	}
	return ""
}

func nodeText(node *sitter.Node, lines []string) string {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()
	start := int(startPoint.Row)
	startCol := int(startPoint.Column)
	endCol := int(endPoint.Column)

	if start < 0 || start >= len(lines) {
		return ""
	}
	line := lines[start]
	if endCol > len(line) {
		endCol = len(line)
	}
	if startCol < 0 || startCol >= len(line) || endCol <= startCol {
		return ""
	}
	return line[startCol:endCol]
}

func (c *TreeSitterChunker) chunkSlidingWindow(filePath, content string) ([]Chunk, error) {
	lines := strings.Split(content, "\n")

	var chunks []Chunk
	windowLines := 30
	stepSize := 20

	for i := 0; i < len(lines); i += stepSize {
		end := i + windowLines
		if end > len(lines) {
			end = len(lines)
		}

		slice := lines[i:end]
		windowContent := strings.Join(slice, "\n")
		if strings.TrimSpace(windowContent) == "" {
			continue
		}

		chunk := createChunk(filePath, "", "text_block", i+1, end, windowContent)
		chunks = append(chunks, chunk)

		if end == len(lines) {
			break
		}
	}

	return chunks, nil
}

func createChunk(filePath, symbol, kind string, lineStart, lineEnd int, content string) Chunk {
	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:8])

	preview := content
	if len(preview) > 150 {
		preview = preview[:147] + "..."
	}

	return Chunk{
		FilePath:    filePath,
		SymbolName:  symbol,
		Kind:        kind,
		LineStart:   lineStart,
		LineEnd:     lineEnd,
		ContentHash: hashStr,
		Content:     content,
		Preview:     preview,
	}
}