package image

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"regexp"
	"strconv"
	"strings"

	"injectctl/internal/core"
	"injectctl/internal/ocr"
)

var (
	ipPattern      = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	portPattern    = regexp.MustCompile(`\b\d{1,5}/(?:tcp|udp)\b`)
	servicePattern = regexp.MustCompile(`(?i)\b(ssh|http|https|rdp|smb|dns|ftp|smtp|imap|pop3|ldap|kerberos|winrm|mysql|mssql|postgres|postgresql|oracle|vnc|telnet|snmp)\b`)
	commandPattern = regexp.MustCompile(`(?i)\b(nmap|ssh|curl|wget|powershell|pwsh|cmd(?:\.exe)?|netstat|ss|nc|ncat|ping|tracert|traceroute|nslookup|whoami|ipconfig|ifconfig)\b`)
)

func Parse(ctx context.Context, artifact core.Artifact, engine ocr.Engine) (core.Artifact, []core.Observation, []string, error) {
	artifact.Parser = "image"
	artifact.Metadata = mergeMetadata(artifact.Metadata, readImageMetadata(artifact.Path))

	if engine == nil || !engine.Available() {
		artifact.Metadata["ocr_status"] = "unavailable"
		return artifact, nil, []string{"tesseract not available; continuing without OCR"}, nil
	}

	text, err := engine.ExtractText(ctx, artifact.Path)
	if err != nil {
		artifact.Metadata["ocr_status"] = "failed"
		return artifact, nil, []string{"image OCR failed; continuing with multimodal synthesis only"}, nil
	}

	artifact.ExtractedText = truncate(text, 10000)
	if strings.TrimSpace(text) == "" {
		artifact.Metadata["ocr_status"] = "empty"
		return artifact, nil, nil, nil
	}

	artifact.Metadata = mergeMetadata(artifact.Metadata, analyzeOCRText(text))

	observations := []core.Observation{
		{
			Title:      "Screenshot text extracted",
			Detail:     truncate(text, 600),
			Category:   "image",
			Confidence: 0.82,
			Evidence: []core.EvidenceExcerpt{
				{
					ArtifactID:  artifact.ID,
					Snippet:     truncate(text, 350),
					Location:    artifact.Path,
					Confidence:  0.82,
					Description: "OCR text extracted from screenshot",
				},
			},
		},
	}

	if summary := buildIndicatorSummary(artifact.Metadata); summary != "" {
		observations = append(observations, core.Observation{
			Title:      "Indicators detected in screenshot",
			Detail:     summary,
			Category:   "image",
			Confidence: 0.78,
			Source: map[string]string{
				"surface": artifact.Metadata["screenshot_surface"],
			},
			Evidence: []core.EvidenceExcerpt{
				{
					ArtifactID:  artifact.ID,
					Snippet:     truncate(text, 250),
					Location:    artifact.Path,
					Confidence:  0.78,
					Description: "Structured indicators inferred from screenshot OCR",
				},
			},
		})
	}

	return artifact, observations, nil, nil
}

func readImageMetadata(path string) map[string]string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil
	}

	return map[string]string{
		"image_width":  strconv.Itoa(cfg.Width),
		"image_height": strconv.Itoa(cfg.Height),
	}
}

func analyzeOCRText(text string) map[string]string {
	metadata := map[string]string{
		"ocr_status":     "succeeded",
		"ocr_line_count": strconv.Itoa(countNonEmptyLines(text)),
		"ocr_word_count": strconv.Itoa(len(strings.Fields(text))),
		"ocr_char_count": strconv.Itoa(len([]rune(text))),
		"ocr_preview":    truncate(strings.Join(strings.Fields(text), " "), 120),
	}

	ips := uniqueMatches(ipPattern.FindAllString(text, -1))
	ports := uniqueLowerMatches(portPattern.FindAllString(text, -1))
	services := uniqueLowerMatches(findSubmatchValues(servicePattern, text))
	commands := uniqueLowerMatches(findSubmatchValues(commandPattern, text))

	if len(ips) > 0 {
		metadata["detected_ips"] = strings.Join(ips, ",")
	}
	if len(ports) > 0 {
		metadata["detected_ports"] = strings.Join(ports, ",")
	}
	if len(services) > 0 {
		metadata["detected_services"] = strings.Join(services, ",")
	}
	if len(commands) > 0 {
		metadata["detected_commands"] = strings.Join(commands, ",")
	}
	metadata["screenshot_surface"] = detectSurface(text, services, commands, ports)

	return metadata
}

func detectSurface(text string, services, commands, ports []string) string {
	lower := strings.ToLower(text)
	switch {
	case len(commands) > 0:
		return "terminal"
	case len(ports) > 0:
		return "terminal"
	case strings.Contains(lower, "$ ") || strings.Contains(lower, "# ") || strings.Contains(lower, "c:\\>") || strings.Contains(lower, "ps ") || strings.Contains(lower, "powershell"):
		return "terminal"
	case strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.") || strings.Contains(lower, "dashboard") || strings.Contains(lower, "sign in"):
		return "browser"
	case len(services) > 0 && (strings.Contains(lower, "scan") || strings.Contains(lower, "open")):
		return "terminal"
	default:
		return "generic"
	}
}

func buildIndicatorSummary(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}

	var parts []string
	if value := metadata["detected_ips"]; value != "" {
		parts = append(parts, "IPs: "+value)
	}
	if value := metadata["detected_ports"]; value != "" {
		parts = append(parts, "Ports: "+value)
	}
	if value := metadata["detected_services"]; value != "" {
		parts = append(parts, "Services: "+value)
	}
	if value := metadata["detected_commands"]; value != "" {
		parts = append(parts, "Commands: "+value)
	}
	if value := metadata["screenshot_surface"]; value != "" && value != "generic" {
		parts = append(parts, "Surface: "+value)
	}
	return strings.Join(parts, " | ")
}

func mergeMetadata(base map[string]string, additions map[string]string) map[string]string {
	if len(base) == 0 && len(additions) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(additions))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range additions {
		if value == "" {
			continue
		}
		merged[key] = value
	}
	return merged
}

func findSubmatchValues(pattern *regexp.Regexp, text string) []string {
	matches := pattern.FindAllStringSubmatch(text, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			values = append(values, match[1])
		} else if len(match) == 1 {
			values = append(values, match[0])
		}
	}
	return values
}

func uniqueMatches(values []string) []string {
	seen := map[string]struct{}{}
	var unique []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func uniqueLowerMatches(values []string) []string {
	seen := map[string]struct{}{}
	var unique []string
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		unique = append(unique, normalized)
	}
	return unique
}

func countNonEmptyLines(text string) int {
	lines := strings.Split(text, "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func truncate(in string, max int) string {
	if len(in) <= max {
		return in
	}
	return strings.TrimSpace(in[:max]) + "..."
}
