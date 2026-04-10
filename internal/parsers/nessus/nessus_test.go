package nessus

import (
	"os"
	"path/filepath"
	"testing"

	"injectctl/internal/core"
)

func TestParseCSVNessus(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nessus.csv")
	content := "Host,Port,Name,Risk,Synopsis,Solution\n10.0.0.5,443,SSL Certificate Expired,High,Certificate is expired,Replace certificate\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write nessus fixture: %v", err)
	}

	artifact, observations, err := Parse(core.Artifact{
		ID:   "def456",
		Path: path,
	})
	if err != nil {
		t.Fatalf("parse nessus csv: %v", err)
	}
	if artifact.Parser != "nessus" {
		t.Fatalf("expected nessus parser, got %q", artifact.Parser)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	if observations[0].Severity != "high" {
		t.Fatalf("expected high severity, got %q", observations[0].Severity)
	}
}
