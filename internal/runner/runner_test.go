package runner

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"injectctl/internal/config"
	"injectctl/internal/core"
	"injectctl/internal/ocr"
)

func TestRunAssessmentEndToEnd(t *testing.T) {
	t.Parallel()

	server := newMockOllamaServer(t, map[string]any{
		"executive_summary": "Draft summary from mocked Ollama.",
		"findings": []map[string]any{
			{
				"id":              "finding-1",
				"title":           "SSH exposed",
				"severity":        "medium",
				"description":     "SSH was detected on the target host.",
				"impact":          "Remote access surface exists.",
				"remediation":     "Restrict SSH access.",
				"evidence_refs":   []string{"obs1:Nmap open port line"},
				"observation_ids": []string{"obs1"},
			},
		},
	})
	defer server.Close()

	dir := t.TempDir()
	artifactDir := filepath.Join(dir, "artifacts")
	outDir := filepath.Join(dir, "out")
	projectDir := filepath.Join(dir, "project")
	manifestPath := filepath.Join(dir, "job.yaml")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "scan.nmap"), []byte("22/tcp open ssh OpenSSH 8.2p1"), 0o644); err != nil {
		t.Fatalf("write nmap artifact: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("mode: assess\ninstructions: test\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Mode = core.ModeAssess
	cfg.Title = "Assessment Run"
	cfg.Client = "Example Corp"
	cfg.Environment = "Prod"
	cfg.Classification = "TLP:AMBER"
	cfg.Instructions = "Draft an assessment report."
	cfg.Artifacts = []string{artifactDir}
	cfg.AI.Endpoint = server.URL
	cfg.Output.ProjectDir = projectDir

	if err := Run(context.Background(), Options{
		Config:       cfg,
		InputPaths:   nil,
		OutputDir:    outDir,
		ManifestPath: manifestPath,
	}); err != nil {
		t.Fatalf("run assessment: %v", err)
	}

	assertFileContains(t, filepath.Join(outDir, "assessment.md"), "SSH exposed")
	assertFileContains(t, filepath.Join(outDir, "assessment.md"), "draft_ready")
	assertFileContains(t, filepath.Join(outDir, "assessment.json"), "executive_summary")
	assertFileContains(t, filepath.Join(outDir, "evidence-index.json"), "\"artifact_id\"")
	assertFileContains(t, filepath.Join(outDir, "evidence-index.md"), "Evidence Index")
	assertFileExists(t, filepath.Join(outDir, "assessment.pdf"))
	assertFileExists(t, filepath.Join(projectDir, "assessment-run.json"))
	assertFileExists(t, filepath.Join(projectDir, "job.yaml"))
}

func TestRunInjectEndToEnd(t *testing.T) {
	t.Parallel()

	server := newMockOllamaServer(t, map[string]any{
		"scenario_summary": "Draft inject scenario from mocked Ollama.",
		"items": []map[string]any{
			{
				"id":              "inject-1",
				"title":           "Suspicious SSH Alert",
				"audience":        "Blue Team",
				"channel":         "Email",
				"trigger":         "At scenario hour 1",
				"expected_action": "Investigate the host and document escalation steps.",
				"body":            "Analysts observe SSH activity on an external-facing system.",
				"evidence_refs":   []string{"obs1:Nmap open port line"},
				"observation_ids": []string{"obs1"},
			},
		},
	})
	defer server.Close()

	dir := t.TempDir()
	artifactDir := filepath.Join(dir, "artifacts")
	outDir := filepath.Join(dir, "out")
	projectDir := filepath.Join(dir, "project")
	manifestPath := filepath.Join(dir, "job.yaml")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "notes.txt"), []byte("SSH was exposed on the internet-facing host."), 0o644); err != nil {
		t.Fatalf("write text artifact: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("mode: inject\ninstructions: test\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Mode = core.ModeInject
	cfg.Title = "Inject Run"
	cfg.Client = "Example Corp"
	cfg.Environment = "Prod"
	cfg.Classification = "TLP:AMBER"
	cfg.Instructions = "Draft an inject pack."
	cfg.Artifacts = []string{artifactDir}
	cfg.AI.Endpoint = server.URL
	cfg.Output.ProjectDir = projectDir

	if err := Run(context.Background(), Options{
		Config:       cfg,
		InputPaths:   nil,
		OutputDir:    outDir,
		ManifestPath: manifestPath,
	}); err != nil {
		t.Fatalf("run inject: %v", err)
	}

	assertFileContains(t, filepath.Join(outDir, "inject.md"), "Suspicious SSH Alert")
	assertFileContains(t, filepath.Join(outDir, "inject.md"), "draft_ready")
	assertFileContains(t, filepath.Join(outDir, "inject.json"), "scenario_summary")
	assertFileContains(t, filepath.Join(outDir, "evidence-index.json"), "\"artifact_id\"")
	assertFileContains(t, filepath.Join(outDir, "evidence-index.md"), "Evidence Index")
	assertFileExists(t, filepath.Join(outDir, "inject.pdf"))
	assertFileExists(t, filepath.Join(projectDir, "inject-run.json"))
	assertFileExists(t, filepath.Join(projectDir, "job.yaml"))
}

func TestRunAssessmentFallsBackToEvidenceOnlyOnSynthesisFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{
					{"name": "gemma4:26b"},
					{"name": "gemma4:e4b"},
				},
			})
		case "/api/generate":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"response": "not valid json",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	artifactDir := filepath.Join(dir, "artifacts")
	outDir := filepath.Join(dir, "out")
	projectDir := filepath.Join(dir, "project")
	manifestPath := filepath.Join(dir, "job.yaml")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "scan.nmap"), []byte("22/tcp open ssh OpenSSH 8.2p1"), 0o644); err != nil {
		t.Fatalf("write nmap artifact: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("mode: assess\ninstructions: test\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Mode = core.ModeAssess
	cfg.Title = "Assessment Run"
	cfg.Client = "Example Corp"
	cfg.Environment = "Prod"
	cfg.Classification = "TLP:AMBER"
	cfg.Instructions = "Draft an assessment report."
	cfg.Artifacts = []string{artifactDir}
	cfg.AI.Endpoint = server.URL
	cfg.Output.ProjectDir = projectDir

	if err := Run(context.Background(), Options{
		Config:       cfg,
		InputPaths:   nil,
		OutputDir:    outDir,
		ManifestPath: manifestPath,
	}); err != nil {
		t.Fatalf("run assessment: %v", err)
	}

	assertFileContains(t, filepath.Join(outDir, "assessment.md"), "evidence_only")
	assertFileContains(t, filepath.Join(outDir, "assessment.md"), "Generation Error")
	assertFileContains(t, filepath.Join(outDir, "assessment.json"), "\"status\": \"evidence_only\"")
	assertFileContains(t, filepath.Join(outDir, "assessment.json"), "\"error_report\"")
	assertFileContains(t, filepath.Join(outDir, "evidence-index.json"), "\"artifact_id\"")
}

func TestRunAssessmentEndToEndWithRealOCRScreenshot(t *testing.T) {
	if !ocr.New().Available() {
		t.Skip("tesseract is not available")
	}

	server := newMockOllamaServer(t, map[string]any{
		"executive_summary": "Draft summary from mocked Ollama.",
		"findings": []map[string]any{
			{
				"id":              "finding-1",
				"title":           "Services visible in screenshot",
				"severity":        "low",
				"description":     "The screenshot contains visible service names.",
				"impact":          "The artifact can be summarized correctly.",
				"remediation":     "Review visible services.",
				"evidence_refs":   []string{"artifact-1:OCR text extracted from screenshot"},
				"observation_ids": []string{"obs1"},
			},
		},
	})
	defer server.Close()

	dir := t.TempDir()
	artifactDir := filepath.Join(dir, "artifacts")
	outDir := filepath.Join(dir, "out")
	projectDir := filepath.Join(dir, "project")
	manifestPath := filepath.Join(dir, "job.yaml")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	imagePath := filepath.Join(artifactDir, "screen.png")
	writeOCRFixturePNG(t, imagePath, []string{"SSH", "HTTP"})
	if err := os.WriteFile(manifestPath, []byte("mode: assess\ninstructions: test\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Mode = core.ModeAssess
	cfg.Title = "Assessment Run"
	cfg.Client = "Example Corp"
	cfg.Environment = "Prod"
	cfg.Classification = "TLP:AMBER"
	cfg.Instructions = "Draft an assessment report."
	cfg.Artifacts = []string{artifactDir}
	cfg.AI.Endpoint = server.URL
	cfg.AI.Profile = "fast"
	cfg.AI.Model = "gemma4:e4b"
	cfg.AI.FallbackModel = "gemma4:e2b"
	cfg.Output.ProjectDir = projectDir

	if err := Run(context.Background(), Options{
		Config:       cfg,
		InputPaths:   nil,
		OutputDir:    outDir,
		ManifestPath: manifestPath,
	}); err != nil {
		t.Fatalf("run assessment with OCR screenshot: %v", err)
	}

	assertFileContains(t, filepath.Join(outDir, "assessment.md"), "draft_ready")
	result := readAssessmentResult(t, filepath.Join(outDir, "assessment.json"))
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(result.Artifacts))
	}
	artifact := result.Artifacts[0]
	if artifact.Metadata["ocr_status"] != "succeeded" {
		t.Fatalf("expected OCR success, got metadata %v", artifact.Metadata)
	}
	if artifact.Metadata["ocr_line_count"] == "" {
		t.Fatalf("expected OCR line count in metadata, got %v", artifact.Metadata)
	}
	if artifact.Metadata["image_width"] == "" || artifact.Metadata["image_height"] == "" {
		t.Fatalf("expected image dimensions in metadata, got %v", artifact.Metadata)
	}
	lowerText := strings.ToLower(artifact.ExtractedText)
	if !strings.Contains(lowerText, "ssh") && !strings.Contains(lowerText, "http") {
		t.Fatalf("expected OCR text to contain ssh or http, got %q", artifact.ExtractedText)
	}
}

func newMockOllamaServer(t *testing.T, responsePayload map[string]any) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{
					{"name": "gemma4:26b"},
					{"name": "gemma4:e4b"},
				},
			})
		case "/api/generate":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"response": mustJSON(responsePayload),
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %q to exist: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, substring string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %q: %v", path, err)
	}
	if !strings.Contains(string(data), substring) {
		t.Fatalf("expected file %q to contain %q", path, substring)
	}
}

