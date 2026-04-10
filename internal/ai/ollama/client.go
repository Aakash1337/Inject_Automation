package ollama

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"injectctl/internal/core"
)

type Client struct {
	http *http.Client
	cfg  core.AIConfig
}

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type generateRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	System  string          `json:"system,omitempty"`
	Images  []string        `json:"images,omitempty"`
	Format  string          `json:"format,omitempty"`
	Stream  bool            `json:"stream"`
	Options generateOptions `json:"options,omitempty"`
}

type generateOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type generateResponse struct {
	Response string `json:"response"`
}

func New(cfg core.AIConfig) *Client {
	return &Client{
		http: &http.Client{Timeout: 90 * time.Second},
		cfg:  cfg,
	}
}

func (c *Client) Health(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/api/tags"), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/api/tags"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var models []string
	for _, model := range payload.Models {
		models = append(models, model.Name)
	}
	return models, nil
}

func (c *Client) SynthesizeAssessment(ctx context.Context, cfg core.Config, artifacts []core.Artifact, observations []core.Observation) (core.AssessmentDraft, []string, error) {
	system := "You are drafting a corporate security assessment report. Return only valid JSON."
	prompt := buildAssessmentPrompt(cfg, artifacts, observations)
	response, warnings, err := c.generate(ctx, system, prompt, artifacts)
	if err != nil {
		return core.AssessmentDraft{}, warnings, err
	}

	var draft core.AssessmentDraft
	if err := decodeStructured(response, &draft); err != nil {
		repairPrompt := "Repair this into valid JSON for the requested schema only:\n" + response
		response, repairWarnings, repairErr := c.generate(ctx, system, repairPrompt, nil)
		warnings = append(warnings, repairWarnings...)
		if repairErr != nil {
			return core.AssessmentDraft{}, warnings, err
		}
		if err := decodeStructured(response, &draft); err != nil {
			return core.AssessmentDraft{}, warnings, err
		}
	}

	applyAssessmentDefaults(&draft, observations)
	return draft, warnings, nil
}

func (c *Client) SynthesizeInject(ctx context.Context, cfg core.Config, artifacts []core.Artifact, observations []core.Observation) (core.InjectDraft, []string, error) {
	system := "You are drafting a cyber exercise inject package. Return only valid JSON."
	prompt := buildInjectPrompt(cfg, artifacts, observations)
	response, warnings, err := c.generate(ctx, system, prompt, artifacts)
	if err != nil {
		return core.InjectDraft{}, warnings, err
	}

	var draft core.InjectDraft
	if err := decodeStructured(response, &draft); err != nil {
		repairPrompt := "Repair this into valid JSON for the requested schema only:\n" + response
		response, repairWarnings, repairErr := c.generate(ctx, system, repairPrompt, nil)
		warnings = append(warnings, repairWarnings...)
		if repairErr != nil {
			return core.InjectDraft{}, warnings, err
		}
		if err := decodeStructured(response, &draft); err != nil {
			return core.InjectDraft{}, warnings, err
		}
	}

	applyInjectDefaults(&draft, observations)
	return draft, warnings, nil
}

func (c *Client) generate(ctx context.Context, system, prompt string, artifacts []core.Artifact) (string, []string, error) {
	images, warnings := encodeImages(artifacts)
	response, _, err := c.doGenerate(ctx, c.cfg.Model, system, prompt, images)
	if err == nil {
		return response, warnings, nil
	}

	if len(images) > 0 {
		warnings = append(warnings, "multimodal generation failed; retrying without images")
		response, _, imageErr := c.doGenerate(ctx, c.cfg.Model, system, prompt, nil)
		if imageErr == nil {
			return response, warnings, nil
		}
		err = imageErr
	}

	if c.cfg.FallbackModel != "" {
		warnings = append(warnings, "primary model failed; retrying with fallback model")
		response, _, fallbackErr := c.doGenerate(ctx, c.cfg.FallbackModel, system, prompt, nil)
		if fallbackErr == nil {
			return response, warnings, nil
		}
	}

	return "", warnings, err
}

func (c *Client) doGenerate(ctx context.Context, model, system, prompt string, images []string) (string, string, error) {
	body, err := json.Marshal(generateRequest{
		Model:  model,
		Prompt: prompt,
		System: system,
		Images: images,
		Format: "json",
		Stream: false,
		Options: generateOptions{
			Temperature: c.cfg.Temperature,
			NumPredict:  c.cfg.MaxTokens,
		},
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/generate"), bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("ollama generate failed: %s", strings.TrimSpace(string(payload)))
	}

	var generated generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&generated); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(generated.Response), model, nil
}

