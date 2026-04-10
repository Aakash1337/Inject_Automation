package ollama

import "strings"

func ContainsModel(models []string, target string) bool {
	for _, model := range models {
		if strings.EqualFold(model, target) || strings.HasPrefix(strings.ToLower(model), strings.ToLower(target)+":") {
			return true
		}
	}
	return false
}
