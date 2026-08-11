package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanSkipsBinaryFileWithSourceExtension(t *testing.T) {
	dir := t.TempDir()

	realJS := filepath.Join(dir, "app.js")
	if err := os.WriteFile(realJS, []byte("console.log('hi');\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// A PNG (binary, contains NUL bytes) mislabeled as .js must be skipped.
	fakeJS := filepath.Join(dir, "fake.js")
	if err := os.WriteFile(fakeJS, []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x00binarydata"), 0644); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFileScanner(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := fs.Scan()
	if err != nil {
		t.Fatal(err)
	}

	var sawReal, sawFake bool
	for _, f := range files {
		switch filepath.Base(f) {
		case "app.js":
			sawReal = true
		case "fake.js":
			sawFake = true
		}
	}

	if !sawReal {
		t.Error("expected app.js to be scanned")
	}
	if sawFake {
		t.Error("expected fake.js (binary content) to be skipped")
	}
}

func TestScanSkipsMinifiedBundle(t *testing.T) {
	dir := t.TempDir()

	minified := filepath.Join(dir, "bundle.js")
	longLine := strings.Repeat("abcdefgh", 4000) // 32KB single line
	if err := os.WriteFile(minified, []byte("!function(){"+longLine+"}();\n"), 0644); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFileScanner(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := fs.Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if filepath.Base(f) == "bundle.js" {
			t.Error("expected minified bundle.js to be skipped")
		}
	}
}
