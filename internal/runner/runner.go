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

func Run(ctx context.Context, opts Options) (project.RunSummary, error) {
	started := time.Now().UTC()
	runLog := []project.RunLogEntry{
		logEntry(started, "info", "run_started", "Run started", map[string]string{
			"mode":       string(opts.Config.Mode),
			"output_dir": opts.OutputDir,
		}),
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return project.RunSummary{}, fmt.Errorf("create output directory: %w", err)
	}

	ocrEngine := ocr.New()
	artifacts, observations, warnings, err := ingest.Collect(ctx, opts.Config, opts.InputPaths, ocrEngine)
	if err != nil {
		return project.RunSummary{}, err
	}
	runLog = append(runLog, logEntry(time.Now().UTC(), "info", "artifacts_collected", "Artifacts collected and parsed", map[string]string{
		"artifact_count":    fmt.Sprintf("%d", len(artifacts)),
		"observation_count": fmt.Sprintf("%d", len(observations)),
	}))
	observations = normalize.Observations(observations)
	warnings = append(warnings, normalize.ConflictWarnings(observations)...)
	runLog = append(runLog, logEntry(time.Now().UTC(), "info", "observations_normalized", "Observations normalized", map[string]string{
		"observation_count": fmt.Sprintf("%d", len(observations)),
	}))

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
			return project.RunSummary{}, err
		}
		result.Run.CompletedAt = time.Now().UTC()
		result.Run.Duration = result.Run.CompletedAt.Sub(started)
		selectedTemplate, templateWarnings, err := markdownrender.ResolveTemplate(core.ModeAssess, opts.Config.Template, opts.Config.TemplateDir, opts.Config, result.Observations)
		if err != nil {
			return project.RunSummary{}, err
		}
		opts.Config.Template = selectedTemplate
		result.Config.Template = selectedTemplate
		result.Config.TemplateDir = opts.Config.TemplateDir
		result.Run.Warnings = append(result.Run.Warnings, templateWarnings...)
		sanitizeAssessment(result)
		index := evidence.Build(result.Run, result.Artifacts, result.Observations)
		if err := writeOutputs(opts.OutputDir, opts.Config.Output.Formats, result, nil, index, opts.Config.Template); err != nil {
			return project.RunSummary{}, err
		}
		summary := buildRunSummary(opts.Config.Mode, result.Run, result.Status, len(result.Artifacts), len(result.Observations), outputFiles(opts.Config.Mode, opts.Config.Output.Formats))
		runLog = append(runLog, logEntry(time.Now().UTC(), "info", "assessment_completed", "Assessment run completed", map[string]string{
			"status":            result.Status,
			"artifact_count":    fmt.Sprintf("%d", len(result.Artifacts)),
			"observation_count": fmt.Sprintf("%d", len(result.Observations)),
		}))
		runLog = append(runLog, logEntry(time.Now().UTC(), "info", "outputs_written", "Outputs written", map[string]string{
			"output_count": fmt.Sprintf("%d", len(outputFiles(opts.Config.Mode, opts.Config.Output.Formats))),
		}))
		if err := persistProject(opts, result, nil, index, runLog, summary); err != nil {
			return project.RunSummary{}, err
		}
		return summary, nil
	case core.ModeInject:
		result, err := inject.Build(ctx, client, opts.Config, artifacts, observations, &runRecord)
		if err != nil {
			return project.RunSummary{}, err
		}
		result.Run.CompletedAt = time.Now().UTC()
		result.Run.Duration = result.Run.CompletedAt.Sub(started)
		selectedTemplate, templateWarnings, err := markdownrender.ResolveTemplate(core.ModeInject, opts.Config.Template, opts.Config.TemplateDir, opts.Config, result.Observations)
		if err != nil {
			return project.RunSummary{}, err
		}
		opts.Config.Template = selectedTemplate
		result.Config.Template = selectedTemplate
		result.Config.TemplateDir = opts.Config.TemplateDir
		result.Run.Warnings = append(result.Run.Warnings, templateWarnings...)
		sanitizeInject(result)
		index := evidence.Build(result.Run, result.Artifacts, result.Observations)
		if err := writeOutputs(opts.OutputDir, opts.Config.Output.Formats, nil, result, index, opts.Config.Template); err != nil {
			return project.RunSummary{}, err
		}
		summary := buildRunSummary(opts.Config.Mode, result.Run, result.Status, len(result.Artifacts), len(result.Observations), outputFiles(opts.Config.Mode, opts.Config.Output.Formats))
		runLog = append(runLog, logEntry(time.Now().UTC(), "info", "inject_completed", "Inject run completed", map[string]string{
			"status":            result.Status,
			"artifact_count":    fmt.Sprintf("%d", len(result.Artifacts)),
			"observation_count": fmt.Sprintf("%d", len(result.Observations)),
		}))
		runLog = append(runLog, logEntry(time.Now().UTC(), "info", "outputs_written", "Outputs written", map[string]string{
			"output_count": fmt.Sprintf("%d", len(outputFiles(opts.Config.Mode, opts.Config.Output.Formats))),
		}))
		if err := persistProject(opts, nil, result, index, runLog, summary); err != nil {
			return project.RunSummary{}, err
		}
		return summary, nil
	default:
		return project.RunSummary{}, errors.New("unsupported mode")
	}
}

