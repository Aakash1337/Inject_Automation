package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"injectctl/internal/core"
)

func TestRunReportsInstalledModels(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{
				{"name": "gemma4:26b"},
				{"name": "gemma4:e4b"},
			},
		})
	}))
	defer server.Close()

	status, err := Run(context.Background(), core.AIConfig{
		Endpoint:      server.URL,
		Model:         "gemma4:26b",
		FallbackModel: "gemma4:e4b",
	}, false)
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if !status.OllamaReachable {
		t.Fatal("expected ollama to be reachable")
	}
	if len(status.Errors) != 0 {
		t.Fatalf("expected no blocking errors, got %v", status.Errors)
	}
}

func TestRunSmokeTestPasses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{
					{"name": "gemma4:26b"},
					{"name": "gemma4:e4b"},
				},
			})
		case "/api/generate":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"response": "OK",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	status, err := Run(context.Background(), core.AIConfig{
		Endpoint: server.URL,
		Profile:  "balanced",
	}, true)
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if !status.SmokeTestPassed {
		t.Fatal("expected smoke test to pass")
	}
	if status.SmokeTestModel != "gemma4:26b" {
		t.Fatalf("unexpected smoke test model %q", status.SmokeTestModel)
	}
}
