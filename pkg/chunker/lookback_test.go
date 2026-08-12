package chunker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
)

func TestASTChunkerIncludesDocComment(t *testing.T) {
	tempDir := t.TempDir()
	src := `package demo

// Client wraps the minifier registry.
// It is used during site publish.
type Client struct {
	Name string
}

// Minify minifies the src into dst given a MIME type.
func (m Client) Minify(t, w, r interface{}) error {
	return nil
}
`
	path := filepath.Join(tempDir, "minify.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	c := chunker.NewASTChunker()
	chunks, err := c.ChunkFile(path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var minify *chunker.Chunk
	for i := range chunks {
		if chunks[i].SymbolName == "Minify" {
			minify = &chunks[i]
			break
		}
	}
	if minify == nil {
		t.Fatalf("no Minify chunk found in %d chunks", len(chunks))
	}
	if !strings.Contains(minify.Content, "Minify minifies the src into dst") {
		t.Errorf("doc comment missing from chunk:\n%s", minify.Content)
	}
	if !strings.HasPrefix(minify.Content, "// [Package:") {
		t.Errorf("chunk should start with meta header:\n%s", minify.Content)
	}
}

func TestChunkCodeTextStripsDocComment(t *testing.T) {
	content := "// [Package: demo | File: minify.go | Symbol: Minify | Kind: function]\n// Minify minifies the src into dst given a MIME type.\nfunc (m Client) Minify(t, w, r interface{}) error {\n\treturn nil\n}"
	got := chunker.ChunkCodeText(content)
	if strings.Contains(got, "minifies the src") {
		t.Errorf("code text still contains doc comment:\n%s", got)
	}
	if !strings.HasPrefix(got, "// [Package:") || !strings.Contains(got, "func (m Client) Minify") {
		t.Errorf("code text lost header or definition:\n%s", got)
	}

	// Markdown content must pass through untouched.
	md := "# Title\n\nSome paragraph.\n"
	if got := chunker.ChunkCodeText(md); got != md {
		t.Errorf("markdown chunk was altered:\n%q", got)
	}
}

func TestSyntheticDescriptionUsesDocCommentAndPath(t *testing.T) {
	content := "// [Package: demo | File: minify.go | Symbol: Minify | Kind: function]\n// Minify minifies the src into dst given a MIME type.\nfunc Minify(...) error { return nil }"
	desc := chunker.GenerateSyntheticDescription(content, "Minify", "function_declaration", "/repo/minifiers/minifiers.go")
	if !strings.Contains(desc, "minifiers/minifiers.go") {
		t.Errorf("desc missing module path hint: %q", desc)
	}
	if !strings.Contains(desc, "minifies the src") {
		t.Errorf("desc missing doc comment: %q", desc)
	}

	fallback := chunker.GenerateSyntheticDescription("// [Package: x | File: y.go | Symbol: DoThing | Kind: function]\nfunc DoThing() {}", "DoThing", "function_declaration", "/repo/cmd/y.go")
	if !strings.Contains(fallback, "Function DoThing") {
		t.Errorf("fallback template desc broken: %q", fallback)
	}
}
