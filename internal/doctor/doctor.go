package doctor

import (
	"context"
	"strings"

	"injectctl/internal/ai/ollama"
	"injectctl/internal/core"
	"injectctl/internal/ocr"
)

func Run(ctx context.Context, endpoint, model, fallback string) (core.DoctorStatus, error) {
	client := ollama.New(core.AIConfig{
		Endpoint:      endpoint,
		Model:         model,
		FallbackModel: fallback,
	})

	status := core.DoctorStatus{
		Endpoint:     endpoint,
		OCRAvailable: ocr.New().Available(),
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

	if !containsModel(models, model) {
		if fallback != "" && containsModel(models, fallback) {
			status.Warnings = append(status.Warnings, "primary model missing; fallback model is available")
		} else {
			status.Errors = append(status.Errors, "required primary model is not installed")
		}
	}
	if !status.OCRAvailable {
		status.Warnings = append(status.Warnings, "tesseract is not installed; screenshot OCR will be unavailable")
	}
	return status, nil
}

func containsModel(models []string, target string) bool {
	for _, model := range models {
		if strings.EqualFold(model, target) || strings.HasPrefix(strings.ToLower(model), strings.ToLower(target)+":") {
			return true
		}
	}
	return false
}
