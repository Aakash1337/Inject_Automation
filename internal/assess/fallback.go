package assess

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"injectctl/internal/core"
)

type fallbackRule struct {
	title       string
	severity    string
	description string
	impact      string
	remediation string
	match       func(core.Observation) bool
}

func stabilizeDraft(draft *core.AssessmentDraft, artifacts []core.Artifact, observations []core.Observation) []string {
	var warnings []string

	if len(draft.Findings) == 0 {
		findings := buildFallbackFindings(observations)
		if len(findings) > 0 {
			draft.Findings = findings
			warnings = append(warnings, "model returned no usable findings; generated evidence-linked fallback findings")
		}
	}

	if lowQualitySummary(draft.ExecutiveSummary) {
		draft.ExecutiveSummary = buildExecutiveSummary(draft.Findings, observations, artifacts)
		warnings = append(warnings, "model returned a low-quality executive summary; replaced with deterministic summary")
	}

	return warnings
}

func buildFallbackFindings(observations []core.Observation) []core.Finding {
	ctx := deriveFallbackContext(observations)
	rules := []fallbackRule{
		{
			title:       "Exposed Network Services",
			severity:    "medium",
			description: buildNetworkDescription(ctx),
			impact:      buildNetworkImpact(ctx),
			remediation: buildNetworkRemediation(ctx),
			match: func(observation core.Observation) bool {
				return strings.Contains(strings.ToLower(observation.Detail), "ports:") ||
					strings.Contains(strings.ToLower(observation.Detail), "open ssh") ||
					strings.Contains(strings.ToLower(observation.Detail), "open http")
			},
		},
		{
			title:       "Accessible Web Application Surface",
			severity:    "medium",
			description: buildWebDescription(ctx),
			impact:      buildWebImpact(ctx),
			remediation: buildWebRemediation(ctx),
			match: func(observation core.Observation) bool {
				return observation.Category == "web"
			},
		},
		{
			title:       "Credentials Exposed in Evidence",
			severity:    "high",
			description: buildCredentialDescription(ctx),
			impact:      buildCredentialImpact(ctx),
			remediation: buildCredentialRemediation(ctx),
			match: func(observation core.Observation) bool {
				return observation.Category == "credential"
			},
		},
		{
			title:       "Interactive Access Achieved with Captured Credentials",
			severity:    "high",
			description: buildAccessDescription(ctx),
			impact:      buildAccessImpact(ctx),
			remediation: buildAccessRemediation(ctx),
			match: func(observation core.Observation) bool {
				return observation.Category == "access"
			},
		},
		{
			title:       "Privilege Escalation Path to Root Observed",
			severity:    "critical",
			description: buildPrivilegeDescription(ctx),
			impact:      buildPrivilegeImpact(ctx),
			remediation: buildPrivilegeRemediation(ctx),
			match: func(observation core.Observation) bool {
				return observation.Category == "privilege"
			},
		},
	}

	var findings []core.Finding
	for _, rule := range rules {
		matched := matchingObservations(observations, rule.match)
		if len(matched) == 0 {
			continue
		}
		finding := core.Finding{
			Title:          rule.title,
			Severity:       rule.severity,
			Description:    rule.description,
			Impact:         rule.impact,
			Remediation:    rule.remediation,
			ObservationIDs: observationIDs(matched),
			EvidenceRefs:   evidenceRefs(matched),
		}
		findings = append(findings, finding)
	}
	return findings
}

