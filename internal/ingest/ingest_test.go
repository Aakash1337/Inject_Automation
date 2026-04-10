package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"injectctl/internal/config"
	"injectctl/internal/core"
)

func TestCollectParsesTextAndNmapArtifacts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	textPath := filepath.Join(dir, "notes.txt")
	nmapPath := filepath.Join(dir, "scan.nmap")

	if err := os.WriteFile(textPath, []byte("Observed HTTP and SSH exposure."), 0o644); err != nil {
		t.Fatalf("write text fixture: %v", err)
	}
	if err := os.WriteFile(nmapPath, []byte("22/tcp open ssh OpenSSH 8.2p1\n80/tcp open http Apache 2.4.41"), 0o644); err != nil {
		t.Fatalf("write nmap fixture: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Mode = core.ModeAssess
	cfg.Artifacts = []string{dir}

	artifacts, observations, warnings, err := Collect(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Fatalf("collect artifacts: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
	if len(observations) < 3 {
		t.Fatalf("expected at least 3 observations, got %d", len(observations))
	}
}
