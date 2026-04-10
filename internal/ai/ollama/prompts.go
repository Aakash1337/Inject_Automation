package ollama

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"injectctl/internal/core"
)

//go:embed prompt_defaults/*.txt
var embeddedPrompts embed.FS

type promptBundle struct {
	assessmentSystem string
	injectSystem     string
}

func loadPromptBundle(cfg core.AIConfig) (promptBundle, []string, error) {
	bundle := promptBundle{}
	var warnings []string

	var err error
	bundle.assessmentSystem, err = readPrompt(cfg.PromptDir, "assessment_system.txt")
	if err != nil {
		return bundle, warnings, err
	}
	bundle.injectSystem, err = readPrompt(cfg.PromptDir, "inject_system.txt")
	if err != nil {
		return bundle, warnings, err
	}

	if cfg.PromptDir != "" {
		for _, name := range []string{"assessment_system.txt", "inject_system.txt"} {
			if _, err := os.Stat(filepath.Join(cfg.PromptDir, name)); err != nil {
				warnings = append(warnings, "custom prompt file not found, using embedded default: "+name)
			}
		}
	}

	return bundle, warnings, nil
}

func readPrompt(promptDir, name string) (string, error) {
	if promptDir != "" {
		path := filepath.Join(promptDir, name)
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data)), nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("read custom prompt %s: %w", path, err)
		}
	}

	data, err := embeddedPrompts.ReadFile(filepath.ToSlash(filepath.Join("prompt_defaults", name)))
	if err != nil {
		return "", fmt.Errorf("read embedded prompt %s: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}
