package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	texttemplate "text/template"

	"github.com/flosch/pongo2/v6"

	"injectctl/internal/core"
)

const defaultAssessmentTemplate = `# {{ config.title }}

**Client:** {{ config.client }}
**Environment:** {{ config.environment }}
**Classification:** {{ config.classification }}
**Status:** {{ status }}
{% if run.ai %}
{% if run.ai.requested_model %}
**AI Requested Model:** {{ run.ai.requested_model }}
{% endif %}
{% if run.ai.fallback_model %}
**AI Fallback Model:** {{ run.ai.fallback_model }}
{% endif %}
{% if run.ai.models_used %}
**AI Models Used:** {{ run.ai.models_used|join:", " }}
{% endif %}
{% if run.ai.batch_count %}
**AI Batch Count:** {{ run.ai.batch_count }}
{% endif %}
{% if run.ai.stages %}
**AI Stages:**
{% for stage in run.ai.stages %}
- {{ stage.name }}: {{ stage.model }}
{% endfor %}
{% endif %}
{% endif %}

{% if error_report %}
## Generation Error
**Stage:** {{ error_report.stage }}
**Message:** {{ error_report.message }}

{% if error_report.recommendations %}
### Recommendations
{% for recommendation in error_report.recommendations %}
- {{ recommendation }}
{% endfor %}
{% endif %}
{% endif %}

## Executive Summary
{{ draft.executive_summary }}

## Findings
{% if draft.findings %}
{% for finding in draft.findings %}
### {{ finding.title }} ({{ finding.severity }})
{{ finding.description }}

**Impact:** {{ finding.impact }}
**Remediation:** {{ finding.remediation }}
**Evidence:** {{ finding.evidence_refs|join:", " }}
**Observations:** {{ finding.observation_ids|join:", " }}

{% endfor %}
{% else %}
No findings were generated. Review the observations section.
{% endif %}

## Observations
{% for observation in observations %}
- {{ observation.id }} | {{ observation.title }} | {{ observation.detail }}
{% endfor %}
`

const defaultInjectTemplate = `# {{ config.title }}

**Client:** {{ config.client }}
**Environment:** {{ config.environment }}
**Classification:** {{ config.classification }}
**Status:** {{ status }}
{% if run.ai %}
{% if run.ai.requested_model %}
**AI Requested Model:** {{ run.ai.requested_model }}
{% endif %}
{% if run.ai.fallback_model %}
**AI Fallback Model:** {{ run.ai.fallback_model }}
{% endif %}
{% if run.ai.models_used %}
**AI Models Used:** {{ run.ai.models_used|join:", " }}
{% endif %}
{% if run.ai.batch_count %}
**AI Batch Count:** {{ run.ai.batch_count }}
{% endif %}
{% if run.ai.stages %}
**AI Stages:**
{% for stage in run.ai.stages %}
- {{ stage.name }}: {{ stage.model }}
{% endfor %}
{% endif %}
{% endif %}

{% if error_report %}
## Generation Error
**Stage:** {{ error_report.stage }}
**Message:** {{ error_report.message }}

{% if error_report.recommendations %}
### Recommendations
{% for recommendation in error_report.recommendations %}
- {{ recommendation }}
{% endfor %}
{% endif %}
{% endif %}

## Scenario Summary
{{ draft.scenario_summary }}

## Injects
{% if draft.items %}
{% for item in draft.items %}
### {{ item.title }}
**Audience:** {{ item.audience }}
**Channel:** {{ item.channel }}
**Trigger:** {{ item.trigger }}
**Expected Action:** {{ item.expected_action }}

{{ item.body }}

**Evidence:** {{ item.evidence_refs|join:", " }}
**Observations:** {{ item.observation_ids|join:", " }}

{% endfor %}
{% else %}
No injects were generated. Review the observations section.
{% endif %}

## Observations
{% for observation in observations %}
- {{ observation.id }} | {{ observation.title }} | {{ observation.detail }}
{% endfor %}
`

func Validate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	engine := templateEngineForPath(path)
	if engine == templateEngineJinja {
		_, err = pongo2.FromString(string(data))
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

type Candidate struct {
	Path        string
	Name        string
	Mode        string
	Score       int
	Reason      string
	Description string
}

type candidateMetadata struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Mode        string   `json:"mode" yaml:"mode"`
	Tags        []string `json:"tags" yaml:"tags"`
	Priority    int      `json:"priority" yaml:"priority"`
}

