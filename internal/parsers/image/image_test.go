package image

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"injectctl/internal/core"
)

type stubOCR struct {
	available bool
	text      string
}

func (s stubOCR) Available() bool {
	return s.available
}

func (s stubOCR) ExtractText(context.Context, string) (string, error) {
	return s.text, nil
}

func TestParseEnrichesImageMetadataAndObservations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.png")
	writeSolidPNG(t, path, 320, 140)

	artifact, observations, warnings, err := Parse(context.Background(), core.Artifact{
		ID:   "artifact-1",
		Path: path,
		Kind: core.ArtifactImage,
	}, stubOCR{
		available: true,
		text:      "nmap -sV 10.0.0.5\n22/tcp open ssh\n80/tcp open http",
	})
	if err != nil {
		t.Fatalf("parse image artifact: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if artifact.Parser != "image" {
		t.Fatalf("expected image parser, got %q", artifact.Parser)
	}
	if artifact.Metadata["ocr_status"] != "succeeded" {
		t.Fatalf("expected successful OCR status, got %q", artifact.Metadata["ocr_status"])
	}
	if artifact.Metadata["image_width"] != "320" || artifact.Metadata["image_height"] != "140" {
		t.Fatalf("unexpected image dimensions metadata: %v", artifact.Metadata)
	}
	if artifact.Metadata["screenshot_surface"] != "terminal" {
		t.Fatalf("expected terminal surface, got %q", artifact.Metadata["screenshot_surface"])
	}
	if artifact.Metadata["detected_ips"] != "10.0.0.5" {
		t.Fatalf("unexpected detected IPs: %q", artifact.Metadata["detected_ips"])
	}
	if artifact.Metadata["detected_ports"] != "22/tcp,80/tcp" {
		t.Fatalf("unexpected detected ports: %q", artifact.Metadata["detected_ports"])
	}
	if artifact.Metadata["detected_services"] != "ssh,http" {
		t.Fatalf("unexpected detected services: %q", artifact.Metadata["detected_services"])
	}
	if artifact.Metadata["detected_commands"] != "nmap,ssh" {
		t.Fatalf("unexpected detected commands: %q", artifact.Metadata["detected_commands"])
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(observations))
	}
	if observations[1].Title != "Indicators detected in screenshot" {
		t.Fatalf("unexpected secondary observation title: %q", observations[1].Title)
	}
}

func writeSolidPNG(t *testing.T, path string, width, height int) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
}
