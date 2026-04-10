package template

import (
	"strings"
	"testing"

	"injectctl/internal/core"
)

func TestRenderAssessmentDefaultTemplate(t *testing.T) {
	t.Parallel()

	result := &core.AssessmentResult{
		Config: core.Config{
			Title:          "Assessment",
			Client:         "Example Corp",
			Environment:    "Prod",
			Classification: "TLP:AMBER",
		},
		Observations: []core.Observation{
			{ID: "obs-1", Title: "Open service detected", Detail: "22/tcp open ssh"},
		},
		Draft: core.AssessmentDraft{
			ExecutiveSummary: "Summary",
			Findings: []core.Finding{
				{
					Title:          "SSH exposed",
					Severity:       "medium",
					Description:    "SSH is reachable.",
					Impact:         "Remote exposure",
					Remediation:    "Restrict access",
					EvidenceRefs:   []string{"abc123:Nmap open port line"},
					ObservationIDs: []string{"obs-1"},
				},
			},
		},
	}

	data, err := RenderAssessment(result, "")
	if err != nil {
		t.Fatalf("render assessment: %v", err)
	}
	output := string(data)
	if !strings.Contains(output, "SSH exposed") {
		t.Fatalf("expected finding title in output: %s", output)
	}
	if !strings.Contains(output, "abc123:Nmap open port line") {
		t.Fatalf("expected evidence refs in output: %s", output)
	}
}
