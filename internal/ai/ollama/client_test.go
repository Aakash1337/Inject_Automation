package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"injectctl/internal/core"
)

func TestLoadPromptBundleUsesCustomPromptDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "assessment_system.txt"), []byte("custom assessment system"), 0o644); err != nil {
		t.Fatalf("write assessment prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inject_system.txt"), []byte("custom inject system"), 0o644); err != nil {
		t.Fatalf("write inject prompt: %v", err)
	}

	bundle, warnings, err := loadPromptBundle(core.AIConfig{PromptDir: dir})
	if err != nil {
		t.Fatalf("load prompt bundle: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if bundle.assessmentSystem != "custom assessment system" {
		t.Fatalf("unexpected assessment prompt: %q", bundle.assessmentSystem)
	}
	if bundle.injectSystem != "custom inject system" {
		t.Fatalf("unexpected inject prompt: %q", bundle.injectSystem)
	}
}

func TestResolvePrimaryModelFallsBackWhenPrimaryMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{
				{"name": "gemma4:e4b"},
			},
		})
	}))
	defer server.Close()

	client := New(core.AIConfig{
		Endpoint:      server.URL,
		Model:         "gemma4:26b",
		FallbackModel: "gemma4:e4b",
		TimeoutSeconds: 30,
	})

	model, warnings, err := client.resolvePrimaryModel(context.Background())
	if err != nil {
		t.Fatalf("resolve primary model: %v", err)
	}
	if model != "gemma4:e4b" {
		t.Fatalf("expected fallback model, got %q", model)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "using fallback model") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestSummarizePromptInputsTruncatesLargeInputs(t *testing.T) {
	t.Parallel()

	artifacts := []core.Artifact{
		{ID: "a1", ExtractedText: strings.Repeat("x", 1300)},
		{ID: "a2", ExtractedText: "second"},
	}
	observations := []core.Observation{
		{ID: "o1", Detail: strings.Repeat("y", 700)},
		{ID: "o2", Detail: "second"},
	}

	summarizedArtifacts, summarizedObservations, warnings := summarizePromptInputs(core.AIConfig{
		MaxPromptArtifacts:    1,
		MaxPromptObservations: 1,
	}, artifacts, observations)

	if len(summarizedArtifacts) != 1 || len(summarizedObservations) != 1 {
		t.Fatalf("expected truncation to one artifact and one observation")
	}
	if len(warnings) != 2 {
		t.Fatalf("expected truncation warnings, got %v", warnings)
	}
	if !strings.HasSuffix(summarizedArtifacts[0].ExtractedText, "...") {
		t.Fatalf("expected truncated artifact text, got %q", summarizedArtifacts[0].ExtractedText)
	}
	if !strings.HasSuffix(summarizedObservations[0].Detail, "...") {
		t.Fatalf("expected truncated observation detail, got %q", summarizedObservations[0].Detail)
	}
}
