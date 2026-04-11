package assess

import (
	"strings"
	"testing"

	"injectctl/internal/core"
)

func TestStabilizeDraftGeneratesFallbackFindings(t *testing.T) {
	t.Parallel()

	draft := core.AssessmentDraft{
		ExecutiveSummary: strings.Repeat("investigation into the board machine ", 8),
	}
	artifacts := []core.Artifact{
		{ID: "artifact-1"},
		{ID: "artifact-2"},
	}
	observations := []core.Observation{
		{
			ID:       "obs-network",
			Title:    "Indicators detected in screenshot",
			Detail:   "Ports: 22/tcp,80/tcp | Services: ssh,http",
			Category: "image",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "Structured indicators inferred from screenshot OCR"}},
		},
		{
			ID:       "obs-cred",
			Title:    "Credentials visible in screenshot",
			Detail:   "Screenshot content appears to expose reusable credentials or application secrets.",
			Category: "credential",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "Credential-like content visible in screenshot OCR"}},
		},
		{
			ID:       "obs-access",
			Title:    "Credential reuse or user access visible",
			Detail:   "Screenshot shows interactive access or user switching with reusable credentials.",
			Category: "access",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "User switching or credential reuse visible in screenshot OCR"}},
		},
		{
			ID:       "obs-priv",
			Title:    "Privilege escalation or root access visible",
			Detail:   "Screenshot indicates a privilege escalation path or root-level compromise was achieved.",
			Category: "privilege",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-2", Description: "Privilege escalation or root-access indicator visible in screenshot OCR"}},
		},
	}

	warnings := stabilizeDraft(&draft, artifacts, observations)
	if len(draft.Findings) < 3 {
		t.Fatalf("expected fallback findings, got %+v", draft.Findings)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected fallback warnings, got %v", warnings)
	}
	if strings.Contains(strings.ToLower(draft.ExecutiveSummary), "investigation into the board machine") {
		t.Fatalf("expected deterministic summary replacement, got %q", draft.ExecutiveSummary)
	}
}

func TestStabilizeDraftBuildsSpecificFindingsFromEvidence(t *testing.T) {
	t.Parallel()

	draft := core.AssessmentDraft{}
	artifacts := []core.Artifact{{ID: "artifact-1"}}
	observations := []core.Observation{
		{
			ID:       "obs-network",
			Title:    "Indicators detected in screenshot",
			Detail:   "IPs: 10.129.92.185 | Ports: 22/tcp,80/tcp | Services: ssh,http",
			Category: "image",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "Structured indicators inferred from screenshot OCR", Snippet: "Nmap scan report for 10.129.92.185 22/tcp open ssh 80/tcp open http"}},
		},
		{
			ID:       "obs-web",
			Title:    "Web login surface visible",
			Detail:   "Screenshot shows an accessible web application login surface or administrative interface.",
			Category: "web",
			Severity: "medium",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "Web login or application interface visible in screenshot OCR", Snippet: "http://crm.board.htb/ Dolibarr Login Password forgotten"}},
		},
		{
			ID:       "obs-cred",
			Title:    "Credentials visible in screenshot",
			Detail:   "Screenshot content appears to expose reusable credentials or application secrets.",
			Category: "credential",
			Severity: "high",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "Credential-like content visible in screenshot OCR", Snippet: "$dolibarr_main_db_user='dolibarrowner'; $dolibarr_main_db_pass='serverfun2$2023!!'; conf.php"}},
		},
		{
			ID:       "obs-access",
			Title:    "Credential reuse or user access visible",
			Detail:   "Screenshot shows interactive access or user switching with reusable credentials.",
			Category: "access",
			Severity: "high",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "User switching or credential reuse visible in screenshot OCR", Snippet: "su - larissa Password: serverfun2$2023!! ssh larissa@10.129.92.185"}},
		},
		{
			ID:       "obs-priv",
			Title:    "Privilege escalation or root access visible",
			Detail:   "Screenshot indicates a privilege escalation path or root-level compromise was achieved.",
			Category: "privilege",
			Severity: "critical",
			Evidence: []core.EvidenceExcerpt{{ArtifactID: "artifact-1", Description: "Privilege escalation or root-access indicator visible in screenshot OCR", Snippet: "CVE-2022-37706 Enjoy the root shell final flag and root access"}},
		},
	}

	stabilizeDraft(&draft, artifacts, observations)

	if len(draft.Findings) != 5 {
		t.Fatalf("expected 5 findings, got %d", len(draft.Findings))
	}
	joined := draft.ExecutiveSummary + "\n"
	for _, finding := range draft.Findings {
		joined += finding.Description + "\n" + finding.Impact + "\n" + finding.Remediation + "\n"
	}
	for _, needle := range []string{"10.129.92.185", "crm.board.htb", "dolibarrowner", "larissa", "CVE-2022-37706"} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("expected output to mention %q, got %q", needle, joined)
		}
	}
	if strings.Contains(joined, "serverfun2$2023") {
		t.Fatalf("expected output to avoid leaking raw password, got %q", joined)
	}
}
