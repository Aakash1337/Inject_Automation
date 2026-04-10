package template

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	texttemplate "text/template"

	"injectctl/internal/core"
)

const defaultAssessmentTemplate = `# {{ .Config.Title }}

**Client:** {{ .Config.Client }}
**Environment:** {{ .Config.Environment }}
**Classification:** {{ .Config.Classification }}

## Executive Summary
{{ .Draft.ExecutiveSummary }}

## Findings
{{- if .Draft.Findings }}
{{- range .Draft.Findings }}
### {{ .Title }} ({{ .Severity }})
{{ .Description }}

**Impact:** {{ .Impact }}
**Remediation:** {{ .Remediation }}
**Evidence:** {{ join .EvidenceRefs ", " }}
**Observations:** {{ join .ObservationIDs ", " }}

{{- end }}
{{- else }}
No findings were generated. Review the observations section.
{{- end }}

## Observations
{{- range .Observations }}
- {{ .ID }} | {{ .Title }} | {{ .Detail }}
{{- end }}
`

const defaultInjectTemplate = `# {{ .Config.Title }}

**Client:** {{ .Config.Client }}
**Environment:** {{ .Config.Environment }}
**Classification:** {{ .Config.Classification }}

## Scenario Summary
{{ .Draft.ScenarioSummary }}

## Injects
{{- if .Draft.Items }}
{{- range .Draft.Items }}
### {{ .Title }}
**Audience:** {{ .Audience }}
**Channel:** {{ .Channel }}
**Trigger:** {{ .Trigger }}
**Expected Action:** {{ .ExpectedAction }}

{{ .Body }}

**Evidence:** {{ join .EvidenceRefs ", " }}
**Observations:** {{ join .ObservationIDs ", " }}

{{- end }}
{{- else }}
No injects were generated. Review the observations section.
{{- end }}

## Observations
{{- range .Observations }}
- {{ .ID }} | {{ .Title }} | {{ .Detail }}
{{- end }}
`

func Validate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = texttemplate.New("validate").Funcs(funcMap()).Parse(string(data))
	return err
}

func RenderAssessment(result *core.AssessmentResult, path string) ([]byte, error) {
	return render(result, path, defaultAssessmentTemplate)
}

func RenderInject(result *core.InjectResult, path string) ([]byte, error) {
	return render(result, path, defaultInjectTemplate)
}

func render(data any, path, fallback string) ([]byte, error) {
	source := fallback
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read template: %w", err)
		}
		source = string(raw)
	}

	tmpl, err := texttemplate.New("report").Funcs(funcMap()).Parse(source)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func funcMap() texttemplate.FuncMap {
	return texttemplate.FuncMap{
		"join": func(items []string, sep string) string {
			return strings.Join(items, sep)
		},
	}
}
