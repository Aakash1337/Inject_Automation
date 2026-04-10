package assess

import (
	"context"
	"fmt"
	"strings"

	"injectctl/internal/ai/ollama"
	"injectctl/internal/core"
)

func Build(ctx context.Context, client *ollama.Client, cfg core.Config, artifacts []core.Artifact, observations []core.Observation, run *core.RunRecord) (*core.AssessmentResult, error) {
	reachable, err := client.Health(ctx)
	if err != nil || !reachable {
		return nil, fmt.Errorf("ollama is not reachable at %s", cfg.AI.Endpoint)
	}

	draft, warnings, synthErr := client.SynthesizeAssessment(ctx, cfg, artifacts, observations)
	run.Warnings = append(run.Warnings, warnings...)
	if synthErr != nil {
		run.Errors = append(run.Errors, "assessment synthesis failed; heuristic draft emitted")
		draft = heuristicDraft(observations)
	}

	return &core.AssessmentResult{
		Run:          *run,
		Config:       cfg,
		Artifacts:    artifacts,
		Observations: observations,
		Draft:        draft,
	}, nil
}

func heuristicDraft(observations []core.Observation) core.AssessmentDraft {
	draft := core.AssessmentDraft{
		ExecutiveSummary: "AI synthesis failed validation. Review the evidence-backed findings below before distribution.",
	}
	for i, observation := range observations {
		severity := observation.Severity
		if severity == "" {
			severity = "info"
		}
		var evidenceRefs []string
		for _, evidence := range observation.Evidence {
			evidenceRefs = append(evidenceRefs, evidence.ArtifactID+":"+strings.TrimSpace(evidence.Description))
		}
		draft.Findings = append(draft.Findings, core.Finding{
			ID:             fmt.Sprintf("heuristic-finding-%d", i+1),
			Title:          observation.Title,
			Severity:       severity,
			Description:    observation.Detail,
			Impact:         "Analyst review required to validate impact.",
			Remediation:    "Investigate and confirm the observation against source evidence.",
			EvidenceRefs:   evidenceRefs,
			ObservationIDs: []string{observation.ID},
		})
	}
	return draft
}
