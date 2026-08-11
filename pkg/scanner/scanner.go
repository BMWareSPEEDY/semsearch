package scanner

import (
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
