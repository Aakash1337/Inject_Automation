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
	"sort"
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
	Format  any             `json:"format,omitempty"`
	Stream  bool            `json:"stream"`
	Options generateOptions `json:"options,omitempty"`
}

type generateOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type generateResponse struct {
	Response   string `json:"response"`
	DoneReason string `json:"done_reason,omitempty"`
	EvalCount  int    `json:"eval_count,omitempty"`
}

type promptBatch struct {
	Label        string
	Artifacts    []core.Artifact
	Observations []core.Observation
}

type synthesisTrace struct {
	RequestedModel string
	FallbackModel  string
	ModelsUsed     []string
	Stages         []core.RunAIStage
	BatchCount     int
}

func New(cfg core.AIConfig) *Client {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Client{
		http: &http.Client{Timeout: timeout},
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

func (c *Client) SynthesizeAssessment(ctx context.Context, cfg core.Config, artifacts []core.Artifact, observations []core.Observation) (core.AssessmentDraft, core.RunAITrace, []string, error) {
	prompts, warnings, err := loadPromptBundle(cfg.AI)
	if err != nil {
		return core.AssessmentDraft{}, core.RunAITrace{}, warnings, err
	}
	batches, batchWarnings := planPromptBatches(cfg.AI, artifacts, observations)
	warnings = append(warnings, batchWarnings...)
	trace := newSynthesisTrace(cfg.AI, len(batches))

	partials := make([]core.AssessmentDraft, 0, len(batches))
	for i, batch := range batches {
		prompt, promptWarnings := buildAssessmentPrompt(cfg, batch.Artifacts, batch.Observations, batch.Label)
		warnings = append(warnings, promptWarnings...)

		var partial core.AssessmentDraft
		stageWarnings, usedModel, err := c.generateStructured(ctx, prompts.assessmentSystem, prompt, batch.Artifacts, "assessment synthesis", &partial, func() []string {
			return validateAssessmentDraft(partial)
		})
		warnings = append(warnings, stageWarnings...)
		if err != nil {
			return core.AssessmentDraft{}, trace.toCore(), warnings, fmt.Errorf("assessment batch %d/%d failed: %w", i+1, len(batches), err)
		}
		trace.addStage(fmt.Sprintf("assessment_batch_%d", i+1), usedModel)

		applyAssessmentDefaults(&partial, batch.Observations)
		partials = append(partials, partial)
	}

	if len(partials) == 1 {
		applyAssessmentDefaults(&partials[0], observations)
		return partials[0], trace.toCore(), warnings, nil
	}

	warnings = append(warnings, fmt.Sprintf("assessment synthesis used %d prompt batches", len(partials)))
	merged, usedModel, mergeWarnings, mergeErr := c.reduceAssessmentDrafts(ctx, prompts.assessmentSystem, cfg, partials)
	warnings = append(warnings, mergeWarnings...)
	trace.addStage("assessment_reduce", usedModel)
	if mergeErr != nil {
		return core.AssessmentDraft{}, trace.toCore(), warnings, mergeErr
	}
	applyAssessmentDefaults(&merged, observations)
	return merged, trace.toCore(), warnings, nil
}

func (c *Client) SynthesizeInject(ctx context.Context, cfg core.Config, artifacts []core.Artifact, observations []core.Observation) (core.InjectDraft, core.RunAITrace, []string, error) {
	prompts, warnings, err := loadPromptBundle(cfg.AI)
	if err != nil {
		return core.InjectDraft{}, core.RunAITrace{}, warnings, err
	}
	batches, batchWarnings := planPromptBatches(cfg.AI, artifacts, observations)
	warnings = append(warnings, batchWarnings...)
	trace := newSynthesisTrace(cfg.AI, len(batches))

	partials := make([]core.InjectDraft, 0, len(batches))
	for i, batch := range batches {
		prompt, promptWarnings := buildInjectPrompt(cfg, batch.Artifacts, batch.Observations, batch.Label)
		warnings = append(warnings, promptWarnings...)

		var partial core.InjectDraft
		stageWarnings, usedModel, err := c.generateStructured(ctx, prompts.injectSystem, prompt, batch.Artifacts, "inject synthesis", &partial, func() []string {
			return validateInjectDraft(partial)
		})
		warnings = append(warnings, stageWarnings...)
		if err != nil {
			return core.InjectDraft{}, trace.toCore(), warnings, fmt.Errorf("inject batch %d/%d failed: %w", i+1, len(batches), err)
		}
		trace.addStage(fmt.Sprintf("inject_batch_%d", i+1), usedModel)

		applyInjectDefaults(&partial, batch.Observations)
		partials = append(partials, partial)
	}

	if len(partials) == 1 {
		applyInjectDefaults(&partials[0], observations)
		return partials[0], trace.toCore(), warnings, nil
	}

	warnings = append(warnings, fmt.Sprintf("inject synthesis used %d prompt batches", len(partials)))
	merged, usedModel, mergeWarnings, mergeErr := c.reduceInjectDrafts(ctx, prompts.injectSystem, cfg, partials)
	warnings = append(warnings, mergeWarnings...)
	trace.addStage("inject_reduce", usedModel)
	if mergeErr != nil {
		return core.InjectDraft{}, trace.toCore(), warnings, mergeErr
	}
	applyInjectDefaults(&merged, observations)
	return merged, trace.toCore(), warnings, nil
}

func (c *Client) generate(ctx context.Context, system, prompt string, artifacts []core.Artifact, warnings []string) (string, string, []string, error) {
	model, modelWarnings, err := c.ResolveModel(ctx)
	warnings = append(warnings, modelWarnings...)
	if err != nil {
		return "", "", warnings, err
	}

	images, warnings := encodeImages(artifacts)
	response, _, err := c.doGenerate(ctx, model, system, prompt, images)
	if err == nil {
		return response, model, warnings, nil
	}

	if len(images) > 0 {
		warnings = append(warnings, "multimodal generation failed; retrying without images")
		response, _, imageErr := c.doGenerate(ctx, model, system, prompt, nil)
		if imageErr == nil {
			return response, model, warnings, nil
		}
		err = imageErr
	}

	if c.cfg.FallbackModel != "" && !strings.EqualFold(model, c.cfg.FallbackModel) {
		warnings = append(warnings, "primary model failed; retrying with fallback model")
		response, _, fallbackErr := c.doGenerate(ctx, c.cfg.FallbackModel, system, prompt, nil)
		if fallbackErr == nil {
			return response, c.cfg.FallbackModel, warnings, nil
		}
	}

	return "", "", warnings, err
}

func (c *Client) ResolveModel(ctx context.Context) (string, []string, error) {
	return c.resolvePrimaryModel(ctx)
}

func (c *Client) SmokeTest(ctx context.Context) (string, []string, error) {
	model, warnings, err := c.ResolveModel(ctx)
	if err != nil {
		return "", warnings, err
	}

	result, _, err := c.doGenerateText(ctx, model, "You are a connectivity smoke test. Reply with exactly OK.", "Reply with exactly OK.")
	if err != nil {
		if c.cfg.FallbackModel != "" && !strings.EqualFold(model, c.cfg.FallbackModel) {
			warnings = append(warnings, "primary model smoke test failed; retrying with fallback model")
			result, _, err = c.doGenerateText(ctx, c.cfg.FallbackModel, "You are a connectivity smoke test. Reply with exactly OK.", "Reply with exactly OK.")
			if err == nil {
				if strings.Contains(strings.ToUpper(result.Response), "OK") {
					return c.cfg.FallbackModel, warnings, nil
				}
				if strings.TrimSpace(result.Response) == "" && result.EvalCount > 0 {
					warnings = append(warnings, "fallback model smoke test returned an empty visible response but completed token evaluation")
					return c.cfg.FallbackModel, warnings, nil
				}
			}
		}
		return "", warnings, err
	}

	if strings.Contains(strings.ToUpper(result.Response), "OK") {
		return model, warnings, nil
	}
	if strings.TrimSpace(result.Response) == "" && result.EvalCount > 0 {
		warnings = append(warnings, "smoke test returned an empty visible response but completed token evaluation")
		return model, warnings, nil
	}
	return "", warnings, fmt.Errorf("smoke test returned unexpected response: %q", result.Response)
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

func (c *Client) doGenerateText(ctx context.Context, model, system, prompt string) (generateResponse, string, error) {
	body, err := json.Marshal(generateRequest{
		Model:  model,
		Prompt: prompt,
		System: system,
		Stream: false,
		Options: generateOptions{
			Temperature: 0,
			NumPredict:  32,
		},
	})
	if err != nil {
		return generateResponse{}, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/generate"), bytes.NewReader(body))
	if err != nil {
		return generateResponse{}, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return generateResponse{}, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return generateResponse{}, "", fmt.Errorf("ollama text generate failed: %s", strings.TrimSpace(string(payload)))
	}

	var generated generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&generated); err != nil {
		return generateResponse{}, "", err
	}
	generated.Response = strings.TrimSpace(generated.Response)
	return generated, model, nil
}

func (c *Client) url(path string) string {
	return strings.TrimRight(c.cfg.Endpoint, "/") + path
}

func (c *Client) resolvePrimaryModel(ctx context.Context) (string, []string, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list ollama models: %w", err)
	}
	if ContainsModel(models, c.cfg.Model) {
		return c.cfg.Model, nil, nil
	}
	if c.cfg.FallbackModel != "" && ContainsModel(models, c.cfg.FallbackModel) {
		return c.cfg.FallbackModel, []string{"primary model missing; using fallback model for synthesis"}, nil
	}
	return "", nil, fmt.Errorf("neither primary model %q nor fallback model %q is installed", c.cfg.Model, c.cfg.FallbackModel)
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

func (c *Client) generateStructured(ctx context.Context, system, prompt string, artifacts []core.Artifact, stage string, out any, validator func() []string) ([]string, string, error) {
	response, usedModel, warnings, err := c.generate(ctx, system, prompt, artifacts, nil)
	if err != nil {
		return warnings, usedModel, err
	}
	if err := decodeStructured(response, out); err == nil {
		if validator == nil {
			return warnings, usedModel, nil
		}
		if problems := validator(); len(problems) == 0 {
			return warnings, usedModel, nil
		} else {
			response, repairModel, repairWarnings, repairErr := c.generate(ctx, system, buildValidationRepairPrompt(prompt, response, problems), nil, nil)
			warnings = append(warnings, repairWarnings...)
			if repairErr != nil {
				return warnings, usedModel, fmt.Errorf("%s validation repair failed: %w", stage, repairErr)
			}
			if err := decodeStructured(response, out); err != nil {
				return warnings, usedModel, fmt.Errorf("%s validation repair returned invalid JSON: %w", stage, err)
			}
			if repairedProblems := validator(); len(repairedProblems) > 0 {
				return warnings, usedModel, fmt.Errorf("%s returned unusable structured output: %s", stage, strings.Join(repairedProblems, "; "))
			}
			if repairModel != "" {
				usedModel = repairModel
			}
			return warnings, usedModel, nil
		}
	}

	repairPrompt := "Repair this into valid JSON for the requested schema only:\n" + response
	response, repairModel, repairWarnings, repairErr := c.generate(ctx, system, repairPrompt, nil, nil)
	warnings = append(warnings, repairWarnings...)
	if repairErr != nil {
		return warnings, usedModel, fmt.Errorf("%s repair failed after invalid JSON: %w", stage, repairErr)
	}
	if err := decodeStructured(response, out); err != nil {
		return warnings, usedModel, fmt.Errorf("%s returned invalid JSON after repair: %w", stage, err)
	}
	if validator != nil {
		if problems := validator(); len(problems) > 0 {
			return warnings, usedModel, fmt.Errorf("%s returned unusable structured output after repair: %s", stage, strings.Join(problems, "; "))
		}
	}
	if repairModel != "" {
		usedModel = repairModel
	}
	return warnings, usedModel, nil
}

func buildAssessmentPrompt(cfg core.Config, artifacts []core.Artifact, observations []core.Observation, scope string) (string, []string) {
	return buildPrompt(cfg, artifacts, observations, scope, `Assessment requirements:
- Produce 1 to 7 concrete findings only when directly supported by the supplied evidence.
- Prefer high-signal observations such as exposed services, discovered web surfaces, exposed credentials, successful access, and privilege escalation over generic screenshot summaries.
- Separate materially different issues into distinct findings when the evidence shows an attack chain.
- Keep findings analyst-readable and specific enough to review without re-reading the full OCR text.`, `Return JSON with:
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

func buildInjectPrompt(cfg core.Config, artifacts []core.Artifact, observations []core.Observation, scope string) (string, []string) {
	return buildPrompt(cfg, artifacts, observations, scope, `Inject requirements:
- Produce 1 to 8 evidence-linked inject items only when directly supported by the supplied evidence.
- Prefer realistic injects tied to concrete technical events, user actions, or detection opportunities shown in the evidence.
- Keep injects operational and reviewable by an exercise lead; avoid fiction, unsupported adversary detail, or unsupported assumptions.`, `Return JSON with:
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

func buildPrompt(cfg core.Config, artifacts []core.Artifact, observations []core.Observation, scope, guidance, schema string) (string, []string) {
	summarizedArtifacts, summarizedObservations, warnings := summarizePromptInputs(cfg.AI, artifacts, observations)
	artifactSummary, _ := json.Marshal(summarizedArtifacts)
	observationSummary, _ := json.Marshal(summarizedObservations)
	sections := []string{
		"Mode: " + string(cfg.Mode),
		"Title: " + cfg.Title,
		"Client: " + cfg.Client,
		"Environment: " + cfg.Environment,
		"Classification: " + cfg.Classification,
		"Instructions: " + cfg.Instructions,
		guidance,
		"Evidence Highlights:\n" + formatEvidenceHighlights(summarizedObservations),
		"Artifacts JSON: " + string(artifactSummary),
		"Observations JSON: " + string(observationSummary),
		"Each finding or inject item must cite evidence_refs and observation_ids. Use the supplied observations and artifacts only.",
		schema,
	}
	if strings.TrimSpace(scope) != "" {
		sections = append([]string{"Scope: " + scope}, sections...)
	}
	return strings.Join(sections, "\n\n"), warnings
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

func summarizePromptInputs(ai core.AIConfig, artifacts []core.Artifact, observations []core.Observation) ([]core.Artifact, []core.Observation, []string) {
	var warnings []string

	artifactLimit := ai.MaxPromptArtifacts
	if artifactLimit <= 0 {
		artifactLimit = len(artifacts)
	}
	observationLimit := ai.MaxPromptObservations
	if observationLimit <= 0 {
		observationLimit = len(observations)
	}

	summarizedArtifacts := artifacts
	if len(summarizedArtifacts) > artifactLimit {
		summarizedArtifacts = append([]core.Artifact{}, artifacts[:artifactLimit]...)
		warnings = append(warnings, fmt.Sprintf("artifacts truncated for prompt context: %d of %d included", artifactLimit, len(artifacts)))
	}
	for i := range summarizedArtifacts {
		summarizedArtifacts[i].ExtractedText = truncateForPrompt(summarizedArtifacts[i].ExtractedText, 1200)
		if len(summarizedArtifacts[i].Metadata) > 0 {
			trimmed := make(map[string]string, len(summarizedArtifacts[i].Metadata))
			for key, value := range summarizedArtifacts[i].Metadata {
				trimmed[key] = truncateForPrompt(value, 200)
			}
			summarizedArtifacts[i].Metadata = trimmed
		}
	}

	summarizedObservations := observations
	if len(summarizedObservations) > observationLimit {
		summarizedObservations = append([]core.Observation{}, observations[:observationLimit]...)
		warnings = append(warnings, fmt.Sprintf("observations truncated for prompt context: %d of %d included", observationLimit, len(observations)))
	}
	for i := range summarizedObservations {
		summarizedObservations[i].Detail = truncateForPrompt(summarizedObservations[i].Detail, 600)
		for j := range summarizedObservations[i].Evidence {
			summarizedObservations[i].Evidence[j].Snippet = truncateForPrompt(summarizedObservations[i].Evidence[j].Snippet, 240)
			summarizedObservations[i].Evidence[j].Description = truncateForPrompt(summarizedObservations[i].Evidence[j].Description, 120)
		}
	}

	return summarizedArtifacts, summarizedObservations, warnings
}

func planPromptBatches(ai core.AIConfig, artifacts []core.Artifact, observations []core.Observation) ([]promptBatch, []string) {
	var warnings []string

	artifactLimit := ai.MaxPromptArtifacts
	if artifactLimit <= 0 {
		if len(artifacts) == 0 {
			artifactLimit = 1
		} else {
			artifactLimit = len(artifacts)
		}
	}
	observationLimit := ai.MaxPromptObservations
	if observationLimit <= 0 {
		if len(observations) == 0 {
			observationLimit = 1
		} else {
			observationLimit = len(observations)
		}
	}

	if len(observations) == 0 {
		if len(artifacts) == 0 {
			return []promptBatch{{Label: "full evidence set"}}, nil
		}
		if len(artifacts) <= artifactLimit {
			return []promptBatch{{
				Label:     "full evidence set",
				Artifacts: append([]core.Artifact{}, artifacts...),
			}}, nil
		}
		var batches []promptBatch
		for start := 0; start < len(artifacts); start += artifactLimit {
			end := min(start+artifactLimit, len(artifacts))
			batches = append(batches, promptBatch{
				Label:     fmt.Sprintf("artifact batch %d of %d", len(batches)+1, (len(artifacts)+artifactLimit-1)/artifactLimit),
				Artifacts: append([]core.Artifact{}, artifacts[start:end]...),
			})
		}
		warnings = append(warnings, fmt.Sprintf("artifacts exceeded prompt window; split synthesis across %d batches", len(batches)))
		return batches, warnings
	}

	if len(observations) <= observationLimit && len(artifacts) <= artifactLimit {
		return []promptBatch{{
			Label:        "full evidence set",
			Artifacts:    append([]core.Artifact{}, artifacts...),
			Observations: append([]core.Observation{}, observations...),
		}}, nil
	}

	totalBatches := (len(observations) + observationLimit - 1) / observationLimit
	batches := make([]promptBatch, 0, totalBatches)
	artifactsByID := make(map[string]core.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		artifactsByID[artifact.ID] = artifact
	}

	for start := 0; start < len(observations); start += observationLimit {
		end := min(start+observationLimit, len(observations))
		chunkObservations := append([]core.Observation{}, observations[start:end]...)
		selectedArtifacts := selectBatchArtifacts(chunkObservations, artifacts, artifactsByID, artifactLimit)
		if len(selectedArtifacts) == artifactLimit && len(linkedArtifactIDs(chunkObservations)) > artifactLimit {
			warnings = append(warnings, fmt.Sprintf("linked artifacts truncated for observation batch %d of %d", len(batches)+1, totalBatches))
		}
		batches = append(batches, promptBatch{
			Label:        fmt.Sprintf("evidence batch %d of %d", len(batches)+1, totalBatches),
			Artifacts:    selectedArtifacts,
			Observations: chunkObservations,
		})
	}

	warnings = append(warnings, fmt.Sprintf("observations exceeded prompt window; split synthesis across %d batches", len(batches)))
	return batches, warnings
}

func selectBatchArtifacts(observations []core.Observation, artifacts []core.Artifact, artifactsByID map[string]core.Artifact, artifactLimit int) []core.Artifact {
	linked := linkedArtifactIDs(observations)
	selected := make([]core.Artifact, 0, min(len(linked), artifactLimit))
	if len(linked) > 0 {
		for _, artifact := range artifacts {
			if !linked[artifact.ID] {
				continue
			}
			selected = append(selected, artifactsByID[artifact.ID])
			if len(selected) >= artifactLimit {
				return selected
			}
		}
	}

	for _, artifact := range artifacts {
		if len(selected) >= artifactLimit {
			break
		}
		if linked[artifact.ID] || containsArtifact(selected, artifact.ID) {
			continue
		}
		selected = append(selected, artifact)
	}
	return selected
}

func linkedArtifactIDs(observations []core.Observation) map[string]bool {
	linked := make(map[string]bool)
	for _, observation := range observations {
		for _, evidence := range observation.Evidence {
			if evidence.ArtifactID != "" {
				linked[evidence.ArtifactID] = true
			}
		}
	}
	return linked
}

func containsArtifact(artifacts []core.Artifact, id string) bool {
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return true
		}
	}
	return false
}

func (c *Client) reduceAssessmentDrafts(ctx context.Context, system string, cfg core.Config, partials []core.AssessmentDraft) (core.AssessmentDraft, string, []string, error) {
	prompt := buildAssessmentMergePrompt(cfg, partials)
	var merged core.AssessmentDraft
	warnings, usedModel, err := c.generateStructured(ctx, system, prompt, nil, "assessment reduction", &merged, func() []string {
		return validateAssessmentDraft(merged)
	})
	if err != nil {
		warnings = append(warnings, "assessment reduction failed; using deterministic merge of chunk drafts")
		return mergeAssessmentDrafts(partials), usedModel, warnings, nil
	}
	if len(merged.Findings) == 0 {
		warnings = append(warnings, "assessment reduction returned no findings; using deterministic merge of chunk drafts")
		return mergeAssessmentDrafts(partials), usedModel, warnings, nil
	}
	return merged, usedModel, warnings, nil
}

func (c *Client) reduceInjectDrafts(ctx context.Context, system string, cfg core.Config, partials []core.InjectDraft) (core.InjectDraft, string, []string, error) {
	prompt := buildInjectMergePrompt(cfg, partials)
	var merged core.InjectDraft
	warnings, usedModel, err := c.generateStructured(ctx, system, prompt, nil, "inject reduction", &merged, func() []string {
		return validateInjectDraft(merged)
	})
	if err != nil {
		warnings = append(warnings, "inject reduction failed; using deterministic merge of chunk drafts")
		return mergeInjectDrafts(partials), usedModel, warnings, nil
	}
	if len(merged.Items) == 0 {
		warnings = append(warnings, "inject reduction returned no items; using deterministic merge of chunk drafts")
		return mergeInjectDrafts(partials), usedModel, warnings, nil
	}
	return merged, usedModel, warnings, nil
}

func buildAssessmentMergePrompt(cfg core.Config, partials []core.AssessmentDraft) string {
	summary, _ := json.Marshal(partials)
	return strings.Join([]string{
		"Mode: " + string(cfg.Mode),
		"Title: " + cfg.Title,
		"Client: " + cfg.Client,
		"Environment: " + cfg.Environment,
		"Classification: " + cfg.Classification,
		"Instructions: " + cfg.Instructions,
		"Consolidate the following partial assessment drafts into one final report draft.",
		"Deduplicate overlapping findings, preserve evidence_refs and observation_ids, and do not invent unsupported items.",
		"Partial Drafts JSON: " + string(summary),
		`Return JSON with:
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
}`,
	}, "\n\n")
}

func buildInjectMergePrompt(cfg core.Config, partials []core.InjectDraft) string {
	summary, _ := json.Marshal(partials)
	return strings.Join([]string{
		"Mode: " + string(cfg.Mode),
		"Title: " + cfg.Title,
		"Client: " + cfg.Client,
		"Environment: " + cfg.Environment,
		"Classification: " + cfg.Classification,
		"Instructions: " + cfg.Instructions,
		"Consolidate the following partial inject drafts into one final inject pack.",
		"Deduplicate overlapping inject items, preserve evidence_refs and observation_ids, and do not invent unsupported items.",
		"Partial Drafts JSON: " + string(summary),
		`Return JSON with:
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
}`,
	}, "\n\n")
}

func mergeAssessmentDrafts(partials []core.AssessmentDraft) core.AssessmentDraft {
	merged := core.AssessmentDraft{
		ExecutiveSummary: mergeTextSummaries(func() []string {
			summaries := make([]string, 0, len(partials))
			for _, partial := range partials {
				summaries = append(summaries, partial.ExecutiveSummary)
			}
			return summaries
		}()),
	}

	indexByKey := make(map[string]int)
	for _, partial := range partials {
		for _, finding := range partial.Findings {
			key := strings.ToLower(strings.TrimSpace(finding.Title)) + "|" + strings.ToLower(strings.TrimSpace(finding.Severity))
			if idx, ok := indexByKey[key]; ok {
				merged.Findings[idx] = mergeFinding(merged.Findings[idx], finding)
				continue
			}
			indexByKey[key] = len(merged.Findings)
			merged.Findings = append(merged.Findings, finding)
		}
	}
	return merged
}

func mergeInjectDrafts(partials []core.InjectDraft) core.InjectDraft {
	merged := core.InjectDraft{
		ScenarioSummary: mergeTextSummaries(func() []string {
			summaries := make([]string, 0, len(partials))
			for _, partial := range partials {
				summaries = append(summaries, partial.ScenarioSummary)
			}
			return summaries
		}()),
	}

	indexByKey := make(map[string]int)
	for _, partial := range partials {
		for _, item := range partial.Items {
			key := strings.ToLower(strings.TrimSpace(item.Title)) + "|" + strings.ToLower(strings.TrimSpace(item.Audience)) + "|" + strings.ToLower(strings.TrimSpace(item.Trigger))
			if idx, ok := indexByKey[key]; ok {
				merged.Items[idx] = mergeInjectItem(merged.Items[idx], item)
				continue
			}
			indexByKey[key] = len(merged.Items)
			merged.Items = append(merged.Items, item)
		}
	}
	return merged
}

func mergeTextSummaries(parts []string) string {
	seen := make(map[string]bool)
	unique := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, trimmed)
	}
	return strings.Join(unique, " ")
}

func mergeFinding(left, right core.Finding) core.Finding {
	if left.ID == "" {
		left.ID = right.ID
	}
	if left.Title == "" {
		left.Title = right.Title
	}
	if left.Severity == "" {
		left.Severity = right.Severity
	}
	if left.Description == "" {
		left.Description = right.Description
	}
	if left.Impact == "" {
		left.Impact = right.Impact
	}
	if left.Remediation == "" {
		left.Remediation = right.Remediation
	}
	left.EvidenceRefs = mergeUniqueStrings(left.EvidenceRefs, right.EvidenceRefs)
	left.ObservationIDs = mergeUniqueStrings(left.ObservationIDs, right.ObservationIDs)
	return left
}

func mergeInjectItem(left, right core.InjectItem) core.InjectItem {
	if left.ID == "" {
		left.ID = right.ID
	}
	if left.Title == "" {
		left.Title = right.Title
	}
	if left.Audience == "" {
		left.Audience = right.Audience
	}
	if left.Channel == "" {
		left.Channel = right.Channel
	}
	if left.Trigger == "" {
		left.Trigger = right.Trigger
	}
	if left.ExpectedAction == "" {
		left.ExpectedAction = right.ExpectedAction
	}
	if left.Body == "" {
		left.Body = right.Body
	}
	left.EvidenceRefs = mergeUniqueStrings(left.EvidenceRefs, right.EvidenceRefs)
	left.ObservationIDs = mergeUniqueStrings(left.ObservationIDs, right.ObservationIDs)
	return left
}

func mergeUniqueStrings(left, right []string) []string {
	seen := make(map[string]bool, len(left)+len(right))
	merged := make([]string, 0, len(left)+len(right))
	for _, value := range append(append([]string{}, left...), right...) {
		if strings.TrimSpace(value) == "" || seen[value] {
			continue
		}
		seen[value] = true
		merged = append(merged, value)
	}
	return merged
}

func newSynthesisTrace(cfg core.AIConfig, batchCount int) synthesisTrace {
	return synthesisTrace{
		RequestedModel: cfg.Model,
		FallbackModel:  cfg.FallbackModel,
		BatchCount:     batchCount,
	}
}

func (t *synthesisTrace) addStage(name, model string) {
	if strings.TrimSpace(model) == "" {
		return
	}
	t.Stages = append(t.Stages, core.RunAIStage{
		Name:  name,
		Model: model,
	})
	if !containsString(t.ModelsUsed, model) {
		t.ModelsUsed = append(t.ModelsUsed, model)
	}
}

func (t synthesisTrace) toCore() core.RunAITrace {
	return core.RunAITrace{
		RequestedModel: t.RequestedModel,
		FallbackModel:  t.FallbackModel,
		ModelsUsed:     append([]string{}, t.ModelsUsed...),
		Stages:         append([]core.RunAIStage{}, t.Stages...),
		BatchCount:     t.BatchCount,
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

func truncateForPrompt(in string, max int) string {
	if len(in) <= max {
		return in
	}
	return strings.TrimSpace(in[:max]) + "..."
}

func buildValidationRepairPrompt(originalPrompt, priorResponse string, problems []string) string {
	return strings.Join([]string{
		"The previous response failed required output rules.",
		"Problems:",
		"- " + strings.Join(problems, "\n- "),
		"Re-run the same task and return corrected JSON only.",
		"Do not omit findings or items when the supplied evidence clearly supports them.",
		"Original task:",
		originalPrompt,
		"Previous response:",
		truncateForPrompt(priorResponse, 2400),
	}, "\n\n")
}

func validateAssessmentDraft(draft core.AssessmentDraft) []string {
	var problems []string
	if len(strings.Fields(draft.ExecutiveSummary)) < 2 {
		problems = append(problems, "executive_summary is missing or too short")
	}
	if len(draft.Findings) == 0 {
		problems = append(problems, "findings is empty")
		return problems
	}
	for i, finding := range draft.Findings {
		prefix := fmt.Sprintf("findings[%d]", i)
		if strings.TrimSpace(finding.Title) == "" {
			problems = append(problems, prefix+".title is empty")
		}
		if !isValidSeverity(finding.Severity) {
			problems = append(problems, prefix+".severity is invalid")
		}
		if len(strings.Fields(finding.Description)) < 2 {
			problems = append(problems, prefix+".description is too short")
		}
		if len(strings.Fields(finding.Impact)) < 1 {
			problems = append(problems, prefix+".impact is too short")
		}
		if len(strings.Fields(finding.Remediation)) < 1 {
			problems = append(problems, prefix+".remediation is too short")
		}
		if len(finding.EvidenceRefs) == 0 {
			problems = append(problems, prefix+".evidence_refs is empty")
		}
		if len(finding.ObservationIDs) == 0 {
			problems = append(problems, prefix+".observation_ids is empty")
		}
	}
	return problems
}

func validateInjectDraft(draft core.InjectDraft) []string {
	var problems []string
	if len(strings.Fields(draft.ScenarioSummary)) < 2 {
		problems = append(problems, "scenario_summary is missing or too short")
	}
	if len(draft.Items) == 0 {
		problems = append(problems, "items is empty")
		return problems
	}
	for i, item := range draft.Items {
		prefix := fmt.Sprintf("items[%d]", i)
		if strings.TrimSpace(item.Title) == "" {
			problems = append(problems, prefix+".title is empty")
		}
		if strings.TrimSpace(item.Audience) == "" {
			problems = append(problems, prefix+".audience is empty")
		}
		if strings.TrimSpace(item.Channel) == "" {
			problems = append(problems, prefix+".channel is empty")
		}
		if len(strings.Fields(item.Trigger)) < 1 {
			problems = append(problems, prefix+".trigger is too short")
		}
		if len(strings.Fields(item.ExpectedAction)) < 2 {
			problems = append(problems, prefix+".expected_action is too short")
		}
		if len(strings.Fields(item.Body)) < 2 {
			problems = append(problems, prefix+".body is too short")
		}
		if len(item.EvidenceRefs) == 0 {
			problems = append(problems, prefix+".evidence_refs is empty")
		}
		if len(item.ObservationIDs) == 0 {
			problems = append(problems, prefix+".observation_ids is empty")
		}
	}
	return problems
}

func isValidSeverity(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high", "medium", "low", "info":
		return true
	default:
		return false
	}
}

func formatEvidenceHighlights(observations []core.Observation) string {
	if len(observations) == 0 {
		return "None"
	}

	ranked := append([]core.Observation{}, observations...)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := promptObservationScore(ranked[i])
		right := promptObservationScore(ranked[j])
		if left != right {
			return left > right
		}
		if ranked[i].Confidence != ranked[j].Confidence {
			return ranked[i].Confidence > ranked[j].Confidence
		}
		return ranked[i].Title < ranked[j].Title
	})

	limit := min(len(ranked), 10)
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		observation := ranked[i]
		line := fmt.Sprintf("%d. [%s/%s] %s :: %s", i+1, blankAs(observation.Category, "uncategorized"), blankAs(observation.Severity, "unspecified"), observation.Title, truncateForPrompt(strings.TrimSpace(observation.Detail), 180))
		if len(observation.Evidence) > 0 && strings.TrimSpace(observation.Evidence[0].Snippet) != "" {
			line += " | Evidence: " + truncateForPrompt(strings.TrimSpace(observation.Evidence[0].Snippet), 140)
		}
		if observation.ID != "" {
			line += " | Observation ID: " + observation.ID
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func promptObservationScore(observation core.Observation) int {
	score := 0
	switch strings.ToLower(strings.TrimSpace(observation.Category)) {
	case "privilege":
		score += 500
	case "access":
		score += 400
	case "credential":
		score += 350
	case "web":
		score += 300
	case "nessus", "nmap":
		score += 250
	case "text":
		score += 200
	case "image":
		score += 50
	}
	switch strings.ToLower(strings.TrimSpace(observation.Severity)) {
	case "critical":
		score += 50
	case "high":
		score += 40
	case "medium":
		score += 30
	case "low":
		score += 20
	case "info":
		score += 10
	}
	score += int(observation.Confidence * 10)
	return score
}

func blankAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
