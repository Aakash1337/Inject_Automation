package image

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

func TestParseDerivesCredentialAndPrivilegeObservations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.png")
	writeSolidPNG(t, path, 400, 220)

	artifact, observations, warnings, err := Parse(context.Background(), core.Artifact{
		ID:   "artifact-1",
		Path: path,
		Kind: core.ArtifactImage,
	}, stubOCR{
		available: true,
		text:      "db_pass='serverfun2$2023!!'\nsu - larissa\nssh larissa@10.129.92.185\nCVE-2022-37706\ncat root.txt",
	})
	if err != nil {
		t.Fatalf("parse image artifact: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if artifact.Metadata["ocr_status"] != "succeeded" {
		t.Fatalf("expected successful OCR status, got %q", artifact.Metadata["ocr_status"])
	}

	var titles []string
	for _, observation := range observations {
		titles = append(titles, observation.Title)
	}
	expected := []string{
		"Credentials visible in screenshot",
		"Credential reuse or user access visible",
		"Privilege escalation or root access visible",
	}
	for _, title := range expected {
		found := false
		for _, actual := range titles {
			if actual == title {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected derived observation %q, got %v", title, titles)
		}
	}
}

func TestParseDerivesObservationsFromNoisyRealWorldOCR(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.png")
	writeSolidPNG(t, path, 640, 360)

	text := strings.Join([]string{
		"[x]$ gobuster vhost -u http://board.htb/ -w",
		"Found: crm.board.htb Status: 200 [Size: 6360]",
		"O & http://crm.board.htb/",
		"Dolibart",
		"Password forgotten? - Need help or support?",
		"$dolibarr_main_db_user='dolibarrowner' ;",
		"$dolibarr_main_db_pass='serverfun2$2023!!';",
		"www-data@boardlight$ su - larissa",
		"Password: serverfun2$2023!!",
		"ssh larissa@10.129.92.185",
		"CVE-2022-37706",
		"[+] Enjoy the root shell :)",
		"final flag and root access",
	}, "\n")

	_, observations, warnings, err := Parse(context.Background(), core.Artifact{
		ID:   "artifact-1",
		Path: path,
		Kind: core.ArtifactImage,
	}, stubOCR{
		available: true,
		text:      text,
	})
	if err != nil {
		t.Fatalf("parse image artifact: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	expectedTitles := []string{
		"Subdomain or vhost discovery visible",
		"Web login surface visible",
		"Credentials visible in screenshot",
		"Credential reuse or user access visible",
		"Privilege escalation or root access visible",
	}

	var titles []string
	for _, observation := range observations {
		titles = append(titles, observation.Title)
	}
	for _, expected := range expectedTitles {
		found := false
		for _, actual := range titles {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected derived observation %q, got %v", expected, titles)
		}
	}
}

func TestParseDoesNotConfuseVhostDiscoveryWithLoginSurface(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.png")
	writeSolidPNG(t, path, 640, 240)

	_, observations, warnings, err := Parse(context.Background(), core.Artifact{
		ID:   "artifact-1",
		Path: path,
		Kind: core.ArtifactImage,
	}, stubOCR{
		available: true,
		text:      "[x]$ gobuster vhost -u http://board.htb/ -w\nFound: crm.board.htb Status: 200 [Size: 6360]",
	})
	if err != nil {
		t.Fatalf("parse image artifact: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	var titles []string
	for _, observation := range observations {
		titles = append(titles, observation.Title)
	}
	for _, title := range titles {
		if title == "Web login surface visible" {
			t.Fatalf("did not expect login-surface observation for pure vhost discovery, got %v", titles)
		}
	}
	foundVhost := false
	for _, title := range titles {
		if title == "Subdomain or vhost discovery visible" {
			foundVhost = true
			break
		}
	}
	if !foundVhost {
		t.Fatalf("expected vhost discovery observation, got %v", titles)
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
