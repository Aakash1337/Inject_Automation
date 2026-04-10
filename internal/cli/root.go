package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"injectctl/internal/config"
	"injectctl/internal/core"
	"injectctl/internal/doctor"
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

	resolvedOut := *outDir
	if !filepath.IsAbs(resolvedOut) {
		resolvedOut = filepath.Join(baseDir, resolvedOut)
	}

	var inputs []string
	if *input != "" {
		inputs = append(inputs, *input)
	}

	return runner.Run(ctx, runner.Options{
		Config:      cfg,
		ManifestPath: *manifest,
		InputPaths:  inputs,
		OutputDir:   resolvedOut,
	})
}

func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "http://127.0.0.1:11434", "Ollama endpoint")
	model := fs.String("model", "gemma4:26b", "Primary model to validate")
	fallback := fs.String("fallback-model", "gemma4:e4b", "Fallback model to validate")
	if err := fs.Parse(args); err != nil {
		return err
	}

	status, err := doctor.Run(ctx, *endpoint, *model, *fallback)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Ollama endpoint: %s\n", status.Endpoint)
	fmt.Fprintf(os.Stdout, "Reachable: %t\n", status.OllamaReachable)
	fmt.Fprintf(os.Stdout, "OCR available: %t\n", status.OCRAvailable)
	if len(status.InstalledModels) > 0 {
		fmt.Fprintln(os.Stdout, "Installed models:")
		for _, model := range status.InstalledModels {
			fmt.Fprintf(os.Stdout, "  - %s\n", model)
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
	if len(args) == 0 || args[0] != "validate" {
		return errors.New("template requires the validate subcommand")
	}

	fs := flag.NewFlagSet("template validate", flag.ContinueOnError)
	templatePath := fs.String("template", "", "Path to the template file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *templatePath == "" {
		return errors.New("--template is required")
	}

	return templaterender.Validate(*templatePath)
}

func runInit(args []string) error {
	if len(args) == 0 || args[0] != "manifest" {
		return errors.New("init requires the manifest subcommand")
	}

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
	cfg.Template = "templates/default/" + string(cfg.Mode) + ".md.tmpl"
	cfg.Artifacts = []string{"./artifacts"}
	cfg.Output.ProjectDir = "./project"

	data, err := config.MarshalYAML(cfg)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(*out, data, 0o644)
}

func usageError() error {
	return errors.New("usage: injectctl <assess|inject|doctor|template|init> ...")
}
