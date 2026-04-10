package image

import (
	"context"
	"strings"

	"injectctl/internal/core"
	"injectctl/internal/ocr"
)

func Parse(ctx context.Context, artifact core.Artifact, engine ocr.Engine) (core.Artifact, []core.Observation, []string, error) {
	artifact.Parser = "image"
	if engine == nil || !engine.Available() {
		return artifact, nil, []string{"tesseract not available; continuing without OCR"}, nil
	}

	text, err := engine.ExtractText(ctx, artifact.Path)
	if err != nil {
		return artifact, nil, []string{"image OCR failed; continuing with multimodal synthesis only"}, nil
	}
	artifact.ExtractedText = truncate(text, 10000)
	if strings.TrimSpace(text) == "" {
		return artifact, nil, nil, nil
	}

	observation := core.Observation{
		Title:      "Screenshot text extracted",
		Detail:     truncate(text, 600),
		Category:   "image",
		Confidence: 0.82,
		Evidence: []core.EvidenceExcerpt{
			{
				ArtifactID:  artifact.ID,
				Snippet:     truncate(text, 350),
				Location:    artifact.Path,
				Confidence:  0.82,
				Description: "OCR text extracted from screenshot",
			},
		},
	}
	return artifact, []core.Observation{observation}, nil, nil
}

func truncate(in string, max int) string {
	if len(in) <= max {
		return in
	}
	return strings.TrimSpace(in[:max]) + "..."
}
