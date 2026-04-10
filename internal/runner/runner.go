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
	"injectctl/internal/ingest"
	"injectctl/internal/inject"
	jsonrender "injectctl/internal/render/json"
	markdownrender "injectctl/internal/render/markdown"
	pdfrender "injectctl/internal/render/pdf"
	"injectctl/internal/project"
	"injectctl/internal/normalize"
	"injectctl/internal/ocr"
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
		if err := writeOutputs(opts.OutputDir, opts.Config.Output.Formats, result, nil, opts.Config.Template); err != nil {
			return err
		}
		return persistProject(opts, result, nil)
	case core.ModeInject:
		result, err := inject.Build(ctx, client, opts.Config, artifacts, observations, &runRecord)
		if err != nil {
			return err
		}
		result.Run.CompletedAt = time.Now().UTC()
		result.Run.Duration = result.Run.CompletedAt.Sub(started)
		sanitizeInject(result)
		if err := writeOutputs(opts.OutputDir, opts.Config.Output.Formats, nil, result, opts.Config.Template); err != nil {
			return err
		}
		return persistProject(opts, nil, result)
	default:
		return errors.New("unsupported mode")
	}
}

func writeOutputs(outDir string, formats []string, assessResult *core.AssessmentResult, injectResult *core.InjectResult, templatePath string) error {
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
	return nil
}

func persistProject(opts Options, assessResult *core.AssessmentResult, injectResult *core.InjectResult) error {
	if opts.Config.Output.ProjectDir == "" {
		return nil
	}
	if err := os.MkdirAll(opts.Config.Output.ProjectDir, 0o755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}
	if assessResult != nil {
		return project.WriteAssessment(opts.Config.Output.ProjectDir, opts.ManifestPath, *assessResult)
	}
	return project.WriteInject(opts.Config.Output.ProjectDir, opts.ManifestPath, *injectResult)
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
