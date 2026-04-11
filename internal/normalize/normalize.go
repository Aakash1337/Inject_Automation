package normalize

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"injectctl/internal/core"
)

func Observations(in []core.Observation) []core.Observation {
	seen := map[string]struct{}{}
	var out []core.Observation
	for _, observation := range in {
		observation.Title = strings.TrimSpace(observation.Title)
		observation.Detail = strings.TrimSpace(observation.Detail)
		if observation.Title == "" && observation.Detail == "" {
			continue
		}
		if observation.Confidence == 0 {
			observation.Confidence = 0.7
		}
		if observation.ID == "" {
			observation.ID = stableID(observation.Title + "|" + observation.Detail)
		}
		key := strings.ToLower(observation.Title + "|" + observation.Detail)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, observation)
	}
	return out
}

func ConflictWarnings(observations []core.Observation) []string {
	type conflictGroup struct {
		details    map[string]struct{}
		severities map[string]struct{}
		count      int
	}

	groups := map[string]*conflictGroup{}
	for _, observation := range observations {
		key := conflictKey(observation)
		if key == "" {
			continue
		}
		group := groups[key]
		if group == nil {
			group = &conflictGroup{
				details:    map[string]struct{}{},
				severities: map[string]struct{}{},
			}
			groups[key] = group
		}
		group.count++
		group.details[normalizeText(observation.Detail)] = struct{}{}
		if severity := normalizeText(observation.Severity); severity != "" {
			group.severities[severity] = struct{}{}
		}
	}

	var warnings []string
	keys := make([]string, 0, len(groups))
	for key, group := range groups {
		if group.count < 2 {
			continue
		}
		if len(group.details) < 2 && len(group.severities) < 2 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		subject, title, category := parseConflictKey(key)
		warnings = append(warnings, fmt.Sprintf("potentially conflicting observations detected for %s [%s/%s]", subject, category, title))
	}
	return warnings
}

func RedactString(in string, rules []string) string {
	compiled, _ := compileRules(rules)
	return compiled.apply(in)
}

func RedactAssessmentResult(result *core.AssessmentResult, rules []string) []string {
	compiled, warnings := compileRules(rules)
	redactValue(reflect.ValueOf(result), compiled)
	return warnings
}

func RedactInjectResult(result *core.InjectResult, rules []string) []string {
	compiled, warnings := compileRules(rules)
	redactValue(reflect.ValueOf(result), compiled)
	return warnings
}

type compiledRules struct {
	patterns []*regexp.Regexp
	literals []string
}

func compileRules(rules []string) (compiledRules, []string) {
	var compiled compiledRules
	var warnings []string
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		pattern, err := regexp.Compile(rule)
		if err != nil {
			compiled.literals = append(compiled.literals, rule)
			warnings = append(warnings, "invalid redaction regex treated as literal: "+rule)
			continue
		}
		compiled.patterns = append(compiled.patterns, pattern)
	}
	return compiled, warnings
}

func (c compiledRules) apply(in string) string {
	out := in
	for _, literal := range c.literals {
		out = strings.ReplaceAll(out, literal, "[REDACTED]")
	}
	for _, pattern := range c.patterns {
		out = pattern.ReplaceAllString(out, "[REDACTED]")
	}
	return out
}

func redactValue(v reflect.Value, rules compiledRules) {
	if !v.IsValid() {
		return
	}

	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			redactValue(v.Elem(), rules)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			redactValue(v.Field(i), rules)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			redactValue(v.Index(i), rules)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			value := iter.Value()
			switch value.Kind() {
			case reflect.String:
				v.SetMapIndex(iter.Key(), reflect.ValueOf(rules.apply(value.String())))
			case reflect.Pointer:
				if !value.IsNil() {
					clone := reflect.New(value.Type().Elem())
					clone.Elem().Set(value.Elem())
					redactValue(clone, rules)
					v.SetMapIndex(iter.Key(), clone)
				}
			case reflect.Struct:
				clone := reflect.New(value.Type()).Elem()
				clone.Set(value)
				redactValue(clone, rules)
				v.SetMapIndex(iter.Key(), clone)
			case reflect.Slice, reflect.Array:
				clone := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
				reflect.Copy(clone, value)
				redactValue(clone, rules)
				v.SetMapIndex(iter.Key(), clone)
			}
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(rules.apply(v.String()))
		}
	}
}

func stableID(in string) string {
	sum := sha1.Sum([]byte(in))
	return hex.EncodeToString(sum[:])[:12]
}

func conflictKey(observation core.Observation) string {
	title := normalizeText(observation.Title)
	category := normalizeText(observation.Category)
	subject := observationSubject(observation)
	if title == "" || category == "" || subject == "" {
		return ""
	}
	return subject + "|" + title + "|" + category
}

func parseConflictKey(key string) (subject, title, category string) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return key, "", ""
	}
	return parts[0], parts[1], parts[2]
}

func observationSubject(observation core.Observation) string {
	tokens := strings.Fields(strings.TrimSpace(observation.Detail))
	host := normalizeText(observation.Source["host"])

	if host != "" {
		if len(tokens) == 0 {
			return host
		}
		first := normalizeText(tokens[0])
		switch {
		case strings.HasPrefix(first, host+":"):
			return first
		case first == host && len(tokens) > 1 && looksPortToken(tokens[1]):
			return host + " " + normalizeText(tokens[1])
		default:
			return host
		}
	}

	if len(tokens) == 0 {
		return ""
	}
	first := normalizeText(tokens[0])
	if looksPortToken(first) || strings.Contains(first, ":") {
		return first
	}
	if surface := normalizeText(observation.Source["surface"]); surface != "" {
		return surface + " " + first
	}
	return ""
}

func looksPortToken(token string) bool {
	return strings.Contains(token, "/") || strings.Contains(token, ":")
}

func normalizeText(in string) string {
	return strings.ToLower(strings.TrimSpace(in))
}
