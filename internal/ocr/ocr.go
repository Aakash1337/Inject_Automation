package ocr

import (
	"context"
	"errors"
	"os/exec"
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
	_, err := exec.LookPath("tesseract")
	return err == nil
}

func (t Tesseract) ExtractText(ctx context.Context, path string) (string, error) {
	if !t.Available() {
		return "", ErrUnavailable
	}

	cmd := exec.CommandContext(ctx, "tesseract", path, "stdout", "--dpi", "300")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
