package chunker

import (
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_csharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// ASTChunker produces one chunk per top-level (and nested) definition using
// tree-sitter. It supports Go, Python, JS/TS, Java, Rust, C/C++, C#, PHP and
// Ruby. Each chunk is prefixed with a structural metadata header giving the
// embedding model English/structural domain context (package, file, symbol,
// kind), which measurably improves conceptual recall.
type ASTChunker struct {
	defaultChunker     *DefaultChunker
	maxChunkLines      int // hard cap; larger definitions are split into windows
	windowLines        int // sliding window size used when splitting huge definitions
	stepLines          int // sliding window step used when splitting huge definitions
	containerSkipLines int // containers larger than this emit no own chunk
	maxDocLookback     int // max comment/attribute lines pulled above a symbol
}

func NewASTChunker() *ASTChunker {
	return &ASTChunker{
		defaultChunker:     NewDefaultChunker(),
		maxChunkLines:      100,
		windowLines:        60,
		stepLines:          40,
		containerSkipLines: 50,
		maxDocLookback:     100,
	}
}

func (a *ASTChunker) ChunkFile(filePath string, content []byte) ([]Chunk, error) {
	if len(content) == 0 {
		return nil, nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	lang := languageFromExtension(ext)
	if lang == nil {
		// Unsupported extension — never recurse back through the default
		// chunker (that would infinitely recurse). Use the sliding window here.
		lines := splitLines(string(content))
		return a.defaultChunker.chunkSlidingWindow(filePath, lines)
	}

	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang); err != nil {
		lines := splitLines(string(content))
		return a.defaultChunker.chunkSlidingWindow(filePath, lines)
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		lines := splitLines(string(content))
		return a.defaultChunker.chunkSlidingWindow(filePath, lines)
	}
	defer tree.Close()

	lines := splitLines(string(content))
	var chunks []Chunk
	a.walkAST(filePath, tree.RootNode(), lines, &chunks)

	if len(chunks) == 0 {
		return a.defaultChunker.chunkSlidingWindow(filePath, lines)
	}
	return chunks, nil
}

func languageFromExtension(ext string) *sitter.Language {
	switch ext {
	case ".go":
		return sitter.NewLanguage(tree_sitter_go.Language())
	case ".py", ".pyi":
		return sitter.NewLanguage(tree_sitter_python.Language())
	case ".js", ".jsx", ".mjs", ".cjs":
		return sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	case ".ts":
		return sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	case ".tsx":
		return sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
	case ".java":
		return sitter.NewLanguage(tree_sitter_java.Language())
	case ".rs":
		return sitter.NewLanguage(tree_sitter_rust.Language())
	case ".c", ".h":
		return sitter.NewLanguage(tree_sitter_c.Language())
	case ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx":
		return sitter.NewLanguage(tree_sitter_cpp.Language())
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

// symbolKindForNode maps a tree-sitter node type to a coarse chunk kind used
// for display and synthetic-description generation.
func symbolKindForNode(nodeType string) string {
	switch nodeType {
	case "function_declaration", "function_definition", "method_declaration",
		"method_definition", "function_item", "function_signature",
		"func_declaration", "method", "constructor_declaration",
		"generator_function_declaration", "singleton_method",
		"arrow_function":
		return "function"
	case "struct_declaration", "struct_specifier", "struct_type",
		"struct_item", "type_definition", "class_struct",
		"record_declaration", "union_declaration", "union_specifier",
		"union_item":
		return "struct"
	case "class_definition", "class_specifier", "class_item", "class",
		"module", "module_item":
		return "class"
	case "interface_declaration", "interface_type", "interface_item":
		return "interface"
	case "enum_declaration", "enum_specifier", "enum_item", "enum_definition":
		return "enum"
	case "trait_declaration", "trait_definition", "trait_item":
		return "trait"
	case "impl_declaration", "impl_block", "impl_item":
		return "impl"
	case "type_spec", "type_item", "type_alias_declaration", "type_alias":
		return "type"
	default:
		return ""
	}
}

// targetKinds returns true for node kinds that should become chunks.
func targetKinds(nodeType string) bool {
	switch symbolKindForNode(nodeType) {
	case "function", "struct", "class", "interface", "enum", "trait", "impl", "type":
		return true
	}
	return false
}

func (a *ASTChunker) walkAST(filePath string, node *sitter.Node, lines []string, chunks *[]Chunk) {
	if node == nil {
		return
	}

	nodeType := node.Kind()
	kind := symbolKindForNode(nodeType)

	// Special handling for Go's type_spec — its kind depends on the type child.
	if kind == "" && nodeType == "type_spec" {
		for i := uint(0); i < node.ChildCount(); i++ {
			if child := node.Child(i); child != nil && child.Kind() == "struct_type" {
				kind = "struct"
				break
			}
		}
	}

	startPoint := node.StartPosition()
	endPoint := node.EndPosition()
	startLine := int(startPoint.Row)
	endLine := int(endPoint.Row)

	emitChunk := kind != "" && startLine >= 0 && endLine >= startLine
	// Container kinds (Rust impl blocks, class bodies) can span hundreds of
	// lines. Emitting them as huge overlapping windows produces many
	// near-duplicate chunks that drown out the precise method/function chunks
	// nested inside them. Skip the container when it is large and rely on the
	// nested definitions instead; keep small containers for context.
	if emitChunk && (kind == "impl" || kind == "class") && endLine-startLine+1 > a.containerSkipLines {
		emitChunk = false
	}

	// Skip anonymous inner nodes that only mirror their named parent (e.g. Go's
	// struct_type directly under type_spec) so the same lines are not emitted
	// twice with and without a symbol name.
	if emitChunk && node.ChildByFieldName("name") == nil && (nodeType == "struct_type" || nodeType == "class_struct") {
		emitChunk = false
	}

	if emitChunk {
		symbolName := a.extractSymbolName(node, lines)
		pkgName := extractPackageName(lines, filepath.Ext(filePath))

		// Pull the doc comment directly above the definition into the chunk so
		// the embedding model sees the developer's own summary of what the
		// symbol does. Doc comments are the single richest semantic signal for
		// conceptual queries (e.g. hugo's minifiers). For Rust, `#[...]`
		// attributes are folded in too since they sit between doc comments and
		// the item.
		chunkStart := commentLookbackStart(lines, startLine, a.maxDocLookback)

		metaPrefix := fmt.Sprintf("// [Package: %s | File: %s | Symbol: %s | Kind: %s]\n",
			pkgName, filepath.Base(filePath), symbolName, kind)

		a.emit(filePath, symbolName, kind, chunkStart, endLine, lines, metaPrefix, chunks)
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		a.walkAST(filePath, node.NamedChild(i), lines, chunks)
	}
}

// emit writes one chunk, splitting oversized definitions into a sliding window
// that still carries the structural metadata prefix and symbol name.
func (a *ASTChunker) emit(filePath, symbol, kind string, startLine, endLine int, lines []string, metaPrefix string, chunks *[]Chunk) {
	if startLine < 0 || startLine >= len(lines) {
		return
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	if endLine-startLine+1 <= a.maxChunkLines {
		a.appendChunk(filePath, symbol, kind, startLine, endLine, lines, metaPrefix, chunks)
		return
	}

	// Oversized definition: emit overlapping windows so no part of the body is
	// lost, each still anchored with the symbol metadata.
	for i := startLine; i <= endLine; i += a.stepLines {
		wEnd := i + a.windowLines - 1
		if wEnd > endLine {
			wEnd = endLine
		}
		a.appendChunk(filePath, symbol, kind, i, wEnd, lines, metaPrefix, chunks)
		if wEnd == endLine {
			break
		}
	}
}

func (a *ASTChunker) appendChunk(filePath, symbol, kind string, startLine, endLine int, lines []string, metaPrefix string, chunks *[]Chunk) {
	slice := lines[startLine : endLine+1]
	content := metaPrefix + joinLines(slice)
	if strings.TrimSpace(content) == "" {
		return
	}
	chunk := createChunk(filePath, symbol, kind, startLine+1, endLine+1, content)
	*chunks = append(*chunks, chunk)
}

func (a *ASTChunker) extractSymbolName(node *sitter.Node, lines []string) string {
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		if s := nodeText(nameNode, lines); s != "" {
			return s
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		childType := child.Kind()
		if childType == "identifier" || childType == "type_identifier" ||
			childType == "property_identifier" || childType == "field_identifier" ||
			childType == "method_name" || childType == "function_name" {
			if s := nodeText(child, lines); s != "" {
				return s
			}
		}
	}
	return ""
}

// commentLookbackStart walks backwards from startLine over contiguous comment
// (and Rust attribute) lines and returns the earliest line to include, so a
// definition chunk carries the doc comment that precedes it. It stops at the
// first blank line or any code so it never swallows unrelated comments far
// above the symbol.
func commentLookbackStart(lines []string, startLine int, max int) int {
	if startLine <= 0 || max <= 0 {
		return startLine
	}
	s := startLine
	looked := 0
	for s > 0 {
		if looked >= max {
			break
		}
		prev := strings.TrimSpace(lines[s-1])
		if prev == "" {
			break
		}
		if isDocCommentLine(prev) {
			s--
			looked++
			continue
		}
		break
	}
	return s
}

// isDocCommentLine reports whether a trimmed line is a comment, doc comment or
// Rust attribute. Block-comment continuations must have trailing content or be
// inside a block, so only recognize `*` when it clearly continues a comment.
func isDocCommentLine(l string) bool {
	if strings.HasPrefix(l, "//") || strings.HasPrefix(l, "///") {
		return true
	}
	if strings.HasPrefix(l, "/*") || strings.HasPrefix(l, "*/") {
		return true
	}
	if strings.HasPrefix(l, "*") && (len(l) == 1 || l[1] == ' ' || l[1] == '/') {
		return true
	}
	if strings.HasPrefix(l, "#[") || strings.HasPrefix(l, "#!") {
		return true
	}
	if strings.HasPrefix(l, "# ") {
		return true
	}
	return false
}

func nodeText(node *sitter.Node, lines []string) string {	startPoint := node.StartPosition()
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

func extractPackageName(lines []string, ext string) string {
	switch ext {
	case ".rs":
		// Rust: `mod foo;` at top of file — fall through to attribute-based guess.
		return ""
	}
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "package ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return "main"
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
