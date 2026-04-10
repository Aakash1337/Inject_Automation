package core

import "time"

type Mode string

const (
	ModeAssess Mode = "assess"
	ModeInject Mode = "inject"
)

type Config struct {
	Mode           Mode         `json:"mode" yaml:"mode"`
	Title          string       `json:"title" yaml:"title"`
	Client         string       `json:"client" yaml:"client"`
	Environment    string       `json:"environment" yaml:"environment"`
	Classification string       `json:"classification" yaml:"classification"`
	Instructions   string       `json:"instructions" yaml:"instructions"`
	Template       string       `json:"template" yaml:"template"`
	Artifacts      []string     `json:"artifacts" yaml:"artifacts"`
	AI             AIConfig     `json:"ai" yaml:"ai"`
	Output         OutputConfig `json:"output" yaml:"output"`
}

type AIConfig struct {
	Provider              string  `json:"provider" yaml:"provider"`
	Endpoint              string  `json:"endpoint" yaml:"endpoint"`
	Profile               string  `json:"profile,omitempty" yaml:"profile,omitempty"`
	Model                 string  `json:"model" yaml:"model"`
	FallbackModel         string  `json:"fallback_model" yaml:"fallback_model"`
	Temperature           float64 `json:"temperature" yaml:"temperature"`
	MaxTokens             int     `json:"max_tokens" yaml:"max_tokens"`
	TimeoutSeconds        int     `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	PromptDir             string  `json:"prompt_dir,omitempty" yaml:"prompt_dir,omitempty"`
	MaxPromptArtifacts    int     `json:"max_prompt_artifacts,omitempty" yaml:"max_prompt_artifacts,omitempty"`
	MaxPromptObservations int     `json:"max_prompt_observations,omitempty" yaml:"max_prompt_observations,omitempty"`
}

type OutputConfig struct {
	Formats        []string `json:"formats" yaml:"formats"`
	ProjectDir     string   `json:"project_dir" yaml:"project_dir"`
	RedactionRules []string `json:"redaction_rules" yaml:"redaction_rules"`
}

type ArtifactKind string

const (
	ArtifactImage  ArtifactKind = "image"
	ArtifactText   ArtifactKind = "text"
	ArtifactNmap   ArtifactKind = "nmap"
	ArtifactNessus ArtifactKind = "nessus"
	ArtifactOther  ArtifactKind = "other"
)

type Artifact struct {
	ID            string            `json:"id"`
	Path          string            `json:"path"`
	Kind          ArtifactKind      `json:"kind"`
	MIME          string            `json:"mime"`
	SHA256        string            `json:"sha256"`
	Parser        string            `json:"parser"`
	ExtractedText string            `json:"extracted_text,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type EvidenceExcerpt struct {
	ArtifactID  string  `json:"artifact_id"`
	Snippet     string  `json:"snippet"`
	Location    string  `json:"location"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description,omitempty"`
}

type Observation struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Detail     string            `json:"detail"`
	Category   string            `json:"category"`
	Severity   string            `json:"severity,omitempty"`
	Confidence float64           `json:"confidence"`
	Source     map[string]string `json:"source,omitempty"`
	Evidence   []EvidenceExcerpt `json:"evidence"`
}

type Finding struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Severity       string   `json:"severity"`
	Description    string   `json:"description"`
	Impact         string   `json:"impact"`
	Remediation    string   `json:"remediation"`
	EvidenceRefs   []string `json:"evidence_refs"`
	ObservationIDs []string `json:"observation_ids,omitempty"`
}

type InjectItem struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Audience       string   `json:"audience"`
	Channel        string   `json:"channel"`
	Trigger        string   `json:"trigger"`
	ExpectedAction string   `json:"expected_action"`
	Body           string   `json:"body"`
	EvidenceRefs   []string `json:"evidence_refs"`
	ObservationIDs []string `json:"observation_ids,omitempty"`
}

type RunRecord struct {
	RunID       string        `json:"run_id"`
	Mode        Mode          `json:"mode"`
	Title       string        `json:"title"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
	Model       string        `json:"model"`
	Warnings    []string      `json:"warnings,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
}

type ErrorReport struct {
	Stage           string    `json:"stage"`
	Message         string    `json:"message"`
	GeneratedAt     time.Time `json:"generated_at"`
	Recommendations []string  `json:"recommendations,omitempty"`
}

type EvidenceIndex struct {
	RunID   string               `json:"run_id"`
	Mode    Mode                 `json:"mode"`
	Entries []EvidenceIndexEntry `json:"entries"`
}

type EvidenceIndexEntry struct {
	ArtifactID     string   `json:"artifact_id"`
	Path           string   `json:"path"`
	Kind           string   `json:"kind"`
	Parser         string   `json:"parser"`
	MIME           string   `json:"mime"`
	SHA256         string   `json:"sha256"`
	ObservationIDs []string `json:"observation_ids,omitempty"`
	Snippets       []string `json:"snippets,omitempty"`
}

type AssessmentDraft struct {
	ExecutiveSummary string    `json:"executive_summary"`
	Findings         []Finding `json:"findings"`
}

type InjectDraft struct {
	ScenarioSummary string       `json:"scenario_summary"`
	Items           []InjectItem `json:"items"`
}

type AssessmentResult struct {
	Run          RunRecord       `json:"run"`
	Status       string          `json:"status"`
	Config       Config          `json:"config"`
	Artifacts    []Artifact      `json:"artifacts"`
	Observations []Observation   `json:"observations"`
	Draft        AssessmentDraft `json:"draft"`
	ErrorReport  *ErrorReport    `json:"error_report,omitempty"`
}

type InjectResult struct {
	Run          RunRecord     `json:"run"`
	Status       string        `json:"status"`
	Config       Config        `json:"config"`
	Artifacts    []Artifact    `json:"artifacts"`
	Observations []Observation `json:"observations"`
	Draft        InjectDraft   `json:"draft"`
	ErrorReport  *ErrorReport  `json:"error_report,omitempty"`
}

type DoctorStatus struct {
	Endpoint        string   `json:"endpoint"`
	Profile         string   `json:"profile,omitempty"`
	PrimaryModel    string   `json:"primary_model,omitempty"`
	FallbackModel   string   `json:"fallback_model,omitempty"`
	OllamaReachable bool     `json:"ollama_reachable"`
	InstalledModels []string `json:"installed_models,omitempty"`
	SmokeTestModel  string   `json:"smoke_test_model,omitempty"`
	SmokeTestPassed bool     `json:"smoke_test_passed,omitempty"`
	OCRAvailable    bool     `json:"ocr_available"`
	Warnings        []string `json:"warnings,omitempty"`
	Errors          []string `json:"errors,omitempty"`
}
