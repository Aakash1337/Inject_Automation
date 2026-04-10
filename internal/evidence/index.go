package evidence

import "injectctl/internal/core"

func Build(run core.RunRecord, artifacts []core.Artifact, observations []core.Observation) core.EvidenceIndex {
	byArtifact := make(map[string]*core.EvidenceIndexEntry, len(artifacts))
	index := core.EvidenceIndex{
		RunID: run.RunID,
		Mode:  run.Mode,
	}

	for _, artifact := range artifacts {
		entry := core.EvidenceIndexEntry{
			ArtifactID: artifact.ID,
			Path:       artifact.Path,
			Kind:       string(artifact.Kind),
			Parser:     artifact.Parser,
			MIME:       artifact.MIME,
			SHA256:     artifact.SHA256,
		}
		index.Entries = append(index.Entries, entry)
		byArtifact[artifact.ID] = &index.Entries[len(index.Entries)-1]
	}

	for _, observation := range observations {
		for _, excerpt := range observation.Evidence {
			entry, ok := byArtifact[excerpt.ArtifactID]
			if !ok {
				continue
			}
			if !contains(entry.ObservationIDs, observation.ID) {
				entry.ObservationIDs = append(entry.ObservationIDs, observation.ID)
			}
			if excerpt.Snippet != "" && !contains(entry.Snippets, excerpt.Snippet) {
				entry.Snippets = append(entry.Snippets, excerpt.Snippet)
			}
		}
	}

	return index
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
