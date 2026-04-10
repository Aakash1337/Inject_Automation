package ocr

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrUnavailable = errors.New("tesseract is not available")

type Engine interface {
	Available() bool
	ExtractText(ctx context.Context, path string) (string, error)
}

type Tesseract struct{}

func New() Engine {
	return Tesseract{}
}

func (Tesseract) Available() bool {
	_, err := resolveTesseractBinary()
	return err == nil
}

func (t Tesseract) ExtractText(ctx context.Context, path string) (string, error) {
	binary, err := resolveTesseractBinary()
	if err != nil {
		return "", ErrUnavailable
	}

	cmd := exec.CommandContext(ctx, binary, path, "stdout", "--dpi", "300")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func resolveTesseractBinary() (string, error) {
	if path, err := exec.LookPath("tesseract"); err == nil {
		return path, nil
	}

	candidates := []string{
		os.Getenv("TESSERACT_PATH"),
		filepath.Join(os.Getenv("ProgramFiles"), "Tesseract-OCR", "tesseract.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Tesseract-OCR", "tesseract.exe"),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", ErrUnavailable
}