func writeOutputs(outDir string, formats []string, assessResult *core.AssessmentResult, injectResult *core.InjectResult, index core.EvidenceIndex, templatePath string) error {
	names := outputNameSetForMode(resultMode(assessResult, injectResult))
	for _, format := range formats {
		switch strings.ToLower(format) {
		case "json":
			if assessResult != nil {
				if err := jsonrender.Write(filepath.Join(outDir, names.ResultJSON), assessResult); err != nil {
					return err
				}
				continue
			}
			if err := jsonrender.Write(filepath.Join(outDir, names.ResultJSON), injectResult); err != nil {
				return err
			}
		case "markdown":
			if assessResult != nil {
				if err := markdownrender.WriteAssessment(filepath.Join(outDir, names.ResultMarkdown), assessResult, templatePath); err != nil {
					return err
				}
				continue
			}
			if err := markdownrender.WriteInject(filepath.Join(outDir, names.ResultMarkdown), injectResult, templatePath); err != nil {
				return err
			}
		case "pdf":
			if assessResult != nil {
				if err := pdfrender.WriteAssessment(filepath.Join(outDir, names.ResultPDF), assessResult); err != nil {
					return err
				}
				continue
			}
			if err := pdfrender.WriteInject(filepath.Join(outDir, names.ResultPDF), injectResult); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported output format: %s", format)
		}
	}
	if err := evidencerender.WriteJSON(filepath.Join(outDir, names.EvidenceJSON), index); err != nil {
		return err
	}
	if err := evidencerender.WriteMarkdown(filepath.Join(outDir, names.EvidenceMarkdown), index); err != nil {
		return err
	}
	return nil
}

func persistProject(opts Options, assessResult *core.AssessmentResult, injectResult *core.InjectResult, index core.EvidenceIndex, runLog []project.RunLogEntry, summary project.RunSummary) error {
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
		if err := project.WriteRunSummary(opts.Config.Output.ProjectDir, summary); err != nil {
			return err
		}
		if err := project.CopyOutputs(opts.Config.Output.ProjectDir, opts.OutputDir, summary.OutputFiles); err != nil {
			return err
		}
		if err := project.WriteOutputInventory(opts.Config.Output.ProjectDir, buildOutputInventory(assessResult.Run.RunID, opts.Config.Mode, opts.OutputDir, opts.Config.Output.ProjectDir, summary.OutputFiles)); err != nil {
			return err
		}
		runLog = append(runLog, logEntry(time.Now().UTC(), "info", "project_persisted", "Project snapshot persisted", map[string]string{
			"project_dir": opts.Config.Output.ProjectDir,
		}))
		return project.WriteRunLog(opts.Config.Output.ProjectDir, runLog)
	}
	if err := project.WriteInject(opts.Config.Output.ProjectDir, opts.ManifestPath, *injectResult); err != nil {
		return err
	}
	if err := project.WriteEvidenceIndex(opts.Config.Output.ProjectDir, index); err != nil {
		return err
	}
	if err := project.WriteRunSummary(opts.Config.Output.ProjectDir, summary); err != nil {
		return err
	}
	if err := project.CopyOutputs(opts.Config.Output.ProjectDir, opts.OutputDir, summary.OutputFiles); err != nil {
		return err
	}
	if err := project.WriteOutputInventory(opts.Config.Output.ProjectDir, buildOutputInventory(injectResult.Run.RunID, opts.Config.Mode, opts.OutputDir, opts.Config.Output.ProjectDir, summary.OutputFiles)); err != nil {
		return err
	}
	runLog = append(runLog, logEntry(time.Now().UTC(), "info", "project_persisted", "Project snapshot persisted", map[string]string{
		"project_dir": opts.Config.Output.ProjectDir,
	}))
	return project.WriteRunLog(opts.Config.Output.ProjectDir, runLog)
}

