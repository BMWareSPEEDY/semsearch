package scanner

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/monochromegane/go-gitignore"
)

var defaultIgnoredDirs = map[string]bool{
	".git":          true,
	".semsearch":    true,
	"node_modules":  true,
	"dist":          true,
	"build":         true,
	"target":        true,
	"__pycache__":   true,
	".venv":         true,
	"vendor":        true,
	".next":         true,
	".out":          true,
}

var defaultIgnoredExts = map[string]bool{
	// Binary / compressed
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".7z": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".wasm": true,
	".bin": true, ".onnx": true,
	// Locks
	".sum": true,
}

var defaultIgnoredFiles = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"Cargo.lock":        true,
	"poetry.lock":       true,
	"desc_cache.json":   true,
	"history.json":      true,
}

type FileScanner struct {
	rootDir string
	ignores []gitignore.IgnoreMatcher
}

func openFile(path string) *os.File {
	f, _ := os.Open(path)
	return f
}

func NewFileScanner(rootDir string) (*FileScanner, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	scanner := &FileScanner{
		rootDir: absRoot,
		ignores: []gitignore.IgnoreMatcher{},
	}

	// Load top-level .gitignore if present
	gitIgnorePath := filepath.Join(absRoot, ".gitignore")
	if f, err := os.Open(gitIgnorePath); err == nil {
		matcher := gitignore.NewGitIgnoreFromReader(absRoot, f)
		f.Close()
		scanner.ignores = append(scanner.ignores, matcher)
	}

	// Load top-level .semsearchignore if present
	semsearchIgnorePath := filepath.Join(absRoot, ".semsearchignore")
	if f, err := os.Open(semsearchIgnorePath); err == nil {
		matcher := gitignore.NewGitIgnoreFromReader(absRoot, f)
		f.Close()
		scanner.ignores = append(scanner.ignores, matcher)
	}

	return scanner, nil
}

func (fs *FileScanner) Scan() ([]string, error) {
	var files []string

	err := filepath.Walk(fs.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(fs.rootDir, path)
		if err != nil {
			return nil
		}

		if relPath == "." {
			return nil
		}

		baseName := filepath.Base(path)

		if info.IsDir() {
			if defaultIgnoredDirs[baseName] || strings.HasPrefix(baseName, ".") {
				return filepath.SkipDir
			}
			for _, matcher := range fs.ignores {
				if matcher.Match(filepath.Join(fs.rootDir, relPath), true) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Skip hidden dotfiles, gitignore files, compiled binaries, or unit test files
		if strings.HasPrefix(baseName, ".") || defaultIgnoredFiles[baseName] || baseName == "semsearch" || strings.HasSuffix(baseName, "_test.go") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if defaultIgnoredExts[ext] {
			return nil
		}

		if isSkippableContent(path) {
			return nil
		}

		for _, matcher := range fs.ignores {
			if matcher.Match(filepath.Join(fs.rootDir, relPath), false) {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// isSkippableContent reports whether a file should be excluded, opening it
// once: NUL bytes within the first 4KB indicate binary content, and a single
// line longer than 20KB anywhere in the file indicates a minified bundle.
// Binary content with a source-like extension (e.g. a PNG renamed to .js)
// would otherwise be handed to the tree-sitter AST chunker, which can take
// pathological amounts of time trying to recover; minified bundles are
// low-information and their many near-identical chunks pollute results.
func isSkippableContent(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Binary sniff: a NUL byte in the first 4KB is a strong signal.
	head := make([]byte, 4096)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return false
	}
	for _, b := range head[:n] {
		if b == 0 {
			return true
		}
	}

	// Minified sniff: a single line above 20KB chars is a strong signal.
	if _, err := f.Seek(0, 0); err != nil {
		return false
	}
	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := reader.ReadSlice('\n')
		if len(line) > 20*1024 {
			return true
		}
		if err != nil {
			return false
		}
	}
}
