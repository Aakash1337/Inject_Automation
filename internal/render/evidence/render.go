package evidence

import (
	"os"
	"strings"

	"injectctl/internal/core"
)

func WriteJSON(path string, index core.EvidenceIndex) error {
	return writeJSON(path, index)
}

func WriteMarkdown(path string, index core.EvidenceIndex) error {
	var builder strings.Builder
	builder.WriteString("# Evidence Index\n\n")
	builder.WriteString("Run ID: " + index.RunID + "\n")
	builder.WriteString("Mode: " + string(index.Mode) + "\n\n")
	if len(index.Entries) == 0 {
		builder.WriteString("No evidence entries were captured.\n")
		return os.WriteFile(path, []byte(builder.String()), 0o644)
	}

	for _, entry := range index.Entries {
		builder.WriteString("## " + entry.ArtifactID + "\n\n")
		builder.WriteString("- Path: " + entry.Path + "\n")
		builder.WriteString("- Kind: " + entry.Kind + "\n")
		builder.WriteString("- Parser: " + entry.Parser + "\n")
		builder.WriteString("- MIME: " + entry.MIME + "\n")
		builder.WriteString("- SHA256: " + entry.SHA256 + "\n")
		if len(entry.ObservationIDs) > 0 {
			builder.WriteString("- Observations: " + strings.Join(entry.ObservationIDs, ", ") + "\n")
		}
		if len(entry.Snippets) > 0 {
			builder.WriteString("- Snippets:\n")
			for _, snippet := range entry.Snippets {
				builder.WriteString("  - " + strings.ReplaceAll(snippet, "\n", " ") + "\n")
			}
		}
		builder.WriteString("\n")
	}

	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func writeJSON(path string, payload any) error {
	data, err := marshalJSON(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
