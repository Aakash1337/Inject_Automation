package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"injectctl/internal/config"
	"injectctl/internal/core"
	"injectctl/internal/demo"
	"injectctl/internal/doctor"
	"injectctl/internal/project"
	"injectctl/internal/runner"
	templaterender "injectctl/internal/template"
)

func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "assess":
		return runMode(ctx, core.ModeAssess, args[1:])
	case "inject":
		return runMode(ctx, core.ModeInject, args[1:])
	case "doctor":
		return runDoctor(ctx, args[1:])
	case "template":
		return runTemplate(args[1:])
	case "init":
		return runInit(args[1:])
	default:
		return usageError()
	}
}

func runMode(ctx context.Context, mode core.Mode, args []string) error {
	if len(args) == 0 || args[0] != "run" {
		return fmt.Errorf("%s command requires the run subcommand", mode)
	}

	fs := flag.NewFlagSet(string(mode)+" run", flag.ContinueOnError)
	manifest := fs.String("manifest", "", "Path to the manifest file")
	input := fs.String("input", "", "Path to an input file or directory")
	outDir := fs.String("out", "out", "Output directory")
	templatePath := fs.String("template", "", "Path to a specific Markdown template file")
	templateDir := fs.String("template-dir", "", "Directory of candidate Markdown templates for auto-selection")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *manifest == "" {
		return errors.New("--manifest is required")
	}

	cfg, baseDir, err := config.Load(*manifest)
	if err != nil {
		return err
	}
	cfg.Mode = mode
	config.ResolvePaths(&cfg, baseDir)
	if *templatePath != "" {
		if filepath.IsAbs(*templatePath) {
			cfg.Template = *templatePath
		} else {
			cfg.Template = filepath.Join(baseDir, *templatePath)
		}
	}
	if *templateDir != "" {
		if filepath.IsAbs(*templateDir) {
			cfg.TemplateDir = *templateDir
		} else {
			cfg.TemplateDir = filepath.Join(baseDir, *templateDir)
		}
	}

	resolvedOut, err := resolveOutputDir(fs, baseDir, *outDir)
	if err != nil {
		return err
	}

	var inputs []string
	if *input != "" {
		inputs = append(inputs, *input)
	}

	summary, err := runner.Run(ctx, runner.Options{
		Config:       cfg,
		ManifestPath: *manifest,
		InputPaths:   inputs,
		OutputDir:    resolvedOut,
	})
	if err != nil {
		return err
	}
	printRunSummary(summary, resolvedOut, cfg.Output.ProjectDir)
	return nil
}

func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "http://127.0.0.1:11434", "Ollama endpoint")
	profile := fs.String("profile", "balanced", "Model profile to validate: fast, balanced, quality")
	model := fs.String("model", "", "Primary model to validate")
	fallback := fs.String("fallback-model", "", "Fallback model to validate")
	smoke := fs.Bool("smoke", false, "Run a lightweight live inference smoke test")
	if err := fs.Parse(args); err != nil {
		return err
	}

	status, err := doctor.Run(ctx, core.AIConfig{
		Endpoint:      *endpoint,
		Profile:       *profile,
		Model:         *model,
		FallbackModel: *fallback,
	}, *smoke)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Ollama endpoint: %s\n", status.Endpoint)
	if status.Profile != "" {
		fmt.Fprintf(os.Stdout, "Profile: %s\n", status.Profile)
	}
	if status.PrimaryModel != "" {
		fmt.Fprintf(os.Stdout, "Primary model: %s\n", status.PrimaryModel)
	}
	if status.FallbackModel != "" {
		fmt.Fprintf(os.Stdout, "Fallback model: %s\n", status.FallbackModel)
	}
	fmt.Fprintf(os.Stdout, "Reachable: %t\n", status.OllamaReachable)
	fmt.Fprintf(os.Stdout, "OCR available: %t\n", status.OCRAvailable)
	if len(status.InstalledModels) > 0 {
		fmt.Fprintln(os.Stdout, "Installed models:")
		for _, model := range status.InstalledModels {
			fmt.Fprintf(os.Stdout, "  - %s\n", model)
		}
	}
	if *smoke {
		if status.SmokeTestPassed {
			fmt.Fprintf(os.Stdout, "Smoke test: passed (%s)\n", status.SmokeTestModel)
		} else {
			fmt.Fprintln(os.Stdout, "Smoke test: failed")
		}
	}
	for _, warning := range status.Warnings {
		fmt.Fprintf(os.Stdout, "warning: %s\n", warning)
	}
	for _, failure := range status.Errors {
		fmt.Fprintf(os.Stdout, "error: %s\n", failure)
	}
	if len(status.Errors) > 0 {
		return errors.New("doctor found blocking issues")
	}
	return nil
}

