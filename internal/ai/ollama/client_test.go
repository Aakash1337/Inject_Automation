package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
		Endpoint:       server.URL,
		Model:          "gemma4:26b",
		FallbackModel:  "gemma4:e4b",
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

func TestSmokeTestUsesPrimaryModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{
					{"name": "gemma4:26b"},
					{"name": "gemma4:e4b"},
				},
			})
		case "/api/generate":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"response": "OK",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(core.AIConfig{
		Endpoint:       server.URL,
		Model:          "gemma4:26b",
		FallbackModel:  "gemma4:e4b",
		TimeoutSeconds: 30,
	})

	model, warnings, err := client.SmokeTest(context.Background())
	if err != nil {
		t.Fatalf("smoke test: %v", err)
	}
	if model != "gemma4:26b" {
		t.Fatalf("expected primary model, got %q", model)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestPlanPromptBatchesSplitsObservationWindowsAndKeepsLinkedArtifacts(t *testing.T) {
	t.Parallel()

	artifacts := []core.Artifact{
		{ID: "artifact-1", Path: "screen.png"},
		{ID: "artifact-2", Path: "notes.txt"},
		{ID: "artifact-3", Path: "scan.nmap"},
	}
	observations := []core.Observation{
		{ID: "obs-1", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1"}}},
		{ID: "obs-2", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-2"}}},
		{ID: "obs-3", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-3"}}},
	}

	batches, warnings := planPromptBatches(core.AIConfig{
		MaxPromptArtifacts:    2,
		MaxPromptObservations: 2,
	}, artifacts, observations)

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "split synthesis across 2 batches") {
		t.Fatalf("expected split warning, got %v", warnings)
	}
	if batches[0].Label != "evidence batch 1 of 2" {
		t.Fatalf("unexpected first batch label %q", batches[0].Label)
	}
	if len(batches[0].Observations) != 2 || batches[0].Observations[0].ID != "obs-1" || batches[0].Observations[1].ID != "obs-2" {
		t.Fatalf("unexpected first batch observations: %+v", batches[0].Observations)
	}
	if len(batches[0].Artifacts) != 2 || batches[0].Artifacts[0].ID != "artifact-1" || batches[0].Artifacts[1].ID != "artifact-2" {
		t.Fatalf("unexpected first batch artifacts: %+v", batches[0].Artifacts)
	}
	if len(batches[1].Observations) != 1 || batches[1].Observations[0].ID != "obs-3" {
		t.Fatalf("unexpected second batch observations: %+v", batches[1].Observations)
	}
	if len(batches[1].Artifacts) == 0 || batches[1].Artifacts[0].ID != "artifact-3" {
		t.Fatalf("expected linked artifact first in second batch, got %+v", batches[1].Artifacts)
	}
}

func TestBuildAssessmentPromptIncludesEvidenceHighlights(t *testing.T) {
	t.Parallel()

	prompt, warnings := buildAssessmentPrompt(core.Config{
		Mode:           core.ModeAssess,
		Title:          "Assessment",
		Client:         "Example Corp",
		Environment:    "Prod",
		Classification: "TLP:AMBER",
		Instructions:   "Review the evidence.",
		AI: core.AIConfig{
			MaxPromptArtifacts:    4,
			MaxPromptObservations: 10,
		},
	}, []core.Artifact{
		{ID: "artifact-1", Kind: core.ArtifactImage, ExtractedText: "ssh and http visible"},
	}, []core.Observation{
		{
			ID:         "obs-priv",
			Title:      "Privilege escalation or root access visible",
			Detail:     "Screenshot indicates a privilege escalation path or root-level compromise was achieved.",
			Category:   "privilege",
			Severity:   "critical",
			Confidence: 0.92,
			Evidence:   []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Snippet: "CVE-2022-37706 ... root shell"}},
		},
		{
			ID:         "obs-image",
			Title:      "Screenshot text extracted",
			Detail:     "Generic OCR text.",
			Category:   "image",
			Confidence: 0.82,
		},
	}, "full evidence set")

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if !strings.Contains(prompt, "Evidence Highlights:") {
		t.Fatalf("expected evidence highlights in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Privilege escalation or root access visible") {
		t.Fatalf("expected high-signal observation in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Assessment requirements:") {
		t.Fatalf("expected assessment guidance in prompt, got %q", prompt)
	}
}

func TestSynthesizeAssessmentUsesChunkedReduction(t *testing.T) {
	t.Parallel()

	server, prompts := newQueuedGenerateServer(
		[]string{
			mustJSON(map[string]any{
				"executive_summary": "Batch one summary",
				"findings": []map[string]any{
					{
						"id":              "finding-1",
						"title":           "SSH exposed",
						"severity":        "medium",
						"description":     "SSH was visible in batch one.",
						"impact":          "Remote access surface exists.",
						"remediation":     "Restrict SSH access.",
						"evidence_refs":   []string{"artifact-1:ssh evidence"},
						"observation_ids": []string{"obs-1"},
					},
				},
			}),
			mustJSON(map[string]any{
				"executive_summary": "Batch two summary",
				"findings": []map[string]any{
					{
						"id":              "finding-2",
						"title":           "HTTP exposed",
						"severity":        "low",
						"description":     "HTTP was visible in batch two.",
						"impact":          "Web surface exists.",
						"remediation":     "Review web exposure.",
						"evidence_refs":   []string{"artifact-2:http evidence"},
						"observation_ids": []string{"obs-2"},
					},
				},
			}),
			mustJSON(map[string]any{
				"executive_summary": "Merged summary",
				"findings": []map[string]any{
					{
						"id":              "finding-merged-1",
						"title":           "SSH exposed",
						"severity":        "medium",
						"description":     "SSH remains the primary issue.",
						"impact":          "Remote access surface exists.",
						"remediation":     "Restrict SSH access.",
						"evidence_refs":   []string{"artifact-1:ssh evidence"},
						"observation_ids": []string{"obs-1"},
					},
					{
						"id":              "finding-merged-2",
						"title":           "HTTP exposed",
						"severity":        "low",
						"description":     "HTTP remains a secondary issue.",
						"impact":          "Web surface exists.",
						"remediation":     "Review web exposure.",
						"evidence_refs":   []string{"artifact-2:http evidence"},
						"observation_ids": []string{"obs-2"},
					},
				},
			}),
		},
	)
	defer server.Close()

	client := New(core.AIConfig{
		Endpoint:              server.URL,
		Model:                 "gemma4:26b",
		FallbackModel:         "gemma4:e4b",
		TimeoutSeconds:        30,
		MaxPromptArtifacts:    2,
		MaxPromptObservations: 1,
	})

	draft, trace, warnings, err := client.SynthesizeAssessment(context.Background(), core.Config{
		Mode:         core.ModeAssess,
		Title:        "Chunked Test",
		Client:       "Local",
		Environment:  "Lab",
		Instructions: "Create evidence-linked findings only.",
		AI:           client.cfg,
	}, []core.Artifact{
		{ID: "artifact-1", Path: "notes-1.txt", Kind: core.ArtifactText},
		{ID: "artifact-2", Path: "notes-2.txt", Kind: core.ArtifactText},
	}, []core.Observation{
		{ID: "obs-1", Title: "SSH", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "ssh evidence"}}},
		{ID: "obs-2", Title: "HTTP", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-2", Description: "http evidence"}}},
	})
	if err != nil {
		t.Fatalf("chunked assessment synthesis failed: %v", err)
	}
	if trace.BatchCount != 2 {
		t.Fatalf("expected batch count 2, got %d", trace.BatchCount)
	}
	if len(trace.Stages) != 3 {
		t.Fatalf("expected 3 AI stages, got %d", len(trace.Stages))
	}
	if len(trace.ModelsUsed) != 1 || trace.ModelsUsed[0] != "gemma4:26b" {
		t.Fatalf("unexpected models used trace: %+v", trace.ModelsUsed)
	}
	if draft.ExecutiveSummary != "Merged summary" {
		t.Fatalf("expected merged summary, got %q", draft.ExecutiveSummary)
	}
	if len(draft.Findings) != 2 {
		t.Fatalf("expected 2 merged findings, got %d", len(draft.Findings))
	}
	if !slices.ContainsFunc(warnings, func(warning string) bool {
		return strings.Contains(warning, "assessment synthesis used 2 prompt batches")
	}) {
		t.Fatalf("expected chunk warning, got %v", warnings)
	}
	if len(*prompts) != 3 {
		t.Fatalf("expected 3 generate prompts, got %d", len(*prompts))
	}
	if !strings.Contains((*prompts)[0], "Scope: evidence batch 1 of 2") {
		t.Fatalf("expected first prompt to include batch scope, got %q", (*prompts)[0])
	}
	if !strings.Contains((*prompts)[2], "Consolidate the following partial assessment drafts") {
		t.Fatalf("expected reduction prompt, got %q", (*prompts)[2])
	}
}

func TestSynthesizeAssessmentRepairsSemanticallyEmptyDraft(t *testing.T) {
	t.Parallel()

	server, prompts := newQueuedGenerateServer(
		[]string{
			mustJSON(map[string]any{
				"executive_summary": "short summary",
				"findings":          []map[string]any{},
			}),
			mustJSON(map[string]any{
				"executive_summary": "Evidence shows exposed services and credential-backed access that require analyst review.",
				"findings": []map[string]any{
					{
						"id":              "finding-1",
						"title":           "Credentials Exposed in Evidence",
						"severity":        "high",
						"description":     "Configuration output exposes reusable application credentials in the evidence set.",
						"impact":          "Exposed credentials can enable unauthorized access and follow-on compromise.",
						"remediation":     "Rotate the credentials and remove secrets from accessible files immediately.",
						"evidence_refs":   []string{"artifact-1:credential evidence"},
						"observation_ids": []string{"obs-1"},
					},
				},
			}),
		},
	)
	defer server.Close()

	client := New(core.AIConfig{
		Endpoint:       server.URL,
		Model:          "gemma4:26b",
		FallbackModel:  "gemma4:e4b",
		TimeoutSeconds: 30,
	})

	draft, _, _, err := client.SynthesizeAssessment(context.Background(), core.Config{
		Mode:         core.ModeAssess,
		Title:        "Repair Test",
		Client:       "Local",
		Environment:  "Lab",
		Instructions: "Create evidence-linked findings only.",
		AI:           client.cfg,
	}, []core.Artifact{
		{ID: "artifact-1", Path: "notes.txt", Kind: core.ArtifactText},
	}, []core.Observation{
		{
			ID:       "obs-1",
			Title:    "Credentials visible in screenshot",
			Detail:   "Screenshot content appears to expose reusable credentials or application secrets.",
			Category: "credential",
			Severity: "high",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "credential evidence"}},
		},
	})
	if err != nil {
		t.Fatalf("assessment synthesis repair failed: %v", err)
	}
	if len(draft.Findings) != 1 {
		t.Fatalf("expected repaired findings, got %+v", draft.Findings)
	}
	if len(*prompts) != 2 {
		t.Fatalf("expected initial prompt plus repair prompt, got %d prompts", len(*prompts))
	}
	if !strings.Contains((*prompts)[1], "Problems:") {
		t.Fatalf("expected repair prompt to include validation problems, got %q", (*prompts)[1])
	}
}

