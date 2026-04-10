package text

import (
	"os"
	"strings"

	"injectctl/internal/core"
)

func Parse(artifact core.Artifact) (core.Artifact, []core.Observation, error) {
	data, err := os.ReadFile(artifact.Path)
	if err != nil {
		return artifact, nil, err
	}

	content := strings.TrimSpace(string(data))
	artifact.Parser = "text"
	artifact.ExtractedText = truncate(content, 12000)

	if content == "" {
		return artifact, nil, nil
	}

	observation := core.Observation{
		Title:      "Text evidence ingested",
		Detail:     truncate(content, 700),
		Category:   "text",
		Confidence: 0.92,
		Evidence: []core.EvidenceExcerpt{
			{
				ArtifactID:  artifact.ID,
				Snippet:     truncate(content, 350),
				Location:    artifact.Path,
				Confidence:  0.92,
				Description: "Freeform note or text artifact",
			},
		},
	}

	return artifact, []core.Observation{observation}, nil
}

func truncate(in string, max int) string {
	if len(in) <= max {
		return in
	}
	return strings.TrimSpace(in[:max]) + "..."
}