func readAssessmentResult(t *testing.T, path string) core.AssessmentResult {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read assessment result %q: %v", path, err)
	}
	var result core.AssessmentResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode assessment result %q: %v", path, err)
	}
	return result
}

var bitmapFont = map[rune][]string{
	'H': {
		"10001",
		"10001",
		"11111",
		"10001",
		"10001",
		"10001",
		"10001",
	},
	'P': {
		"11110",
		"10001",
		"10001",
		"11110",
		"10000",
		"10000",
		"10000",
	},
	'S': {
		"01111",
		"10000",
		"10000",
		"01110",
		"00001",
		"00001",
		"11110",
	},
	'T': {
		"11111",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
	},
	' ': {
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
	},
}

func writeOCRFixturePNG(t *testing.T, path string, lines []string) {
	t.Helper()

	const (
		scale       = 14
		glyphWidth  = 5
		glyphHeight = 7
		padding     = 30
		lineGap     = 18
	)

	maxRunes := 0
	for _, line := range lines {
		if len([]rune(line)) > maxRunes {
			maxRunes = len([]rune(line))
		}
	}
	width := padding*2 + maxRunes*(glyphWidth*scale+scale)
	height := padding*2 + len(lines)*(glyphHeight*scale) + (len(lines)-1)*lineGap
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fillImage(img, color.White)

	for lineIdx, line := range lines {
		y := padding + lineIdx*(glyphHeight*scale+lineGap)
		x := padding
		for _, char := range line {
			drawGlyph(img, x, y, char, scale)
			x += glyphWidth*scale + scale
		}
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create OCR fixture png: %v", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode OCR fixture png: %v", err)
	}
}

func fillImage(img *image.RGBA, fill color.Color) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, fill)
		}
	}
}

func drawGlyph(img *image.RGBA, startX, startY int, char rune, scale int) {
	pattern, ok := bitmapFont[char]
	if !ok {
		pattern = bitmapFont[' ']
	}
	for row, line := range pattern {
		for col, pixel := range line {
			if pixel != '1' {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(startX+col*scale+dx, startY+row*scale+dy, color.Black)
				}
			}
		}
	}
}
