package nmap

import (
	"os"
	"path/filepath"
	"testing"

	"injectctl/internal/core"
)

func TestParseTextNmap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "scan.nmap")
	content := "22/tcp open ssh OpenSSH 8.2p1 Ubuntu 4ubuntu0.11\n80/tcp open http Apache httpd 2.4.41"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write nmap fixture: %v", err)
	}

	artifact, observations, err := Parse(core.Artifact{
		ID:   "abc123",
		Path: path,
	})
	if err != nil {
		t.Fatalf("parse nmap text: %v", err)
	}
	if artifact.Parser != "nmap" {
		t.Fatalf("expected nmap parser, got %q", artifact.Parser)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(observations))
	}
	if observations[0].Title != "Open service detected" {
		t.Fatalf("unexpected observation title: %q", observations[0].Title)
	}
}
