package project

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"injectctl/internal/core"
	evidencerender "injectctl/internal/render/evidence"
)

func WriteAssessment(dir, manifestPath string, result core.AssessmentResult) error {
	if err := snapshotManifest(dir, manifestPath); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "assessment-run.json"), result)
}

func WriteInject(dir, manifestPath string, result core.InjectResult) error {
	if err := snapshotManifest(dir, manifestPath); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "inject-run.json"), result)
}

func WriteEvidenceIndex(dir string, index core.EvidenceIndex) error {
	if err := evidencerender.WriteJSON(filepath.Join(dir, "evidence-index.json"), index); err != nil {
		return err
	}
	return evidencerender.WriteMarkdown(filepath.Join(dir, "evidence-index.md"), index)
}

func WriteRunSummary(dir string, summary RunSummary) error {
	return writeJSON(filepath.Join(dir, "run-summary.json"), summary)
}

func CopyOutputs(projectDir, outputDir string, files []string) error {
	targetDir := filepath.Join(projectDir, "outputs")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	for _, name := range files {
		sourcePath := filepath.Join(outputDir, name)
		source, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("open output for snapshot: %w", err)
		}

		targetPath := filepath.Join(targetDir, filepath.Base(name))
		target, err := os.Create(targetPath)
		if err != nil {
			source.Close()
			return fmt.Errorf("create output snapshot: %w", err)
		}

		if _, err := io.Copy(target, source); err != nil {
			source.Close()
			target.Close()
			return fmt.Errorf("copy output snapshot: %w", err)
		}
		source.Close()
		if err := target.Close(); err != nil {
			return err
		}
	}
	return nil
}

func WriteOutputInventory(dir string, inventory OutputInventory) error {
	return writeJSON(filepath.Join(dir, "output-inventory.json"), inventory)
}

func WriteRunLog(dir string, entries []RunLogEntry) error {
	file, err := os.Create(filepath.Join(dir, "run-log.jsonl"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	return writer.Flush()
}

type RunSummary struct {
	RunID            string           `json:"run_id"`
	Mode             string           `json:"mode"`
	Title            string           `json:"title"`
	Status           string           `json:"status"`
	Model            string           `json:"model"`
	AI               *core.RunAITrace `json:"ai,omitempty"`
	ArtifactCount    int              `json:"artifact_count"`
	ObservationCount int              `json:"observation_count"`
	Warnings         []string         `json:"warnings,omitempty"`
	Errors           []string         `json:"errors,omitempty"`
	OutputFiles      []string         `json:"output_files,omitempty"`
}

type OutputInventory struct {
	RunID      string                 `json:"run_id"`
	Mode       string                 `json:"mode"`
	OutputDir  string                 `json:"output_dir"`
	ProjectDir string                 `json:"project_dir"`
	Generated  time.Time              `json:"generated_at"`
	Files      []OutputInventoryEntry `json:"files"`
}

type OutputInventoryEntry struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	RelativePath string    `json:"relative_path"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modified_at"`
}

type RunLogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Event     string            `json:"event"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

func snapshotManifest(dir, manifestPath string) error {
	if manifestPath == "" {
		return nil
	}
	source, err := os.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("open manifest for snapshot: %w", err)
	}
	defer source.Close()

	destination, err := os.Create(filepath.Join(dir, filepath.Base(manifestPath)))
	if err != nil {
		return fmt.Errorf("create manifest snapshot: %w", err)
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func writeJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
