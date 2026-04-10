package normalize

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
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
	out := in
	for _, rule := range rules {
		if strings.TrimSpace(rule) == "" {
			continue
		}
		pattern, err := regexp.Compile(rule)
		if err != nil {
			out = strings.ReplaceAll(out, rule, "[REDACTED]")
			continue
		}
		out = pattern.ReplaceAllString(out, "[REDACTED]")
	}
	return out
}

func stableID(in string) string {
	sum := sha1.Sum([]byte(in))
	return hex.EncodeToString(sum[:])[:12]
}
