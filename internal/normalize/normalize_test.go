package normalize

import (
	"strings"
	"testing"
	"time"

	"injectctl/internal/core"
)

func TestRedactAssessmentResultStructurally(t *testing.T) {
	t.Parallel()

	result := &core.AssessmentResult{
		Run: core.RunRecord{
			Warnings: []string{"host 10.0.0.5 mentioned"},
		},
		Status: "draft_ready",
		Config: core.Config{
			Instructions: "Investigate host 10.0.0.5 and server-alpha",
			Output: core.OutputConfig{
				RedactionRules: []string{`10\.0\.0\.5`, `server-alpha`},
			},
		},
		Artifacts: []core.Artifact{
			{
				Path:          `C:\evidence\10.0.0.5.txt`,
				ExtractedText: "SSH exposed on server-alpha",
				Metadata: map[string]string{
					"host": "10.0.0.5",
				},
			},
		},
		Observations: []core.Observation{
			{
				Detail: "10.0.0.5 exposes SSH",
				Source: map[string]string{
					"host": "server-alpha",
				},
				Evidence: []core.EvidenceExcerpt{
					{
						Snippet:     "10.0.0.5:22/tcp open ssh",
						Location:    "server-alpha",
						Description: "nmap for 10.0.0.5",
					},
				},
			},
		},
		Draft: core.AssessmentDraft{
			ExecutiveSummary: "10.0.0.5 belongs to server-alpha",
			Findings: []core.Finding{
				{
					Title:        "SSH on server-alpha",
					Description:  "10.0.0.5 has SSH exposed",
					EvidenceRefs: []string{"10.0.0.5:nmap"},
				},
			},
		},
		ErrorReport: &core.ErrorReport{
			Stage:       "synthesis",
			Message:     "10.0.0.5 caused a failure",
			GeneratedAt: time.Now().UTC(),
		},
	}

	warnings := RedactAssessmentResult(result, result.Config.Output.RedactionRules)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	serialized := strings.Join([]string{
		result.Config.Instructions,
		result.Artifacts[0].Path,
		result.Artifacts[0].ExtractedText,
		result.Artifacts[0].Metadata["host"],
		result.Observations[0].Detail,
		result.Observations[0].Source["host"],
		result.Observations[0].Evidence[0].Snippet,
		result.Observations[0].Evidence[0].Location,
		result.Observations[0].Evidence[0].Description,
		result.Draft.ExecutiveSummary,
		result.Draft.Findings[0].Title,
		result.Draft.Findings[0].Description,
		result.Draft.Findings[0].EvidenceRefs[0],
		result.ErrorReport.Message,
		result.Run.Warnings[0],
	}, " | ")

	if strings.Contains(serialized, "10.0.0.5") || strings.Contains(serialized, "server-alpha") {
		t.Fatalf("expected sensitive values to be redacted: %s", serialized)
	}
	if !strings.Contains(serialized, "[REDACTED]") {
		t.Fatalf("expected redacted marker in serialized content: %s", serialized)
	}
}

func TestRedactInjectResultInvalidRegexFallsBackToLiteral(t *testing.T) {
	t.Parallel()

	result := &core.InjectResult{
		Status: "draft_ready",
		Config: core.Config{
			Output: core.OutputConfig{
				RedactionRules: []string{"[invalid", "server-alpha"},
			},
		},
		Draft: core.InjectDraft{
			ScenarioSummary: "server-alpha triggered alert [invalid",
		},
	}

	warnings := RedactInjectResult(result, result.Config.Output.RedactionRules)
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "invalid redaction regex treated as literal") {
		t.Fatalf("unexpected warning: %v", warnings)
	}
	if strings.Contains(result.Draft.ScenarioSummary, "server-alpha") || strings.Contains(result.Draft.ScenarioSummary, "[invalid") {
		t.Fatalf("expected summary to be redacted, got %q", result.Draft.ScenarioSummary)
	}
}

func TestConflictWarningsDetectHostScopedContradictions(t *testing.T) {
	t.Parallel()

	warnings := ConflictWarnings([]core.Observation{
		{
			Title:    "Vulnerability detected",
			Detail:   "web01:443 TLS certificate expired",
			Category: "vulnerability",
			Severity: "high",
			Source: map[string]string{
				"tool": "nessus",
				"host": "web01",
			},
		},
		{
			Title:    "Vulnerability detected",
			Detail:   "web01:443 TLS certificate valid",
			Category: "vulnerability",
			Severity: "info",
			Source: map[string]string{
				"tool": "nessus",
				"host": "web01",
			},
		},
	})

	if len(warnings) != 1 {
		t.Fatalf("expected one conflict warning, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "web01:443") {
		t.Fatalf("expected host and port in warning, got %v", warnings)
	}
}

func TestConflictWarningsIgnoreIndependentObservations(t *testing.T) {
	t.Parallel()

	warnings := ConflictWarnings([]core.Observation{
		{
			Title:    "Open service detected",
			Detail:   "22/tcp open ssh OpenSSH 8.2p1",
			Category: "network",
			Severity: "medium",
			Source: map[string]string{
				"tool": "nmap",
			},
		},
		{
			Title:    "Open service detected",
			Detail:   "80/tcp open http Apache httpd",
			Category: "network",
			Severity: "medium",
			Source: map[string]string{
				"tool": "nmap",
			},
		},
	})

	if len(warnings) != 0 {
		t.Fatalf("expected no conflict warnings, got %v", warnings)
	}
}