func TestSynthesizeAssessmentFallsBackToDeterministicMergeWhenReductionFails(t *testing.T) {
	t.Parallel()

	server, _ := newQueuedGenerateServer(
		[]string{
			mustJSON(map[string]any{
				"executive_summary": "Batch one summary",
				"findings": []map[string]any{
					{
						"id":              "finding-1",
						"title":           "SSH exposed",
						"severity":        "medium",
						"description":     "SSH evidence",
						"impact":          "Impact",
						"remediation":     "Remediation",
						"evidence_refs":   []string{"artifact-1:ssh evidence"},
						"observation_ids": []string{"obs-1"},
					},
				},
			}),
			mustJSON(map[string]any{
				"executive_summary": "Batch two summary",
				"findings": []map[string]any{
					{
						"id":              "finding-2",
						"title":           "SSH exposed",
						"severity":        "medium",
						"description":     "Same issue from another source",
						"impact":          "Impact",
						"remediation":     "Remediation",
						"evidence_refs":   []string{"artifact-2:ssh evidence second source"},
						"observation_ids": []string{"obs-2"},
					},
				},
			}),
			"not valid json",
			"still not valid json",
		},
	)
	defer server.Close()

	client := New(core.AIConfig{
		Endpoint:              server.URL,
		Model:                 "gemma4:26b",
		FallbackModel:         "gemma4:e4b",
		TimeoutSeconds:        30,
		MaxPromptArtifacts:    2,
		MaxPromptObservations: 1,
	})

	draft, trace, warnings, err := client.SynthesizeAssessment(context.Background(), core.Config{
		Mode:         core.ModeAssess,
		Title:        "Reduction Fallback Test",
		Client:       "Local",
		Environment:  "Lab",
		Instructions: "Create evidence-linked findings only.",
		AI:           client.cfg,
	}, []core.Artifact{
		{ID: "artifact-1", Path: "notes-1.txt", Kind: core.ArtifactText},
		{ID: "artifact-2", Path: "notes-2.txt", Kind: core.ArtifactText},
	}, []core.Observation{
		{ID: "obs-1", Title: "SSH", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "ssh evidence"}}},
		{ID: "obs-2", Title: "SSH duplicate", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-2", Description: "ssh evidence second source"}}},
	})
	if err != nil {
		t.Fatalf("assessment synthesis with deterministic reduction fallback failed: %v", err)
	}
	if trace.BatchCount != 2 {
		t.Fatalf("expected batch count 2, got %d", trace.BatchCount)
	}
	if len(draft.Findings) != 1 {
		t.Fatalf("expected deterministic merge to deduplicate findings, got %d", len(draft.Findings))
	}
	if len(draft.Findings[0].EvidenceRefs) != 2 {
		t.Fatalf("expected merged evidence refs, got %v", draft.Findings[0].EvidenceRefs)
	}
	if !slices.ContainsFunc(warnings, func(warning string) bool {
		return strings.Contains(warning, "using deterministic merge of chunk drafts")
	}) {
		t.Fatalf("expected deterministic merge warning, got %v", warnings)
	}
}