func buildRunSummary(mode core.Mode, run core.RunRecord, status string, artifactCount, observationCount int, files []string) project.RunSummary {
	return project.RunSummary{
		RunID:            run.RunID,
		Mode:             string(mode),
		Title:            run.Title,
		Status:           status,
		Model:            run.Model,
		AI:               run.AI,
		ArtifactCount:    artifactCount,
		ObservationCount: observationCount,
		Warnings:         append([]string{}, run.Warnings...),
		Errors:           append([]string{}, run.Errors...),
		OutputFiles:      append([]string{}, files...),
	}
}

func outputFiles(mode core.Mode, formats []string) []string {
	names := outputNameSetForMode(mode)
	files := []string{names.EvidenceJSON, names.EvidenceMarkdown}
	for _, format := range formats {
		switch strings.ToLower(format) {
		case "json":
			files = append(files, names.ResultJSON)
		case "markdown":
			files = append(files, names.ResultMarkdown)
		case "pdf":
			files = append(files, names.ResultPDF)
		}
	}
	return files
}

func buildOutputInventory(runID string, mode core.Mode, outputDir, projectDir string, files []string) project.OutputInventory {
	inventory := project.OutputInventory{
		RunID:      runID,
		Mode:       string(mode),
		OutputDir:  outputDir,
		ProjectDir: projectDir,
		Generated:  time.Now().UTC(),
	}
	for _, name := range files {
		path := filepath.Join(outputDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		inventory.Files = append(inventory.Files, project.OutputInventoryEntry{
			Name:         filepath.Base(path),
			Path:         path,
			RelativePath: name,
			Size:         info.Size(),
			ModifiedAt:   info.ModTime().UTC(),
		})
	}
	return inventory
}

func logEntry(ts time.Time, level, event, message string, fields map[string]string) project.RunLogEntry {
	return project.RunLogEntry{
		Timestamp: ts,
		Level:     level,
		Event:     event,
		Message:   message,
		Fields:    fields,
	}
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

type outputNameSet struct {
	ResultMarkdown   string
	ResultJSON       string
	ResultPDF        string
	EvidenceJSON     string
	EvidenceMarkdown string
}

func outputNameSetForMode(mode core.Mode) outputNameSet {
	switch mode {
	case core.ModeInject:
		return outputNameSet{
			ResultMarkdown:   "inject-pack-draft.md",
			ResultJSON:       "inject-pack-data.json",
			ResultPDF:        "inject-pack-review.pdf",
			EvidenceJSON:     "inject-evidence-index.json",
			EvidenceMarkdown: "inject-evidence-index.md",
		}
	case core.ModeAssess:
		fallthrough
	default:
		return outputNameSet{
			ResultMarkdown:   "assessment-report-draft.md",
			ResultJSON:       "assessment-report-data.json",
			ResultPDF:        "assessment-report-review.pdf",
			EvidenceJSON:     "assessment-evidence-index.json",
			EvidenceMarkdown: "assessment-evidence-index.md",
		}
	}
}

func resultMode(assessResult *core.AssessmentResult, injectResult *core.InjectResult) core.Mode {
	if assessResult != nil {
		return core.ModeAssess
	}
	if injectResult != nil {
		return core.ModeInject
	}
	return core.ModeAssess
}
