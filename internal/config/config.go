package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"injectctl/internal/ai/modelprofile"
	"injectctl/internal/core"

	"gopkg.in/yaml.v3"
)

func DefaultConfig() core.Config {
	return core.Config{
		Classification: "TLP:AMBER",
		AI: core.AIConfig{
			Provider:              "ollama",
			Endpoint:              "http://127.0.0.1:11434",
			Profile:               "balanced",
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
	cfg := core.Config{}
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
	if err := ApplyAIDefaults(&cfg.AI); err != nil {
		return err
	}
	if cfg.Classification == "" {
		cfg.Classification = "TLP:AMBER"
	}
	if cfg.AI.Temperature == 0 {
		cfg.AI.Temperature = 0.2
	}
	if len(cfg.Output.Formats) == 0 {
		cfg.Output.Formats = []string{"markdown", "json", "pdf"}
	}
	for i, format := range cfg.Output.Formats {
		cfg.Output.Formats[i] = strings.ToLower(strings.TrimSpace(format))
	}
	return nil
}

func ApplyAIDefaults(ai *core.AIConfig) error {
	if ai.Provider == "" {
		ai.Provider = "ollama"
	}
	if ai.Provider != "ollama" {
		return errors.New("only ollama is supported in v1")
	}
	if ai.Endpoint == "" {
		ai.Endpoint = "http://127.0.0.1:11434"
	}
	if ai.Profile == "" {
		ai.Profile = "balanced"
	}
	profile, ok := modelprofile.Resolve(ai.Profile)
	if !ok {
		return fmt.Errorf("unsupported ai profile %q; supported values are fast, balanced, quality", ai.Profile)
	}
	ai.Profile = profile.Name
	if ai.Model == "" {
		ai.Model = profile.Model
	}
	if ai.FallbackModel == "" {
		ai.FallbackModel = profile.FallbackModel
	}
	if ai.MaxTokens <= 0 {
		ai.MaxTokens = 2048
	}
	if ai.TimeoutSeconds <= 0 {
		ai.TimeoutSeconds = 90
	}
	if ai.MaxPromptArtifacts <= 0 {
		ai.MaxPromptArtifacts = 12
	}
	if ai.MaxPromptObservations <= 0 {
		ai.MaxPromptObservations = 40
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