func buildExecutiveSummary(findings []core.Finding, observations []core.Observation, artifacts []core.Artifact) string {
	if len(findings) == 0 {
		return fmt.Sprintf("Draft assessment generated from %d artifacts and %d observations. Analyst review is required because the model did not produce usable findings.", len(artifacts), len(observations))
	}

	ctx := deriveFallbackContext(observations)
	var summary []string
	head := fmt.Sprintf("Evidence from %d artifacts indicates %d primary security issues", len(artifacts), len(findings))
	if ctx.primaryIP != "" {
		head += " affecting " + ctx.primaryIP
	}
	summary = append(summary, head)
	if len(ctx.services) > 0 {
		summary = append(summary, "Initial exposure included "+joinNatural(ctx.services, 2)+" services")
	}
	if ctx.loginDomain != "" {
		web := "A reachable web surface was identified at " + ctx.loginDomain
		if len(ctx.products) > 0 {
			web += " running " + joinNatural(ctx.products, 1)
		}
		summary = append(summary, web)
	}
	if ctx.dbUser != "" {
		summary = append(summary, "Configuration evidence exposed reusable credentials associated with "+ctx.dbUser)
	}
	if ctx.accessUser != "" {
		summary = append(summary, "Those credentials were then used to obtain access as "+ctx.accessUser)
	}
	if len(ctx.vulns) > 0 {
		summary = append(summary, "Privilege escalation to root was observed via "+joinNatural(ctx.vulns, 1))
	} else if ctx.rootObserved {
		summary = append(summary, "Privilege escalation to root was observed")
	}
	return strings.Join(summary, ". ") + ". Analyst review is required before distribution."
}

func lowQualitySummary(summary string) bool {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return true
	}
	if len([]rune(trimmed)) < 30 {
		return true
	}
	words := strings.Fields(strings.ToLower(trimmed))
	if len(words) < 12 {
		return false
	}

	repeated := map[string]int{}
	for i := 0; i+3 < len(words); i++ {
		ngram := strings.Join(words[i:i+4], " ")
		repeated[ngram]++
		if repeated[ngram] >= 3 {
			return true
		}
	}
	return false
}

func matchingObservations(observations []core.Observation, match func(core.Observation) bool) []core.Observation {
	var matched []core.Observation
	for _, observation := range observations {
		if match(observation) {
			matched = append(matched, observation)
		}
	}
	return matched
}

func observationIDs(observations []core.Observation) []string {
	ids := make([]string, 0, len(observations))
	seen := map[string]struct{}{}
	for _, observation := range observations {
		if observation.ID == "" {
			continue
		}
		if _, ok := seen[observation.ID]; ok {
			continue
		}
		seen[observation.ID] = struct{}{}
		ids = append(ids, observation.ID)
	}
	sort.Strings(ids)
	return ids
}

