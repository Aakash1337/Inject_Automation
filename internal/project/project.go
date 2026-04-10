package project

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

type RunSummary struct {
	RunID            string   `json:"run_id"`
	Mode             string   `json:"mode"`
	Title            string   `json:"title"`
	Status           string   `json:"status"`
	Model            string   `json:"model"`
	ArtifactCount    int      `json:"artifact_count"`
	ObservationCount int      `json:"observation_count"`
	Warnings         []string `json:"warnings,omitempty"`
	Errors           []string `json:"errors,omitempty"`
	OutputFiles      []string `json:"output_files,omitempty"`
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