func runTemplate(args []string) error {
	if len(args) == 0 {
		return errors.New("template requires the validate or list subcommand")
	}

	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("template validate", flag.ContinueOnError)
		templatePath := fs.String("template", "", "Path to the template file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *templatePath == "" {
			return errors.New("--template is required")
		}
		return templaterender.Validate(*templatePath)
	case "list":
		fs := flag.NewFlagSet("template list", flag.ContinueOnError)
		dir := fs.String("dir", "", "Directory of template candidates")
		mode := fs.String("mode", "assess", "Mode: assess or inject")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" {
			return errors.New("--dir is required")
		}

		templateMode := core.ModeAssess
		switch strings.ToLower(strings.TrimSpace(*mode)) {
		case string(core.ModeAssess):
			templateMode = core.ModeAssess
		case string(core.ModeInject):
			templateMode = core.ModeInject
		default:
			return errors.New("--mode must be assess or inject")
		}

		cfg := config.DefaultConfig()
		cfg.Mode = templateMode
		candidates, err := templaterender.ListCandidates(templateMode, *dir, cfg, nil)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			fmt.Fprintln(os.Stdout, "No compatible templates found.")
			return nil
		}
		fmt.Fprintln(os.Stdout, "Template candidates:")
		for _, candidate := range candidates {
			fmt.Fprintf(os.Stdout, "- %s\n", candidate.Path)
			fmt.Fprintf(os.Stdout, "  score: %d\n", candidate.Score)
			fmt.Fprintf(os.Stdout, "  reason: %s\n", candidate.Reason)
			if candidate.Description != "" {
				fmt.Fprintf(os.Stdout, "  description: %s\n", candidate.Description)
			}
		}
		return nil
	default:
		return errors.New("template requires the validate or list subcommand")
	}
}

func runInit(args []string) error {
	if len(args) == 0 {
		return errors.New("init requires the manifest or demo subcommand")
	}

	switch args[0] {
	case "manifest":
		fs := flag.NewFlagSet("init manifest", flag.ContinueOnError)
		mode := fs.String("mode", "assess", "Mode: assess or inject")
		out := fs.String("out", "job.yaml", "Destination path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		cfg := config.DefaultConfig()
		switch *mode {
		case string(core.ModeAssess):
			cfg.Mode = core.ModeAssess
			cfg.Title = "Assessment Run"
			cfg.Instructions = "Turn the provided evidence into a draft assessment report."
		case string(core.ModeInject):
			cfg.Mode = core.ModeInject
			cfg.Title = "Exercise Inject Run"
			cfg.Instructions = "Turn the provided evidence into a draft inject pack."
		default:
			return errors.New("--mode must be assess or inject")
		}
		cfg.Client = "Example Corp"
		cfg.Environment = "Production"
		cfg.Artifacts = []string{"./artifacts"}
		cfg.Output.ProjectDir = "./project"

		data, err := config.MarshalYAML(cfg)
		if err != nil {
			return fmt.Errorf("marshal manifest: %w", err)
		}
		return os.WriteFile(*out, data, 0o644)
	case "demo":
		fs := flag.NewFlagSet("init demo", flag.ContinueOnError)
		mode := fs.String("mode", "assess", "Mode: assess or inject")
		out := fs.String("out", "demo", "Destination directory")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		switch *mode {
		case string(core.ModeAssess):
			return demo.Generate(*out, core.ModeAssess)
		case string(core.ModeInject):
			return demo.Generate(*out, core.ModeInject)
		default:
			return errors.New("--mode must be assess or inject")
		}
	default:
		return errors.New("init requires the manifest or demo subcommand")
	}
}

func usageError() error {
	return errors.New("usage: injectctl <assess|inject|doctor|template|init> ...")
}

func resolveOutputDir(fs *flag.FlagSet, baseDir, outDir string) (string, error) {
	if filepath.IsAbs(outDir) {
		return outDir, nil
	}

	outExplicitlySet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "out" {
			outExplicitlySet = true
		}
	})
	if !outExplicitlySet {
		return filepath.Join(baseDir, outDir), nil
	}
	return filepath.Abs(outDir)
}

func printRunSummary(summary project.RunSummary, outputDir, projectDir string) {
	writeRunSummary(os.Stdout, summary, outputDir, projectDir)
}

func writeRunSummary(out io.Writer, summary project.RunSummary, outputDir, projectDir string) {
	fmt.Fprintf(out, "Status: %s\n", summary.Status)
	fmt.Fprintf(out, "Output directory: %s\n", outputDir)
	if projectDir != "" {
		fmt.Fprintf(out, "Project snapshot: %s\n", projectDir)
	}
	if summary.AI != nil {
		if len(summary.AI.ModelsUsed) > 0 {
			fmt.Fprintf(out, "AI models used: %s\n", strings.Join(summary.AI.ModelsUsed, ", "))
		}
		if summary.AI.BatchCount > 0 {
			fmt.Fprintf(out, "AI batch count: %d\n", summary.AI.BatchCount)
		}
	}
	if len(summary.Warnings) > 0 {
		fmt.Fprintf(out, "Warnings: %d\n", len(summary.Warnings))
	}
	if len(summary.OutputFiles) > 0 {
		fmt.Fprintln(out, "Generated files:")
		for _, file := range summary.OutputFiles {
			fmt.Fprintf(out, "  - %s\n", file)
		}
	}
}
