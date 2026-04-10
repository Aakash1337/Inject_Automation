package modelprofile

import "strings"

type Profile struct {
	Name          string
	Model         string
	FallbackModel string
	Description   string
}

var profiles = map[string]Profile{
	"fast": {
		Name:          "fast",
		Model:         "gemma4:e4b",
		FallbackModel: "gemma4:e2b",
		Description:   "Lower-latency local drafting with smaller Gemma 4 models.",
	},
	"balanced": {
		Name:          "balanced",
		Model:         "gemma4:26b",
		FallbackModel: "gemma4:e4b",
		Description:   "Default balance of quality and resource usage.",
	},
	"quality": {
		Name:          "quality",
		Model:         "gemma4:31b",
		FallbackModel: "gemma4:26b",
		Description:   "Best local quality with higher resource requirements.",
	},
}

func Resolve(name string) (Profile, bool) {
	normalized := Normalize(name)
	profile, ok := profiles[normalized]
	return profile, ok
}

func Normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
