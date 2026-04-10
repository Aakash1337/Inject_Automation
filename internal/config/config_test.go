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
	if cfg.AI.Model != "gemma4:26b" {
		t.Fatalf("expected default model, got %q", cfg.AI.Model)
	}
	if len(cfg.Output.Formats) != 3 {
		t.Fatalf("expected default formats, got %v", cfg.Output.Formats)
	}
}

func TestResolvePathsMakesRelativeEntriesAbsolute(t *testing.T) {
	t.Parallel()

	cfg := core.Config{
		Template: "templates/default/assessment.md.tmpl",
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
	if cfg.Artifacts[0] != filepath.Join("C:\\repo", "artifacts") {
		t.Fatalf("unexpected artifact path: %q", cfg.Artifacts[0])
	}
	if cfg.Output.ProjectDir != filepath.Join("C:\\repo", "project") {
		t.Fatalf("unexpected project dir: %q", cfg.Output.ProjectDir)
	}
}
