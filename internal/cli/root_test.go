package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
}

func TestRunTemplateValidateAcceptsValidTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	templatePath := filepath.Join(dir, "template.tmpl")
	if err := os.WriteFile(templatePath, []byte("# {{ .Config.Title }}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := Run(context.Background(), []string{"template", "validate", "--template", templatePath}); err != nil {
		t.Fatalf("validate template: %v", err)
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
