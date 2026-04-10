package evidence

import (
	"testing"

	"injectctl/internal/core"
)

func TestBuildEvidenceIndex(t *testing.T) {
	t.Parallel()

	run := core.RunRecord{RunID: "run-1", Mode: core.ModeAssess}
	artifacts := []core.Artifact{
		{
			ID:     "artifact-1",
			Path:   "/tmp/scan.nmap",
			Kind:   core.ArtifactNmap,
			Parser: "nmap",
			MIME:   "text/plain",
			SHA256: "abc",
		},
	}
	observations := []core.Observation{
		{
			ID: "obs-1",
			Evidence: []core.EvidenceExcerpt{
				{
					ArtifactID: "artifact-1",
					Snippet:    "22/tcp open ssh",
				},
			},
		},
	}

	index := Build(run, artifacts, observations)
	if index.RunID != "run-1" {
		t.Fatalf("unexpected run id: %q", index.RunID)
	}
	if len(index.Entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(index.Entries))
	}
	if len(index.Entries[0].ObservationIDs) != 1 || index.Entries[0].ObservationIDs[0] != "obs-1" {
		t.Fatalf("expected linked observation ids, got %v", index.Entries[0].ObservationIDs)
	}
	if len(index.Entries[0].Snippets) != 1 || index.Entries[0].Snippets[0] != "22/tcp open ssh" {
		t.Fatalf("expected linked snippets, got %v", index.Entries[0].Snippets)
	}
}
