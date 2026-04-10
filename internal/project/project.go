package project

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"injectctl/internal/core"
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