func ResolveTemplatePath(mode core.Mode, preferredPath, templateDir string, cfg core.Config, observations []core.Observation) (string, []string, error) {
	if strings.TrimSpace(preferredPath) != "" {
		if info, err := os.Stat(preferredPath); err != nil {
			return "", nil, fmt.Errorf("template path is not accessible: %w", err)
		} else if info.IsDir() {
			templateDir = preferredPath
		} else {
			return preferredPath, []string{"using manually selected template: " + preferredPath}, nil
		}
	}
	if strings.TrimSpace(templateDir) == "" {
		return "", nil, nil
	}

	candidates, err := ListCandidates(mode, templateDir, cfg, observations)
	if err != nil {
		return "", nil, err
	}
	if len(candidates) == 0 {
		return "", []string{"no compatible templates found in template directory; using built-in template"}, nil
	}
	best := candidates[0]
	return best.Path, []string{fmt.Sprintf("auto-selected template: %s (%s)", best.Path, best.Reason)}, nil
}

func ListCandidates(mode core.Mode, templateDir string, cfg core.Config, observations []core.Observation) ([]Candidate, error) {
	entries, err := discoverTemplateCandidates(templateDir)
	if err != nil {
		return nil, err
	}

	contextTokens := selectionTokens(cfg, observations)
	var candidates []Candidate
	for _, entry := range entries {
		contentBytes, err := os.ReadFile(entry)
		if err != nil {
			return nil, fmt.Errorf("read template candidate: %w", err)
		}
		content := string(contentBytes)
		metadata := loadCandidateMetadata(entry)
		score, reason, compatible := scoreCandidate(mode, entry, content, metadata, contextTokens)
		if !compatible {
			continue
		}
		name := metadata.Name
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(entry)
		}
		candidates = append(candidates, Candidate{
			Path:        entry,
			Name:        name,
			Mode:        string(mode),
			Score:       score,
			Reason:      reason,
			Description: metadata.Description,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Path < candidates[j].Path
	})
	return candidates, nil
}

func render(data any, path, fallback string) ([]byte, error) {
	source := fallback
	engine := templateEngineJinja
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read template: %w", err)
		}
		source = string(raw)
		engine = templateEngineForPath(path)
	}

	if engine == templateEngineJinja {
		return renderJinja(data, source)
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

func discoverTemplateCandidates(dir string) ([]string, error) {
	var candidates []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isTemplateCandidate(path) {
			return nil
		}
		candidates = append(candidates, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan template directory: %w", err)
	}
	sort.Strings(candidates)
	return candidates, nil
}

func isTemplateCandidate(path string) bool {
	lower := strings.ToLower(filepath.Base(path))
	if strings.Contains(lower, ".meta.") {
		return false
	}
	switch {
	case strings.HasSuffix(lower, ".j2"),
		strings.HasSuffix(lower, ".jinja"),
		strings.HasSuffix(lower, ".jinja2"),
		strings.HasSuffix(lower, ".tmpl"),
		strings.HasSuffix(lower, ".tpl"),
		strings.HasSuffix(lower, ".md"):
		return true
	default:
		return false
	}
}

func loadCandidateMetadata(path string) candidateMetadata {
	for _, metaPath := range metadataPathsForTemplate(path) {
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta candidateMetadata
		if strings.HasSuffix(strings.ToLower(metaPath), ".json") {
			if json.Unmarshal(data, &meta) == nil {
				return meta
			}
			continue
		}
		// Minimal YAML support without adding another dependency here.
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(strings.ToLower(parts[0]))
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, `"'`)
			switch key {
			case "name":
				meta.Name = value
			case "description":
				meta.Description = value
			case "mode":
				meta.Mode = strings.ToLower(value)
			case "tags":
				value = strings.Trim(value, "[]")
				for _, item := range strings.Split(value, ",") {
					item = strings.TrimSpace(strings.Trim(item, `"'`))
					if item == "" {
						continue
					}
					meta.Tags = append(meta.Tags, item)
				}
			case "priority":
				if priority, err := strconv.Atoi(value); err == nil {
					meta.Priority = priority
				}
			}
		}
		return meta
	}
	return candidateMetadata{}
}

func metadataPathsForTemplate(path string) []string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	return []string{
		base + ".meta.json",
		base + ".meta.yaml",
		base + ".meta.yml",
		path + ".meta.json",
		path + ".meta.yaml",
		path + ".meta.yml",
	}
}