func evidenceRefs(observations []core.Observation) []string {
	var refs []string
	seen := map[string]struct{}{}
	for _, observation := range observations {
		for _, evidence := range observation.Evidence {
			ref := evidence.ArtifactID + ":" + evidence.Description
			if evidence.ArtifactID == "" || evidence.Description == "" {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}

type fallbackContext struct {
	primaryIP     string
	ports         []string
	services      []string
	domains       []string
	loginDomain   string
	products      []string
	usernames     []string
	dbUser        string
	accessUser    string
	configFiles   []string
	vulns         []string
	credentialHit bool
	accessHit     bool
	rootObserved  bool
}

var (
	fallbackIPPattern      = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	fallbackPortPattern    = regexp.MustCompile(`\b\d{1,5}/(?:tcp|udp)\b`)
	fallbackDomainPattern  = regexp.MustCompile(`\b(?:[a-z0-9-]+\.)+[a-z]{2,}\b`)
	fallbackSuUserPattern  = regexp.MustCompile(`(?i)\bsu\s*-\s*([a-z0-9._-]+)`)
	fallbackSSHUserPattern = regexp.MustCompile(`(?i)\bssh\s+([a-z0-9._-]+)@`)
	fallbackConfigPattern  = regexp.MustCompile(`(?i)\bconf\.php\b`)
	fallbackCVEPattern     = regexp.MustCompile(`(?i)\bCVE-\d{4}-\d+\b`)
	fallbackOpenPattern    = regexp.MustCompile(`(?i)\bopen\s+(ssh|http|https|mysql)\b`)
	fallbackServicesField  = regexp.MustCompile(`(?i)services:\s*([a-z0-9,/_ -]+)`)
	fallbackDbUserPattern  = regexp.MustCompile(`(?i)dolibarr_main_db_user\s*=\s*['"]?([a-z0-9._-]+)`)
)

func deriveFallbackContext(observations []core.Observation) fallbackContext {
	ctx := fallbackContext{}
	serviceSet := map[string]struct{}{}
	portSet := map[string]struct{}{}
	domainSet := map[string]struct{}{}
	productSet := map[string]struct{}{}
	userSet := map[string]struct{}{}
	configSet := map[string]struct{}{}
	vulnSet := map[string]struct{}{}

	for _, observation := range observations {
		lowerCategory := strings.ToLower(observation.Category)
		if lowerCategory == "credential" {
			ctx.credentialHit = true
		}
		if lowerCategory == "access" {
			ctx.accessHit = true
		}
		if lowerCategory == "privilege" {
			ctx.rootObserved = true
		}

		for _, chunk := range observationTextChunks(observation) {
			lowerChunk := strings.ToLower(chunk)
			if ctx.primaryIP == "" {
				if ip := firstPatternMatch(fallbackIPPattern, chunk); ip != "" {
					ctx.primaryIP = ip
				}
			}
			for _, value := range fallbackPortPattern.FindAllString(chunk, -1) {
				portSet[strings.ToLower(value)] = struct{}{}
			}
			for _, value := range fallbackDomainPattern.FindAllString(lowerChunk, -1) {
				if strings.HasSuffix(value, ".png") || strings.HasSuffix(value, ".txt") || strings.Contains(value, "raw.github") || strings.Contains(value, "nmap.org") || strings.HasPrefix(value, "fuzz.") {
					continue
				}
				domainSet[value] = struct{}{}
			}
			if (strings.Contains(lowerChunk, "ports:") || strings.Contains(lowerChunk, "open ")) && len(fallbackServicesField.FindStringSubmatch(lowerChunk)) > 1 {
				match := fallbackServicesField.FindStringSubmatch(lowerChunk)
				for _, value := range strings.Split(match[1], ",") {
					if normalized := normalizeService(value); normalized != "" {
						serviceSet[normalized] = struct{}{}
					}
				}
			}
			for _, match := range fallbackOpenPattern.FindAllStringSubmatch(lowerChunk, -1) {
				if len(match) > 1 {
					if normalized := normalizeService(match[1]); normalized != "" {
						serviceSet[normalized] = struct{}{}
					}
				}
			}
			for _, match := range fallbackSuUserPattern.FindAllStringSubmatch(lowerChunk, -1) {
				if len(match) > 1 {
					userSet[match[1]] = struct{}{}
					if ctx.accessUser == "" {
						ctx.accessUser = match[1]
					}
				}
			}
			for _, match := range fallbackSSHUserPattern.FindAllStringSubmatch(lowerChunk, -1) {
				if len(match) > 1 {
					userSet[match[1]] = struct{}{}
					if ctx.accessUser == "" {
						ctx.accessUser = match[1]
					}
				}
			}
			if match := fallbackDbUserPattern.FindStringSubmatch(lowerChunk); len(match) > 1 {
				userSet[match[1]] = struct{}{}
				if ctx.dbUser == "" {
					ctx.dbUser = match[1]
				}
			}
			for _, value := range fallbackConfigPattern.FindAllString(chunk, -1) {
				configSet[strings.ToLower(value)] = struct{}{}
			}
			for _, value := range fallbackCVEPattern.FindAllString(chunk, -1) {
				vulnSet[strings.ToUpper(value)] = struct{}{}
				ctx.rootObserved = true
			}
			if strings.Contains(lowerChunk, "root shell") || strings.Contains(lowerChunk, "cat root.txt") || strings.Contains(lowerChunk, "final flag and root access") {
				ctx.rootObserved = true
			}
			if strings.Contains(lowerChunk, "dolibarr") {
				productSet["dolibarr"] = struct{}{}
			}
			if ctx.loginDomain == "" && (strings.Contains(lowerChunk, "password forgotten") || strings.Contains(lowerChunk, " login") || strings.Contains(lowerChunk, "dolibarr")) {
				if domain := firstPreferredDomain(sortedKeys(domainSet)); domain != "" {
					ctx.loginDomain = domain
				}
			}
		}
	}

	ctx.ports = sortedKeys(portSet)
	ctx.services = sortedKeys(serviceSet)
	ctx.domains = preferredDomains(sortedKeys(domainSet))
	ctx.products = sortedKeys(productSet)
	ctx.usernames = sortedKeys(userSet)
	ctx.configFiles = sortedKeys(configSet)
	ctx.vulns = collapsePrefixes(sortedKeys(vulnSet))
	if ctx.loginDomain == "" {
		ctx.loginDomain = firstPreferredDomain(ctx.domains)
	}
	return ctx
}

func buildNetworkDescription(ctx fallbackContext) string {
	parts := []string{"Enumeration evidence shows externally reachable network services on the assessed host"}
	if ctx.primaryIP != "" {
		parts[0] += " " + ctx.primaryIP
	}
	if len(ctx.ports) > 0 || len(ctx.services) > 0 {
		parts = append(parts, "Observed exposure includes "+joinExposure(ctx.ports, ctx.services))
	}
	return strings.Join(parts, ". ") + "."
}

func buildNetworkImpact(ctx fallbackContext) string {
	if len(ctx.services) > 0 {
		return fmt.Sprintf("Reachable %s services provide the initial foothold for follow-on enumeration and application-layer compromise.", joinNatural(ctx.services, 2))
	}
	return "Internet-facing or reachable services increase the attack surface and enable further exploitation."
}

func buildNetworkRemediation(ctx fallbackContext) string {
	if ctx.primaryIP != "" {
		return fmt.Sprintf("Review exposure on %s, restrict access to required management paths only, and validate service hardening for the identified ports and protocols.", ctx.primaryIP)
	}
	return "Limit unnecessary exposure, restrict access, and validate service hardening on the identified hosts and ports."
}

func buildWebDescription(ctx fallbackContext) string {
	var parts []string
	if ctx.loginDomain != "" {
		parts = append(parts, "Evidence shows a reachable web application surface at "+ctx.loginDomain)
	} else if len(ctx.domains) > 0 {
		parts = append(parts, "Evidence shows reachable web application surfaces on "+joinNatural(ctx.domains, 2))
	} else {
		parts = append(parts, "Evidence shows a reachable web application surface that could support follow-on access")
	}
	if len(ctx.products) > 0 {
		parts = append(parts, "The observed application appears to include "+joinNatural(ctx.products, 1))
	}
	if len(ctx.domains) > 1 {
		var alternates []string
		for _, domain := range ctx.domains {
			if domain != ctx.loginDomain {
				alternates = append(alternates, domain)
			}
		}
		if len(alternates) > 0 {
			parts = append(parts, "Related virtual-host exposure was also identified on "+alternates[0])
		}
	}
	return strings.Join(parts, ". ") + "."
}

func buildWebImpact(ctx fallbackContext) string {
	if len(ctx.domains) > 1 {
		return "Multiple reachable web surfaces increase the opportunity for credential attacks, application abuse, and follow-on exploitation."
	}
	return "Publicly reachable application surfaces increase the opportunity for credential attacks, exploitation, and sensitive data exposure."
}

func buildWebRemediation(ctx fallbackContext) string {
	if ctx.loginDomain != "" {
		return fmt.Sprintf("Review exposure and authentication controls for %s, remove unnecessary virtual hosts, and validate patching and access restrictions for the associated application surface.", ctx.loginDomain)
	}
	return "Review exposed web applications and virtual hosts, restrict access where appropriate, and validate authentication and patching controls."
}

func buildCredentialDescription(ctx fallbackContext) string {
	parts := []string{"Evidence indicates reusable credentials or application secrets were exposed"}
	if len(ctx.configFiles) > 0 {
		parts[0] += " in " + joinNatural(ctx.configFiles, 2)
	}
	if ctx.dbUser != "" {
		parts = append(parts, "The exposed database or application account context includes "+ctx.dbUser)
	}
	return strings.Join(parts, ". ") + "."
}

func buildCredentialImpact(ctx fallbackContext) string {
	if ctx.dbUser != "" {
		return fmt.Sprintf("Exposed credentials tied to %s can enable unauthorized access, lateral movement, and privilege escalation if the same secret is reused across services or accounts.", ctx.dbUser)
	}
	return "Exposed credentials can enable unauthorized access, lateral movement, and privilege escalation if reused elsewhere."
}

func buildCredentialRemediation(ctx fallbackContext) string {
	return "Rotate the exposed credentials immediately, remove secrets from accessible files and screenshots, and move sensitive values into a controlled secret-management workflow."
}

func buildAccessDescription(ctx fallbackContext) string {
	parts := []string{"Evidence shows interactive access was achieved using captured credentials"}
	if ctx.accessUser != "" {
		parts[0] += " for " + ctx.accessUser
	}
	if ctx.primaryIP != "" {
		parts = append(parts, "The access path includes SSH connectivity to "+ctx.primaryIP)
	}
	return strings.Join(parts, ". ") + "."
}

func buildAccessImpact(ctx fallbackContext) string {
	return "Successful reuse of discovered credentials demonstrates that the compromise progressed beyond information disclosure into authenticated host access."
}

func buildAccessRemediation(ctx fallbackContext) string {
	return "Reset affected accounts, review credential reuse, add MFA where possible, and investigate whether the same credentials provide access to other internal systems."
}

func buildPrivilegeDescription(ctx fallbackContext) string {
	parts := []string{"Evidence indicates local privilege escalation to root was achieved on the target host"}
	if len(ctx.vulns) > 0 {
		parts = append(parts, "The observed escalation path references "+joinNatural(ctx.vulns, 1))
	}
	return strings.Join(parts, ". ") + "."
}

func buildPrivilegeImpact(ctx fallbackContext) string {
	return "Root-level compromise implies full host control, enabling persistence, credential theft, tampering, and broad data access."
}

func buildPrivilegeRemediation(ctx fallbackContext) string {
	if len(ctx.vulns) > 0 {
		return fmt.Sprintf("Patch and verify the issue associated with %s, remove attacker tooling, rotate impacted credentials, and perform full host containment and forensic review.", joinNatural(ctx.vulns, 1))
	}
	return "Patch the vulnerable component, remove attacker tooling, rotate impacted credentials, and perform full host containment and forensic review."
}

func observationTextChunks(observation core.Observation) []string {
	chunks := []string{observation.Detail}
	for _, evidence := range observation.Evidence {
		chunks = append(chunks, evidence.Snippet, evidence.Description)
	}
	return chunks
}

func firstPatternMatch(pattern *regexp.Regexp, input string) string {
	return strings.TrimSpace(pattern.FindString(input))
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func preferredDomains(domains []string) []string {
	var preferred []string
	var others []string
	for _, domain := range domains {
		switch {
		case strings.Contains(domain, "crm.") || strings.Contains(domain, ".htb"):
			preferred = append(preferred, domain)
		default:
			others = append(others, domain)
		}
	}
	if len(preferred) > 0 {
		return append(preferred, others...)
	}
	return domains
}

func firstPreferredDomain(domains []string) string {
	for _, domain := range domains {
		if strings.Contains(domain, "crm.") {
			return domain
		}
	}
	if len(domains) > 0 {
		return domains[0]
	}
	return ""
}

func joinExposure(ports, services []string) string {
	var parts []string
	if len(ports) > 0 {
		parts = append(parts, "ports "+joinNatural(ports, 3))
	}
	if len(services) > 0 {
		parts = append(parts, "services "+joinNatural(services, 3))
	}
	return strings.Join(parts, " with ")
}

func joinNatural(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if limit > 0 && len(values) > limit {
		values = append(append([]string{}, values[:limit]...), fmt.Sprintf("%d more", len(values)-limit))
	}
	switch len(values) {
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
	}
}

func normalizeService(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ssh", "openssh":
		return "ssh"
	case "http", "apache":
		return "http"
	case "mysql":
		return "mysql"
	default:
		return ""
	}
}

func collapsePrefixes(values []string) []string {
	var filtered []string
	for _, value := range values {
		skip := false
		for _, other := range values {
			if value == other {
				continue
			}
			if strings.HasPrefix(other, value) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, value)
		}
	}
	if len(filtered) == 0 {
		return values
	}
	return filtered
}
