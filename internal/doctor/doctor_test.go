package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

	status, err := Run(context.Background(), server.URL, "gemma4:26b", "gemma4:e4b")
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