func scoreCandidate(mode core.Mode, path, content string, metadata candidateMetadata, contextTokens []string) (int, string, bool) {
	lowerName := strings.ToLower(filepath.Base(path))
	lowerContent := strings.ToLower(content)
	metadataText := strings.ToLower(strings.Join([]string{
		metadata.Name,
		metadata.Description,
		strings.Join(metadata.Tags, " "),
	}, " "))
	searchText := strings.Join([]string{lowerName, lowerContent, metadataText}, "\n")
	score := metadata.Priority * 100
	var reasons []string

	modeScore, compatible := modeCompatibility(mode, lowerName, searchText, strings.ToLower(metadata.Mode))
	if !compatible {
		return 0, "", false
	}
	score += modeScore
	if modeScore > 0 {
		reasons = append(reasons, "mode match")
	}

	tokenHits := 0
	for _, token := range contextTokens {
		if token == "" {
			continue
		}
		if strings.Contains(searchText, token) {
			tokenHits++
		}
	}
	if tokenHits > 0 {
		score += minInt(tokenHits*8, 80)
		reasons = append(reasons, fmt.Sprintf("%d context hits", tokenHits))
	}

	if metadata.Priority != 0 {
		reasons = append(reasons, fmt.Sprintf("priority %d", metadata.Priority))
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "generic template match")
	}
	return score, strings.Join(reasons, ", "), true
}

func modeCompatibility(mode core.Mode, lowerName, lowerContent, metaMode string) (int, bool) {
	switch mode {
	case core.ModeInject:
		if metaMode != "" && metaMode != "inject" {
			return 0, false
		}
		if containsAnyString(lowerName, "assessment", "finding") || containsAnyString(lowerContent, `"executive_summary"`, "## executive summary", "draft.findings", ".draft.findings") {
			return 0, false
		}
		score := 40
		if containsAnyString(lowerName, "inject", "scenario", "exercise") || containsAnyString(lowerContent, "scenario_summary", "expected_action", "draft.items", ".draft.items", "inject") {
			score += 180
		}
		return score, true
	default:
		if metaMode != "" && metaMode != "assess" && metaMode != "assessment" {
			return 0, false
		}
		if containsAnyString(lowerName, "inject", "scenario", "exercise") || containsAnyString(lowerContent, "scenario_summary", "expected_action", "draft.items", ".draft.items") {
			return 0, false
		}
		score := 40
		if containsAnyString(lowerName, "assess", "assessment", "report") || containsAnyString(lowerContent, "executive_summary", "draft.findings", ".draft.findings", "remediation", "impact") {
			score += 180
		}
		return score, true
	}
}

func selectionTokens(cfg core.Config, observations []core.Observation) []string {
	var tokens []string
	source := strings.ToLower(strings.Join([]string{
		cfg.Title,
		cfg.Client,
		cfg.Environment,
		cfg.Classification,
		cfg.Instructions,
	}, " "))
	for _, field := range strings.Fields(source) {
		field = strings.Trim(field, ".,:;()[]{}<>/\\")
		if len(field) < 4 {
			continue
		}
		tokens = append(tokens, field)
	}
	for _, observation := range observations {
		for _, field := range strings.Fields(strings.ToLower(observation.Title + " " + observation.Category)) {
			field = strings.Trim(field, ".,:;()[]{}<>/\\")
			if len(field) < 4 {
				continue
			}
			tokens = append(tokens, field)
		}
	}
	return uniqueStrings(tokens)
}

func containsAnyString(input string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(input, needle) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type templateEngine string

const (
	templateEngineGo    templateEngine = "go"
	templateEngineJinja templateEngine = "jinja"
)

func templateEngineForPath(path string) templateEngine {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".j2", ".jinja", ".jinja2":
		return templateEngineJinja
	default:
		return templateEngineGo
	}
}

func renderJinja(data any, source string) ([]byte, error) {
	tmpl, err := pongo2.FromString(source)
	if err != nil {
		return nil, err
	}
	context, err := toPongoContext(data)
	if err != nil {
		return nil, err
	}
	rendered, err := tmpl.Execute(context)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

func toPongoContext(data any) (pongo2.Context, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var context pongo2.Context
	if err := json.Unmarshal(raw, &context); err != nil {
		return nil, err
	}
	return context, nil
}

func funcMap() texttemplate.FuncMap {
	return texttemplate.FuncMap{
		"join": func(items []string, sep string) string {
			return strings.Join(items, sep)
		},
	}
}
