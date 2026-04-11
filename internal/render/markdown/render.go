package markdown

import (
	"os"

	"injectctl/internal/core"
	templaterender "injectctl/internal/template"
)

func ResolveTemplate(mode core.Mode, preferredPath, templateDir string, cfg core.Config, observations []core.Observation) (string, []string, error) {
	return templaterender.ResolveTemplatePath(mode, preferredPath, templateDir, cfg, observations)
}

func WriteAssessment(path string, result *core.AssessmentResult, templatePath string) error {
	data, err := templaterender.RenderAssessment(result, templatePath)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func WriteInject(path string, result *core.InjectResult, templatePath string) error {
	data, err := templaterender.RenderInject(result, templatePath)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
