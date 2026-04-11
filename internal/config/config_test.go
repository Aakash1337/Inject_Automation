package config

import (
	"os"
	"path/filepath"
	"testing"

	"injectctl/internal/core"
)

func TestLoadAppliesDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "job.yaml")
	content := []byte("mode: assess\ninstructions: test instructions\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg, baseDir, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if baseDir != dir {
		t.Fatalf("expected base dir %q, got %q", dir, baseDir)
	}
	if cfg.AI.Provider != "ollama" {
		t.Fatalf("expected ollama provider, got %q", cfg.AI.Provider)
	}
	if cfg.AI.Profile != "balanced" {
		t.Fatalf("expected balanced profile, got %q", cfg.AI.Profile)
	}
	if cfg.AI.Model != "gemma4:26b" {
		t.Fatalf("expected default model, got %q", cfg.AI.Model)
	}
	if cfg.AI.TimeoutSeconds != 90 {
		t.Fatalf("expected default timeout seconds, got %d", cfg.AI.TimeoutSeconds)
	}
	if cfg.AI.MaxPromptArtifacts != 12 {
		t.Fatalf("expected default max prompt artifacts, got %d", cfg.AI.MaxPromptArtifacts)
	}
	if cfg.AI.MaxPromptObservations != 40 {
		t.Fatalf("expected default max prompt observations, got %d", cfg.AI.MaxPromptObservations)
	}
	if len(cfg.Output.Formats) != 3 {
		t.Fatalf("expected default formats, got %v", cfg.Output.Formats)
	}
}

func TestLoadProfileAppliesProfileModels(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "job.yaml")
	content := []byte("mode: inject\ninstructions: test instructions\nai:\n  profile: quality\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AI.Profile != "quality" {
		t.Fatalf("expected quality profile, got %q", cfg.AI.Profile)
	}
	if cfg.AI.Model != "gemma4:31b" {
		t.Fatalf("expected quality profile model, got %q", cfg.AI.Model)
	}
	if cfg.AI.FallbackModel != "gemma4:26b" {
		t.Fatalf("expected quality profile fallback, got %q", cfg.AI.FallbackModel)
	}
}

func TestResolvePathsMakesRelativeEntriesAbsolute(t *testing.T) {
	t.Parallel()

	cfg := core.Config{
		Template:    "templates/default/assessment.md.tmpl",
		TemplateDir: "templates/custom",
		AI: core.AIConfig{
			PromptDir: "prompts",
		},
		Artifacts: []string{
			"./artifacts",
		},
		Output: core.OutputConfig{
			ProjectDir: "./project",
		},
	}

	ResolvePaths(&cfg, "C:\\repo")

	if cfg.Template != filepath.Join("C:\\repo", "templates/default/assessment.md.tmpl") {
		t.Fatalf("unexpected template path: %q", cfg.Template)
	}
	if cfg.TemplateDir != filepath.Join("C:\\repo", "templates/custom") {
		t.Fatalf("unexpected template dir: %q", cfg.TemplateDir)
	}
	if cfg.AI.PromptDir != filepath.Join("C:\\repo", "prompts") {
		t.Fatalf("unexpected prompt dir: %q", cfg.AI.PromptDir)
	}
	if cfg.Artifacts[0] != filepath.Join("C:\\repo", "artifacts") {
		t.Fatalf("unexpected artifact path: %q", cfg.Artifacts[0])
	}
	if cfg.Output.ProjectDir != filepath.Join("C:\\repo", "project") {
		t.Fatalf("unexpected project dir: %q", cfg.Output.ProjectDir)
	}
}
