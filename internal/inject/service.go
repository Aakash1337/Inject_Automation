package inject

import (
	"context"
	"fmt"
	"time"

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
		run.Errors = append(run.Errors, "inject synthesis failed; evidence-only error report emitted")
		return &core.InjectResult{
			Run:          *run,
			Status:       "evidence_only",
			Config:       cfg,
			Artifacts:    artifacts,
			Observations: observations,
			Draft:        core.InjectDraft{},
			ErrorReport: &core.ErrorReport{
				Stage:       "synthesis",
				Message:     synthErr.Error(),
				GeneratedAt: time.Now().UTC(),
				Recommendations: []string{
					"Review the normalized evidence in the JSON output.",
					"Confirm Ollama model availability and rerun the job.",
					"Check prompt inputs and template constraints if the failure persists.",
				},
			},
		}, nil
	}

	return &core.InjectResult{
		Run:          *run,
		Status:       "draft_ready",
		Config:       cfg,
		Artifacts:    artifacts,
		Observations: observations,
		Draft:        draft,
	}, nil
}
