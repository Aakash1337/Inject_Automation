package normalize

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"reflect"
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
