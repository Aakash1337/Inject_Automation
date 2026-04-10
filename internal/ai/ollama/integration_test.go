package ollama

import (
	"context"
	"os"
	"testing"

	"injectctl/internal/core"
)

func TestLiveAssessmentSynthesisIntegration(t *testing.T) {
	if os.Getenv("RUN_OLLAMA_INTEGRATION") != "1" {
		t.Skip("set RUN_OLLAMA_INTEGRATION=1 to run live Ollama integration tests")
	}

	endpoint := os.Getenv("OLLAMA_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "gemma4:26b"
	}

	client := New(core.AIConfig{
		Endpoint:       endpoint,
		Model:          model,
		TimeoutSeconds: 90,
		MaxTokens:      512,
		Temperature:    0.1,
	})

	draft, warnings, err := client.SynthesizeAssessment(context.Background(), core.Config{
		Mode:         core.ModeAssess,
		Title:        "Integration Test",
		Client:       "Local",
		Environment:  "Lab",
		Instructions: "Create one conservative finding from the supplied evidence.",
		AI: core.AIConfig{
			Model:                 model,
			MaxPromptArtifacts:    4,
			MaxPromptObservations: 10,
		},
	}, []core.Artifact{
		{
			ID:            "artifact-1",
			Kind:          core.ArtifactText,
			Path:          "notes.txt",
			ExtractedText: "Observed SSH on 22/tcp and HTTP on 80/tcp.",
		},
	}, []core.Observation{
		{
			ID:       "obs-1",
			Title:    "Open service detected",
			Detail:   "22/tcp open ssh OpenSSH 8.2p1",
			Category: "network",
			Evidence: []core.EvidenceExcerpt{
				{
					ArtifactID:  "artifact-1",
					Snippet:     "22/tcp open ssh OpenSSH 8.2p1",
					Description: "Nmap open port line",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("live synthesis failed: %v (warnings: %v)", err, warnings)
	}
	if draft.ExecutiveSummary == "" {
		t.Fatalf("expected non-empty executive summary")
	}
	if len(draft.Findings) == 0 {
		t.Fatalf("expected at least one finding")
	}
}
