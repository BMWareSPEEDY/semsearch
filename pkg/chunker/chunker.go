package chunker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

type Chunk struct {
	ID          int    `json:"id"`
	FilePath    string `json:"file_path"`
	SymbolName  string `json:"symbol_name,omitempty"`
	Kind        string `json:"kind"` // "function", "struct", "class", "markdown_section", "text_block"
	LineStart   int    `json:"line_start"`
	LineEnd     int    `json:"line_end"`
	ContentHash string `json:"content_hash"`
	Content     string `json:"content"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview"`
}

type ChunkFile interface {
	ChunkFile(filePath string) ([]Chunk, error)
	ChunkBytes(filePath string, content []byte) ([]Chunk, error)
}

// Chunker is the chunking API used by the indexing pipeline.
type Chunker interface {
	ChunkFile
}

type DefaultChunker struct {
	TargetTokens int
	Overlap      int
}

func NewDefaultChunker() *DefaultChunker {
	return &DefaultChunker{
		TargetTokens: 300,
		Overlap:      50,
	}
}

func (c *DefaultChunker) ChunkFile(filePath string) ([]Chunk, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return c.ChunkBytes(filePath, content)
}

// ChunkBytes chunks content already read from disk, avoiding a second file
// read by callers that already have the bytes (indexing, the watcher).
func (c *DefaultChunker) ChunkBytes(filePath string, content []byte) ([]Chunk, error) {
	if len(content) == 0 {
		return nil, nil
	}

	// ScanLines semantics: split on '\n' and strip a trailing '\r' (CRLF), so
	// cross-platform files produce chunk content identical to the bufio path.
	lines := splitLinesExact(string(content))

	if len(lines) == 0 {
		return nil, nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".md" || ext == ".markdown" {
		return c.chunkMarkdown(filePath, lines)
	}

	// Route tree-sitter supported languages through the AST chunker. It falls
	// back to the sliding window internally for unparseable files.
	if languageFromExtension(ext) != nil {
		astChunker := NewASTChunker()
		if chunks, err := astChunker.ChunkFile(filePath, content); err == nil && len(chunks) > 0 {
			return chunks, nil
		}
	}

	return c.chunkSlidingWindow(filePath, lines)
}

func (c *DefaultChunker) chunkMarkdown(filePath string, lines []string) ([]Chunk, error) {
	var chunks []Chunk
	var currentSection []string
	sectionTitle := "Overview"
	startLine := 1

	for i, line := range lines {
		lineNum := i + 1
		if strings.HasPrefix(line, "#") {
			if len(currentSection) > 0 {
				content := strings.Join(currentSection, "\n")
				if strings.TrimSpace(content) != "" {
					chunks = append(chunks, createChunk(filePath, sectionTitle, "markdown_section", startLine, lineNum-1, content))
				}
				currentSection = nil
			}
			sectionTitle = strings.TrimLeft(line, "# ")
			startLine = lineNum
		}
		currentSection = append(currentSection, line)
	}

	if len(currentSection) > 0 {
		content := strings.Join(currentSection, "\n")
		if strings.TrimSpace(content) != "" {
			chunks = append(chunks, createChunk(filePath, sectionTitle, "markdown_section", startLine, len(lines), content))
		}
	}

	return chunks, nil
}

func (c *DefaultChunker) chunkSlidingWindow(filePath string, lines []string) ([]Chunk, error) {
	var chunks []Chunk
	windowSize := 30 // ~30 lines (~200-300 tokens)
	stepSize := 20   // ~10 line overlap

	for i := 0; i < len(lines); i += stepSize {
		end := i + windowSize
		if end > len(lines) {
			end = len(lines)
		}

		slice := lines[i:end]
		content := strings.Join(slice, "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}

		chunk := createChunk(filePath, "", "text_block", i+1, end, content)
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
