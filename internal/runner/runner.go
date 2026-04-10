package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"injectctl/internal/ai/ollama"
	"injectctl/internal/assess"
	"injectctl/internal/core"
	"injectctl/internal/evidence"
	"injectctl/internal/ingest"
	"injectctl/internal/inject"
	"injectctl/internal/normalize"
	"injectctl/internal/ocr"
	"injectctl/internal/project"
	evidencerender "injectctl/internal/render/evidence"
	jsonrender "injectctl/internal/render/json"
	markdownrender "injectctl/internal/render/markdown"
	pdfrender "injectctl/internal/render/pdf"
)

type Options struct {
	Config       core.Config
	ManifestPath string
	InputPaths   []string
	OutputDir    string
}

func Run(ctx context.Context, opts Options) error {
	started := time.Now().UTC()
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	ocrEngine := ocr.New()
	artifacts, observations, warnings, err := ingest.Collect(ctx, opts.Config, opts.InputPaths, ocrEngine)
	if err != nil {
		return err
	}
	observations = normalize.Observations(observations)

	client := ollama.New(opts.Config.AI)

	runRecord := core.RunRecord{
		RunID:     fmt.Sprintf("%d", started.UnixNano()),
		Mode:      opts.Config.Mode,
		Title:     opts.Config.Title,
		StartedAt: started,
		Model:     opts.Config.AI.Model,
		Warnings:  append([]string{}, warnings...),
	}

	switch opts.Config.Mode {
	case core.ModeAssess:
		result, err := assess.Build(ctx, client, opts.Config, artifacts, observations, &runRecord)
		if err != nil {
			return err
		}
		result.Run.CompletedAt = time.Now().UTC()
		result.Run.Duration = result.Run.CompletedAt.Sub(started)
		sanitizeAssessment(result)
		index := evidence.Build(result.Run, result.Artifacts, result.Observations)
		if err := writeOutputs(opts.OutputDir, opts.Config.Output.Formats, result, nil, index, opts.Config.Template); err != nil {
			return err
		}
		return persistProject(opts, result, nil, index)
	case core.ModeInject:
		result, err := inject.Build(ctx, client, opts.Config, artifacts, observations, &runRecord)
		if err != nil {
			return err
		}
		result.Run.CompletedAt = time.Now().UTC()
		result.Run.Duration = result.Run.CompletedAt.Sub(started)
		sanitizeInject(result)
		index := evidence.Build(result.Run, result.Artifacts, result.Observations)
		if err := writeOutputs(opts.OutputDir, opts.Config.Output.Formats, nil, result, index, opts.Config.Template); err != nil {
			return err
		}
		return persistProject(opts, nil, result, index)
	default:
		return errors.New("unsupported mode")
	}
}

func writeOutputs(outDir string, formats []string, assessResult *core.AssessmentResult, injectResult *core.InjectResult, index core.EvidenceIndex, templatePath string) error {
	for _, format := range formats {
		switch strings.ToLower(format) {
		case "json":
			if assessResult != nil {
				if err := jsonrender.Write(filepath.Join(outDir, "assessment.json"), assessResult); err != nil {
					return err
				}
				continue
			}
			if err := jsonrender.Write(filepath.Join(outDir, "inject.json"), injectResult); err != nil {
				return err
			}
		case "markdown":
			if assessResult != nil {
				if err := markdownrender.WriteAssessment(filepath.Join(outDir, "assessment.md"), assessResult, templatePath); err != nil {
					return err
				}
				continue
			}
			if err := markdownrender.WriteInject(filepath.Join(outDir, "inject.md"), injectResult, templatePath); err != nil {
				return err
			}
		case "pdf":
			if assessResult != nil {
				if err := pdfrender.WriteAssessment(filepath.Join(outDir, "assessment.pdf"), assessResult); err != nil {
					return err
				}
				continue
			}
			if err := pdfrender.WriteInject(filepath.Join(outDir, "inject.pdf"), injectResult); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported output format: %s", format)
		}
	}
	if err := evidencerender.WriteJSON(filepath.Join(outDir, "evidence-index.json"), index); err != nil {
		return err
	}
	if err := evidencerender.WriteMarkdown(filepath.Join(outDir, "evidence-index.md"), index); err != nil {
		return err
	}
	return nil
}

func persistProject(opts Options, assessResult *core.AssessmentResult, injectResult *core.InjectResult, index core.EvidenceIndex) error {
	if opts.Config.Output.ProjectDir == "" {
		return nil
	}
	if err := os.MkdirAll(opts.Config.Output.ProjectDir, 0o755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}
	if assessResult != nil {
		if err := project.WriteAssessment(opts.Config.Output.ProjectDir, opts.ManifestPath, *assessResult); err != nil {
			return err
		}
		if err := project.WriteEvidenceIndex(opts.Config.Output.ProjectDir, index); err != nil {
			return err
		}
		return project.WriteRunSummary(opts.Config.Output.ProjectDir, project.RunSummary{
			RunID:            assessResult.Run.RunID,
			Mode:             string(assessResult.Run.Mode),
			Title:            assessResult.Run.Title,
			Status:           assessResult.Status,
			Model:            assessResult.Run.Model,
			ArtifactCount:    len(assessResult.Artifacts),
			ObservationCount: len(assessResult.Observations),
			Warnings:         assessResult.Run.Warnings,
			Errors:           assessResult.Run.Errors,
			OutputFiles:      outputFiles(opts.Config.Mode, opts.Config.Output.Formats),
		})
	}
	if err := project.WriteInject(opts.Config.Output.ProjectDir, opts.ManifestPath, *injectResult); err != nil {
		return err
	}
	if err := project.WriteEvidenceIndex(opts.Config.Output.ProjectDir, index); err != nil {
		return err
	}
	return project.WriteRunSummary(opts.Config.Output.ProjectDir, project.RunSummary{
		RunID:            injectResult.Run.RunID,
		Mode:             string(injectResult.Run.Mode),
		Title:            injectResult.Run.Title,
		Status:           injectResult.Status,
		Model:            injectResult.Run.Model,
		ArtifactCount:    len(injectResult.Artifacts),
		ObservationCount: len(injectResult.Observations),
		Warnings:         injectResult.Run.Warnings,
		Errors:           injectResult.Run.Errors,
		OutputFiles:      outputFiles(opts.Config.Mode, opts.Config.Output.Formats),
	})
}

func outputFiles(mode core.Mode, formats []string) []string {
	files := []string{"evidence-index.json", "evidence-index.md"}
	for _, format := range formats {
		switch strings.ToLower(format) {
		case "json":
			files = append(files, string(mode)+".json")
		case "markdown":
			files = append(files, string(mode)+".md")
		case "pdf":
			files = append(files, string(mode)+".pdf")
		}
	}
	return files
}

func sanitizeAssessment(result *core.AssessmentResult) {
	if len(result.Config.Output.RedactionRules) == 0 {
		return
	}
	result.Run.Warnings = append(result.Run.Warnings, normalize.RedactAssessmentResult(result, result.Config.Output.RedactionRules)...)
}

func sanitizeInject(result *core.InjectResult) {
	if len(result.Config.Output.RedactionRules) == 0 {
		return
	}
	result.Run.Warnings = append(result.Run.Warnings, normalize.RedactInjectResult(result, result.Config.Output.RedactionRules)...)
}