func TestSynthesizeInjectUsesChunkedReduction(t *testing.T) {
	t.Parallel()

	server, prompts := newQueuedGenerateServer(
		[]string{
			mustJSON(map[string]any{
				"scenario_summary": "Batch one scenario",
				"items": []map[string]any{
					{
						"id":              "inject-1",
						"title":           "SSH Alert",
						"audience":        "Blue Team",
						"channel":         "Email",
						"trigger":         "Hour 1",
						"expected_action": "Investigate SSH.",
						"body":            "SSH is visible.",
						"evidence_refs":   []string{"artifact-1:ssh evidence"},
						"observation_ids": []string{"obs-1"},
					},
				},
			}),
			mustJSON(map[string]any{
				"scenario_summary": "Batch two scenario",
				"items": []map[string]any{
					{
						"id":              "inject-2",
						"title":           "HTTP Alert",
						"audience":        "Blue Team",
						"channel":         "Chat",
						"trigger":         "Hour 2",
						"expected_action": "Investigate HTTP.",
						"body":            "HTTP is visible.",
						"evidence_refs":   []string{"artifact-2:http evidence"},
						"observation_ids": []string{"obs-2"},
					},
				},
			}),
			mustJSON(map[string]any{
				"scenario_summary": "Merged scenario",
				"items": []map[string]any{
					{
						"id":              "inject-1",
						"title":           "SSH Alert",
						"audience":        "Blue Team",
						"channel":         "Email",
						"trigger":         "Hour 1",
						"expected_action": "Investigate SSH.",
						"body":            "SSH is visible.",
						"evidence_refs":   []string{"artifact-1:ssh evidence"},
						"observation_ids": []string{"obs-1"},
					},
					{
						"id":              "inject-2",
						"title":           "HTTP Alert",
						"audience":        "Blue Team",
						"channel":         "Chat",
						"trigger":         "Hour 2",
						"expected_action": "Investigate HTTP.",
						"body":            "HTTP is visible.",
						"evidence_refs":   []string{"artifact-2:http evidence"},
						"observation_ids": []string{"obs-2"},
					},
				},
			}),
		},
	)
	defer server.Close()

	client := New(core.AIConfig{
		Endpoint:              server.URL,
		Model:                 "gemma4:26b",
		FallbackModel:         "gemma4:e4b",
		TimeoutSeconds:        30,
		MaxPromptArtifacts:    2,
		MaxPromptObservations: 1,
	})

	draft, trace, warnings, err := client.SynthesizeInject(context.Background(), core.Config{
		Mode:         core.ModeInject,
		Title:        "Chunked Inject Test",
		Client:       "Local",
		Environment:  "Lab",
		Instructions: "Create evidence-linked inject items only.",
		AI:           client.cfg,
	}, []core.Artifact{
		{ID: "artifact-1", Path: "notes-1.txt", Kind: core.ArtifactText},
		{ID: "artifact-2", Path: "notes-2.txt", Kind: core.ArtifactText},
	}, []core.Observation{
		{ID: "obs-1", Title: "SSH", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "ssh evidence"}}},
		{ID: "obs-2", Title: "HTTP", Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-2", Description: "http evidence"}}},
	})
	if err != nil {
		t.Fatalf("chunked inject synthesis failed: %v", err)
	}
	if trace.BatchCount != 2 {
		t.Fatalf("expected batch count 2, got %d", trace.BatchCount)
	}
	if len(trace.Stages) != 3 {
		t.Fatalf("expected 3 AI stages, got %d", len(trace.Stages))
	}
	if draft.ScenarioSummary != "Merged scenario" {
		t.Fatalf("expected merged scenario, got %q", draft.ScenarioSummary)
	}
	if len(draft.Items) != 2 {
		t.Fatalf("expected 2 merged inject items, got %d", len(draft.Items))
	}
	if !slices.ContainsFunc(warnings, func(warning string) bool {
		return strings.Contains(warning, "inject synthesis used 2 prompt batches")
	}) {
		t.Fatalf("expected chunk warning, got %v", warnings)
	}
	if len(*prompts) != 3 || !strings.Contains((*prompts)[2], "Consolidate the following partial inject drafts") {
		t.Fatalf("expected inject reduction prompt, got %v", *prompts)
	}
}

func newQueuedGenerateServer(responses []string) (*httptest.Server, *[]string) {
	var (
		mu      sync.Mutex
		prompts []string
		index   int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{
					{"name": "gemma4:26b"},
					{"name": "gemma4:e4b"},
				},
			})
		case "/api/generate":
			var req generateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			prompts = append(prompts, req.Prompt)
			if index >= len(responses) {
				mu.Unlock()
				http.Error(w, "no queued response available", http.StatusInternalServerError)
				return
			}
			response := responses[index]
			index++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"response": response,
			})
		default:
			http.NotFound(w, r)
		}
	}))

	return server, &prompts
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
