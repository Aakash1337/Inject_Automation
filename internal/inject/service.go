package inject

import (
	"context"
	"fmt"
	"strings"

	"injectctl/internal/ai/ollama"
	"injectctl/internal/core"
)

func Build(ctx context.Context, client *ollama.Client, cfg core.Config, artifacts []core.Artifact, observations []core.Observation, run *core.RunRecord) (*core.InjectResult, error) {
	reachable, err := client.Health(ctx)
	if err != nil || !reachable {
		return nil, fmt.Errorf("ollama is not reachable at %s", cfg.AI.Endpoint)
	}

	draft, warnings, synthErr := client.SynthesizeInject(ctx, cfg, artifacts, observations)
	run.Warnings = append(run.Warnings, warnings...)
	if synthErr != nil {
		run.Errors = append(run.Errors, "inject synthesis failed; heuristic draft emitted")
		draft = heuristicDraft(observations)
	}

	return &core.InjectResult{
		Run:          *run,
		Config:       cfg,
		Artifacts:    artifacts,
		Observations: observations,
		Draft:        draft,
	}, nil
}

func heuristicDraft(observations []core.Observation) core.InjectDraft {
	draft := core.InjectDraft{
		ScenarioSummary: "AI synthesis failed validation. Review the evidence-backed injects below before use in an exercise.",
	}
	for i, observation := range observations {
		var evidenceRefs []string
		for _, evidence := range observation.Evidence {
			evidenceRefs = append(evidenceRefs, evidence.ArtifactID+":"+strings.TrimSpace(evidence.Description))
		}
		draft.Items = append(draft.Items, core.InjectItem{
			ID:             fmt.Sprintf("heuristic-inject-%d", i+1),
			Title:          observation.Title,
			Audience:       "Blue Team",
			Channel:        "Email",
			Trigger:        "Use when the related evidence appears in the scenario timeline.",
			ExpectedAction: "Investigate the evidence and document containment or escalation steps.",
			Body:           observation.Detail,
			EvidenceRefs:   evidenceRefs,
			ObservationIDs: []string{observation.ID},
		})
	}
	return draft
}
