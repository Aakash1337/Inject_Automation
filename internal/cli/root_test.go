package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"injectctl/internal/core"
	"injectctl/internal/project"
)

func TestRunInitManifestCreatesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "job.yaml")
	if err := Run(context.Background(), []string{"init", "manifest", "--mode", "assess", "--out", manifestPath}); err != nil {
		t.Fatalf("run init manifest: %v", err)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest file to exist: %v", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(data), "template:") {
		t.Fatalf("expected generated manifest to rely on built-in template, got %s", string(data))
	}
}

func TestRunInitDemoCreatesArtifactsAndManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	demoPath := filepath.Join(dir, "demo")
	if err := Run(context.Background(), []string{"init", "demo", "--mode", "assess", "--out", demoPath}); err != nil {
		t.Fatalf("run init demo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(demoPath, "job.yaml")); err != nil {
		t.Fatalf("expected demo manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(demoPath, "artifacts", "notes.txt")); err != nil {
		t.Fatalf("expected demo notes artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(demoPath, "artifacts", "scan.nmap")); err != nil {
		t.Fatalf("expected demo nmap artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(demoPath, "artifacts", "terminal.png")); err != nil {
		t.Fatalf("expected demo screenshot artifact: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(demoPath, "job.yaml"))
	if err != nil {
		t.Fatalf("read demo manifest: %v", err)
	}
	if strings.Contains(string(data), "template:") {
		t.Fatalf("expected demo manifest to rely on built-in template, got %s", string(data))
	}
}

func TestRunTemplateValidateAcceptsValidTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	templatePath := filepath.Join(dir, "template.md.j2")
	if err := os.WriteFile(templatePath, []byte("# {{ config.title }}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := Run(context.Background(), []string{"template", "validate", "--template", templatePath}); err != nil {
		t.Fatalf("validate template: %v", err)
	}
}

func TestRunTemplateListAcceptsTemplateDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	templatePath := filepath.Join(dir, "assessment-report.md.j2")
	if err := os.WriteFile(templatePath, []byte("# {{ config.title }}\n{{ draft.executive_summary }}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := Run(context.Background(), []string{"template", "list", "--dir", dir, "--mode", "assess"}); err != nil {
		t.Fatalf("list templates: %v", err)
	}
}

func TestRunDoctorAgainstMockServer(t *testing.T) {
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
				"response": "OK",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := Run(context.Background(), []string{"doctor", "--endpoint", server.URL, "--profile", "balanced", "--smoke"}); err != nil {
		t.Fatalf("run doctor: %v", err)
	}
}

func TestWriteRunSummaryPrintsUsefulFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writeRunSummary(&buf, project.RunSummary{
		Status: "draft_ready",
		AI: &core.RunAITrace{
			ModelsUsed: []string{"gemma4:e4b"},
			BatchCount: 2,
		},
		Warnings:    []string{"warning-1"},
		OutputFiles: []string{"assessment-report-draft.md", "assessment-report-data.json"},
	}, "C:\\out", "C:\\project")

	rendered := buf.String()
	if !strings.Contains(rendered, "Status: draft_ready") {
		t.Fatalf("expected status in summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "Output directory: C:\\out") {
		t.Fatalf("expected output dir in summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "Project snapshot: C:\\project") {
		t.Fatalf("expected project dir in summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "AI models used: gemma4:e4b") {
		t.Fatalf("expected AI model in summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "AI batch count: 2") {
		t.Fatalf("expected batch count in summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "Warnings: 1") {
		t.Fatalf("expected warning count in summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "assessment-report-draft.md") || !strings.Contains(rendered, "assessment-report-data.json") {
		t.Fatalf("expected output files in summary, got %q", rendered)
	}
}

func TestResolveOutputDirUsesManifestBaseForDefaultValue(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("out", "out", "")

	resolved, err := resolveOutputDir(fs, "C:\\repo\\demo", "out")
	if err != nil {
		t.Fatalf("resolve output dir: %v", err)
	}
	expected := filepath.Join("C:\\repo\\demo", "out")
	if resolved != expected {
		t.Fatalf("expected %q, got %q", expected, resolved)
	}
}

func TestResolveOutputDirUsesWorkingDirectoryForExplicitRelativeValue(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("out", "out", "")
	if err := fs.Parse([]string{"--out", "custom-out"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	resolved, err := resolveOutputDir(fs, "C:\\repo\\demo", "custom-out")
	if err != nil {
		t.Fatalf("resolve output dir: %v", err)
	}
	expected, err := filepath.Abs("custom-out")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if resolved != expected {
		t.Fatalf("expected %q, got %q", expected, resolved)
	}
}
