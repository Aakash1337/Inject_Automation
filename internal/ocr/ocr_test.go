package ocr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTesseractBinaryUsesExplicitPath(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	binary := filepath.Join(dir, "tesseract.exe")
	if err := os.WriteFile(binary, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write stub binary: %v", err)
	}
	t.Setenv("TESSERACT_PATH", binary)

	resolved, err := resolveTesseractBinary()
	if err != nil {
		t.Fatalf("resolve tesseract binary: %v", err)
	}
	if resolved != binary {
		t.Fatalf("expected %q, got %q", binary, resolved)
	}
}
