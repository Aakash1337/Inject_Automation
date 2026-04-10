package doctor

import (
	"context"

	"injectctl/internal/ai/ollama"
	"injectctl/internal/config"
	"injectctl/internal/core"
	"injectctl/internal/ocr"
)

func Run(ctx context.Context, aiCfg core.AIConfig, smoke bool) (core.DoctorStatus, error) {
	if err := config.ApplyAIDefaults(&aiCfg); err != nil {
		return core.DoctorStatus{}, err
	}

	client := ollama.New(aiCfg)

	status := core.DoctorStatus{
		Endpoint:      aiCfg.Endpoint,
		Profile:       aiCfg.Profile,
		PrimaryModel:  aiCfg.Model,
		FallbackModel: aiCfg.FallbackModel,
		OCRAvailable:  ocr.New().Available(),
	}

	reachable, err := client.Health(ctx)
	if err != nil {
		status.Errors = append(status.Errors, "ollama is not reachable")
		return status, nil
	}
	status.OllamaReachable = reachable

	models, err := client.ListModels(ctx)
	if err != nil {
		status.Errors = append(status.Errors, "failed to list installed ollama models")
		return status, nil
	}
	status.InstalledModels = models

	if !ollama.ContainsModel(models, aiCfg.Model) {
		if aiCfg.FallbackModel != "" && ollama.ContainsModel(models, aiCfg.FallbackModel) {
			status.Warnings = append(status.Warnings, "primary model missing; fallback model is available")
		} else {
			status.Errors = append(status.Errors, "required primary model is not installed")
		}
	}
	if !status.OCRAvailable {
		status.Warnings = append(status.Warnings, "tesseract is not installed; screenshot OCR will be unavailable")
	}
	if smoke && len(status.Errors) == 0 {
		model, warnings, err := client.SmokeTest(ctx)
		status.Warnings = append(status.Warnings, warnings...)
		if err != nil {
			status.Errors = append(status.Errors, "ollama smoke test failed: "+err.Error())
		} else {
			status.SmokeTestModel = model
			status.SmokeTestPassed = true
		}
	}
	return status, nil
}
