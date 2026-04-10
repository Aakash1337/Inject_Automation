package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"injectctl/internal/core"

	"gopkg.in/yaml.v3"
)

func DefaultConfig() core.Config {
	return core.Config{
		Classification: "TLP:AMBER",
		AI: core.AIConfig{
			Provider:              "ollama",
			Endpoint:              "http://127.0.0.1:11434",
			Model:                 "gemma4:26b",
			FallbackModel:         "gemma4:e4b",
			Temperature:           0.2,
			MaxTokens:             2048,
			TimeoutSeconds:        90,
			MaxPromptArtifacts:    12,
			MaxPromptObservations: 40,
		},
		Output: core.OutputConfig{
			Formats: []string{"markdown", "json", "pdf"},
		},
	}
}

func Load(path string) (core.Config, string, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, "", fmt.Errorf("read manifest: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, "", fmt.Errorf("decode json manifest: %w", err)
		}
	default:
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, "", fmt.Errorf("decode yaml manifest: %w", err)
		}
	}

	if err := Validate(&cfg); err != nil {
		return cfg, "", err
	}

	return cfg, filepath.Dir(path), nil
}

func Validate(cfg *core.Config) error {
	if cfg.Mode != core.ModeAssess && cfg.Mode != core.ModeInject {
		return errors.New("manifest mode must be assess or inject")
	}
	if cfg.AI.Provider == "" {
		cfg.AI.Provider = "ollama"
	}
	if cfg.AI.Provider != "ollama" {
		return errors.New("only ollama is supported in v1")
	}
	if cfg.AI.Endpoint == "" {
		cfg.AI.Endpoint = "http://127.0.0.1:11434"
	}
	if cfg.AI.Model == "" {
		cfg.AI.Model = "gemma4:26b"
	}
	if cfg.AI.MaxTokens <= 0 {
		cfg.AI.MaxTokens = 2048
	}
	if cfg.AI.TimeoutSeconds <= 0 {
		cfg.AI.TimeoutSeconds = 90
	}
	if cfg.AI.MaxPromptArtifacts <= 0 {
		cfg.AI.MaxPromptArtifacts = 12
	}
	if cfg.AI.MaxPromptObservations <= 0 {
		cfg.AI.MaxPromptObservations = 40
	}
	if len(cfg.Output.Formats) == 0 {
		cfg.Output.Formats = []string{"markdown", "json", "pdf"}
	}
	for i, format := range cfg.Output.Formats {
		cfg.Output.Formats[i] = strings.ToLower(strings.TrimSpace(format))
	}
	return nil
}

func ResolvePaths(cfg *core.Config, baseDir string) {
	if cfg.Template != "" && !filepath.IsAbs(cfg.Template) {
		cfg.Template = filepath.Join(baseDir, cfg.Template)
	}
	if cfg.AI.PromptDir != "" && !filepath.IsAbs(cfg.AI.PromptDir) {
		cfg.AI.PromptDir = filepath.Join(baseDir, cfg.AI.PromptDir)
	}
	if cfg.Output.ProjectDir != "" && !filepath.IsAbs(cfg.Output.ProjectDir) {
		cfg.Output.ProjectDir = filepath.Join(baseDir, cfg.Output.ProjectDir)
	}
	for i, artifact := range cfg.Artifacts {
		if filepath.IsAbs(artifact) {
			continue
		}
		cfg.Artifacts[i] = filepath.Join(baseDir, artifact)
	}
}

func MarshalYAML(cfg core.Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}
