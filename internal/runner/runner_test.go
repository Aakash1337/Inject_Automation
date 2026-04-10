package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"injectctl/internal/config"
	"injectctl/internal/core"
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
