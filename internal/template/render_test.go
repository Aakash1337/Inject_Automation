package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"injectctl/internal/core"
)

func TestRenderAssessmentDefaultTemplate(t *testing.T) {
	t.Parallel()

	result := &core.AssessmentResult{
		Status: "draft_ready",
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
	if !strings.Contains(output, "draft_ready") {
		t.Fatalf("expected status in output: %s", output)
	}
}

func TestRenderAssessmentIncludesErrorReport(t *testing.T) {
	t.Parallel()

	result := &core.AssessmentResult{
		Status: "evidence_only",
		Config: core.Config{
			Title:          "Assessment",
			Client:         "Example Corp",
			Environment:    "Prod",
			Classification: "TLP:AMBER",
		},
		ErrorReport: &core.ErrorReport{
			Stage:   "synthesis",
			Message: "model returned invalid json",
			Recommendations: []string{
				"Review the normalized evidence in the JSON output.",
			},
		},
	}

	data, err := RenderAssessment(result, "")
	if err != nil {
		t.Fatalf("render assessment with error report: %v", err)
	}
	output := string(data)
	if !strings.Contains(output, "Generation Error") {
		t.Fatalf("expected error report in output: %s", output)
	}
	if !strings.Contains(output, "evidence_only") {
		t.Fatalf("expected evidence-only status in output: %s", output)
	}
}

func TestValidateAcceptsJinjaTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md.j2")
	if err := os.WriteFile(path, []byte("# {{ config.title }}\n{% if draft.findings %}ok{% endif %}\n"), 0o644); err != nil {
		t.Fatalf("write jinja template: %v", err)
	}
	if err := Validate(path); err != nil {
		t.Fatalf("validate jinja template: %v", err)
	}
}

func TestRenderAssessmentCustomJinjaTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md.j2")
	if err := os.WriteFile(path, []byte("# {{ config.title }}\n{{ draft.findings|length }} findings\n"), 0o644); err != nil {
		t.Fatalf("write jinja template: %v", err)
	}

	result := &core.AssessmentResult{
		Status: "draft_ready",
		Config: core.Config{
			Title: "Assessment",
		},
		Draft: core.AssessmentDraft{
			Findings: []core.Finding{
				{Title: "one"},
				{Title: "two"},
			},
		},
	}

	data, err := RenderAssessment(result, path)
	if err != nil {
		t.Fatalf("render assessment with jinja template: %v", err)
	}
	output := string(data)
	if !strings.Contains(output, "# Assessment") || !strings.Contains(output, "2 findings") {
		t.Fatalf("unexpected jinja output: %s", output)
	}
}

func TestResolveTemplatePathUsesManualSelection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "manual-report.md.j2")
	if err := os.WriteFile(path, []byte("# {{ config.title }}"), 0o644); err != nil {
		t.Fatalf("write manual template: %v", err)
	}

	selected, warnings, err := ResolveTemplatePath(core.ModeAssess, path, "", core.Config{Mode: core.ModeAssess}, nil)
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	if selected != path {
		t.Fatalf("expected manual template path, got %q", selected)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "manually selected template") {
		t.Fatalf("expected manual selection warning, got %v", warnings)
	}
}

func TestListCandidatesPrefersAssessmentTemplateForAssessmentMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "generic.md.j2"), []byte("# {{ config.title }}"), 0o644); err != nil {
		t.Fatalf("write generic template: %v", err)
	}
	assessmentPath := filepath.Join(dir, "assessment-executive.md.j2")
	if err := os.WriteFile(assessmentPath, []byte("# {{ config.title }}\n{{ draft.executive_summary }}\n{% for finding in draft.findings %}{{ finding.title }}{% endfor %}"), 0o644); err != nil {
		t.Fatalf("write assessment template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inject-pack.md.j2"), []byte("# {{ config.title }}\n{{ draft.scenario_summary }}\n{% for item in draft.items %}{{ item.title }}{% endfor %}"), 0o644); err != nil {
		t.Fatalf("write inject template: %v", err)
	}

	candidates, err := ListCandidates(core.ModeAssess, dir, core.Config{
		Mode:         core.ModeAssess,
		Title:        "Assessment Run",
		Instructions: "Draft an assessment report",
	}, []core.Observation{
		{Title: "Privilege escalation or root access visible", Category: "privilege"},
	})
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	if candidates[0].Path != assessmentPath {
		t.Fatalf("expected assessment template to rank first, got %q", candidates[0].Path)
	}
}

func TestListCandidatesUsesMetadataKeywordsForSelection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legalPath := filepath.Join(dir, "plain.md.j2")
	if err := os.WriteFile(legalPath, []byte("# {{ config.title }}"), 0o644); err != nil {
		t.Fatalf("write legal template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.meta.json"), []byte(`{"name":"Counsel Memo","description":"Legal and compliance memo","mode":"inject","tags":["legal","compliance","notification"]}`), 0o644); err != nil {
		t.Fatalf("write legal metadata: %v", err)
	}
	technicalPath := filepath.Join(dir, "worksheet.md.j2")
	if err := os.WriteFile(technicalPath, []byte("# {{ config.title }}"), 0o644); err != nil {
		t.Fatalf("write technical template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worksheet.meta.json"), []byte(`{"name":"Operator Worksheet","description":"Technical operations template","mode":"inject","tags":["technical","operations","engineering"]}`), 0o644); err != nil {
		t.Fatalf("write technical metadata: %v", err)
	}

	candidates, err := ListCandidates(core.ModeInject, dir, core.Config{
		Mode:         core.ModeInject,
		Title:        "Breach Counsel Decision Run",
		Instructions: "Prepare a legal notification memo for compliance counsel and privacy review.",
	}, nil)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) < 2 {
		t.Fatalf("expected at least 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Path != legalPath {
		t.Fatalf("expected legal template to rank first, got %q", candidates[0].Path)
	}
}

func TestShippedLibraryTemplatesValidate(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("..", "..", "templates", "library")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read library templates: %v", err)
	}

	validated := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".j2") {
			continue
		}
		validated++
		if err := Validate(filepath.Join(dir, entry.Name())); err != nil {
			t.Fatalf("validate %s: %v", entry.Name(), err)
		}
	}
	if validated == 0 {
		t.Fatal("expected shipped library templates to validate")
	}
}
