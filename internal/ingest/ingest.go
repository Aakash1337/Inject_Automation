package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"injectctl/internal/core"
	"injectctl/internal/ocr"
	imageparser "injectctl/internal/parsers/image"
	"injectctl/internal/parsers/nessus"
	"injectctl/internal/parsers/nmap"
	textparser "injectctl/internal/parsers/text"
)

func Collect(ctx context.Context, cfg core.Config, cliInputs []string, engine ocr.Engine) ([]core.Artifact, []core.Observation, []string, error) {
	paths, err := discover(cfg.Artifacts, cliInputs)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(paths) == 0 {
		return nil, nil, nil, errors.New("no input artifacts found")
	}

	var artifacts []core.Artifact
	var observations []core.Observation
	var warnings []string

	for _, path := range paths {
		artifact, err := baseArtifact(path)
		if err != nil {
			warnings = append(warnings, "failed to fingerprint "+path)
			continue
		}

		switch classify(path) {
		case core.ArtifactImage:
			artifact.Kind = core.ArtifactImage
			artifact, parsedObservations, parsedWarnings, err := imageparser.Parse(ctx, artifact, engine)
			if err != nil {
				warnings = append(warnings, "failed to parse image "+path)
			}
			warnings = append(warnings, parsedWarnings...)
			artifacts = append(artifacts, artifact)
			observations = append(observations, parsedObservations...)
		case core.ArtifactNmap:
			artifact.Kind = core.ArtifactNmap
			artifact, parsedObservations, err := nmap.Parse(artifact)
			if err != nil {
				warnings = append(warnings, "failed to parse nmap artifact "+path)
			}
			artifacts = append(artifacts, artifact)
			observations = append(observations, parsedObservations...)
		case core.ArtifactNessus:
			artifact.Kind = core.ArtifactNessus
			artifact, parsedObservations, err := nessus.Parse(artifact)
			if err != nil {
				warnings = append(warnings, "failed to parse nessus artifact "+path)
			}
			artifacts = append(artifacts, artifact)
			observations = append(observations, parsedObservations...)
		case core.ArtifactText:
			artifact.Kind = core.ArtifactText
			artifact, parsedObservations, err := textparser.Parse(artifact)
			if err != nil {
				warnings = append(warnings, "failed to parse text artifact "+path)
			}
			artifacts = append(artifacts, artifact)
			observations = append(observations, parsedObservations...)
		default:
			artifact.Kind = core.ArtifactOther
			artifact.Parser = "generic"
			artifacts = append(artifacts, artifact)
		}
	}

	return artifacts, observations, warnings, nil
}

func discover(manifestPaths, cliInputs []string) ([]string, error) {
	var roots []string
	roots = append(roots, manifestPaths...)
	roots = append(roots, cliInputs...)
	if len(roots) == 0 {
		return nil, nil
	}

	seen := map[string]struct{}{}
	var files []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			resolved, err := filepath.Abs(root)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[resolved]; !ok {
				seen[resolved] = struct{}{}
				files = append(files, resolved)
			}
			continue
		}

		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			resolved, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if _, ok := seen[resolved]; ok {
				return nil
			}
			seen[resolved] = struct{}{}
			files = append(files, resolved)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func baseArtifact(path string) (core.Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.Artifact{}, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	return core.Artifact{
		ID:     hash[:12],
		Path:   path,
		MIME:   mimeFor(path),
		SHA256: hash,
	}, nil
}

func classify(path string) core.ArtifactKind {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
		return core.ArtifactImage
	case ".nmap":
		return core.ArtifactNmap
	case ".nessus":
		return core.ArtifactNessus
	case ".txt", ".md", ".log":
		return core.ArtifactText
	case ".csv":
		if strings.Contains(strings.ToLower(filepath.Base(path)), "nessus") {
			return core.ArtifactNessus
		}
		return core.ArtifactText
	case ".xml":
		lower := strings.ToLower(filepath.Base(path))
		switch {
		case strings.Contains(lower, "nmap"):
			return core.ArtifactNmap
		case strings.Contains(lower, "nessus"):
			return core.ArtifactNessus
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return core.ArtifactOther
			}
			content := strings.ToLower(string(data))
			if strings.Contains(content, "<nmaprun") {
				return core.ArtifactNmap
			}
			if strings.Contains(content, "nessusclientdata_v2") || strings.Contains(content, "<reporthost") {
				return core.ArtifactNessus
			}
			return core.ArtifactText
		}
	default:
		return core.ArtifactOther
	}
}

func mimeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".txt", ".log", ".md", ".nmap":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".xml", ".nessus":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}