func (c *Client) url(path string) string {
	return strings.TrimRight(c.cfg.Endpoint, "/") + path
}

func encodeImages(artifacts []core.Artifact) ([]string, []string) {
	var images []string
	var warnings []string
	for _, artifact := range artifacts {
		if artifact.Kind != core.ArtifactImage {
			continue
		}
		if len(images) >= 4 {
			warnings = append(warnings, "more than four images supplied; only the first four were sent to Ollama")
			break
		}
		data, err := os.ReadFile(artifact.Path)
		if err != nil {
			warnings = append(warnings, "failed to read image "+artifact.Path)
			continue
		}
		images = append(images, base64.StdEncoding.EncodeToString(data))
	}
	return images, warnings
}

func decodeStructured(raw string, out any) error {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end < start {
		return fmt.Errorf("response did not contain a JSON object")
	}
	return json.Unmarshal([]byte(raw[start:end+1]), out)
}

func buildAssessmentPrompt(cfg core.Config, artifacts []core.Artifact, observations []core.Observation) string {
	return buildPrompt(cfg, artifacts, observations, `Return JSON with:
{
  "executive_summary": "string",
  "findings": [
    {
      "id": "string",
      "title": "string",
      "severity": "critical|high|medium|low|info",
      "description": "string",
      "impact": "string",
      "remediation": "string",
      "evidence_refs": ["artifactID:summary"],
      "observation_ids": ["id"]
    }
  ]
}`)
}

func buildInjectPrompt(cfg core.Config, artifacts []core.Artifact, observations []core.Observation) string {
	return buildPrompt(cfg, artifacts, observations, `Return JSON with:
{
  "scenario_summary": "string",
  "items": [
    {
      "id": "string",
      "title": "string",
      "audience": "string",
      "channel": "string",
      "trigger": "string",
      "expected_action": "string",
      "body": "string",
      "evidence_refs": ["artifactID:summary"],
      "observation_ids": ["id"]
    }
  ]
}`)
}

func buildPrompt(cfg core.Config, artifacts []core.Artifact, observations []core.Observation, schema string) string {
	artifactSummary, _ := json.Marshal(artifacts)
	observationSummary, _ := json.Marshal(observations)
	return strings.Join([]string{
		"Mode: " + string(cfg.Mode),
		"Title: " + cfg.Title,
		"Client: " + cfg.Client,
		"Environment: " + cfg.Environment,
		"Classification: " + cfg.Classification,
		"Instructions: " + cfg.Instructions,
		"Artifacts JSON: " + string(artifactSummary),
		"Observations JSON: " + string(observationSummary),
		"Each finding or inject item must cite evidence_refs and observation_ids. Use the supplied observations and artifacts only.",
		schema,
	}, "\n\n")
}

func applyAssessmentDefaults(draft *core.AssessmentDraft, observations []core.Observation) {
	if draft.ExecutiveSummary == "" {
		draft.ExecutiveSummary = "Draft assessment generated from local evidence."
	}
	for i := range draft.Findings {
		if draft.Findings[i].ID == "" {
			draft.Findings[i].ID = fmt.Sprintf("finding-%d", i+1)
		}
		if len(draft.Findings[i].ObservationIDs) == 0 && len(observations) > 0 {
			draft.Findings[i].ObservationIDs = []string{observations[min(i, len(observations)-1)].ID}
		}
		if len(draft.Findings[i].EvidenceRefs) == 0 && len(observations) > 0 && len(observations[min(i, len(observations)-1)].Evidence) > 0 {
			evidence := observations[min(i, len(observations)-1)].Evidence[0]
			draft.Findings[i].EvidenceRefs = []string{evidence.ArtifactID + ":" + evidence.Description}
		}
	}
}

func applyInjectDefaults(draft *core.InjectDraft, observations []core.Observation) {
	if draft.ScenarioSummary == "" {
		draft.ScenarioSummary = "Draft inject package generated from local evidence."
	}
	for i := range draft.Items {
		if draft.Items[i].ID == "" {
			draft.Items[i].ID = fmt.Sprintf("inject-%d", i+1)
		}
		if len(draft.Items[i].ObservationIDs) == 0 && len(observations) > 0 {
			draft.Items[i].ObservationIDs = []string{observations[min(i, len(observations)-1)].ID}
		}
		if len(draft.Items[i].EvidenceRefs) == 0 && len(observations) > 0 && len(observations[min(i, len(observations)-1)].Evidence) > 0 {
			evidence := observations[min(i, len(observations)-1)].Evidence[0]
			draft.Items[i].EvidenceRefs = []string{evidence.ArtifactID + ":" + evidence.Description}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
