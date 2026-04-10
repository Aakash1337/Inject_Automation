package nmap

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strings"

	"injectctl/internal/core"
)

var textPortPattern = regexp.MustCompile(`(?m)^(\d+)/(tcp|udp)\s+open\s+(\S+)(?:\s+(.*))?$`)

type xmlRun struct {
	Hosts []xmlHost `xml:"host"`
}

type xmlHost struct {
	Addresses []xmlAddress `xml:"address"`
	Ports     []xmlPorts   `xml:"ports"`
}

type xmlAddress struct {
	Addr string `xml:"addr,attr"`
}

type xmlPorts struct {
	Ports []xmlPort `xml:"port"`
}

type xmlPort struct {
	Protocol string         `xml:"protocol,attr"`
	PortID   string         `xml:"portid,attr"`
	State    xmlPortState   `xml:"state"`
	Service  xmlPortService `xml:"service"`
}

type xmlPortState struct {
	State string `xml:"state,attr"`
}

type xmlPortService struct {
	Name      string `xml:"name,attr"`
	Product   string `xml:"product,attr"`
	Version   string `xml:"version,attr"`
	ExtraInfo string `xml:"extrainfo,attr"`
}

func Parse(artifact core.Artifact) (core.Artifact, []core.Observation, error) {
	data, err := os.ReadFile(artifact.Path)
	if err != nil {
		return artifact, nil, err
	}

	artifact.Parser = "nmap"
	artifact.ExtractedText = truncate(string(data), 16000)

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "<") {
		return parseXML(artifact, data)
	}
	return parseText(artifact, trimmed)
}

func parseText(artifact core.Artifact, content string) (core.Artifact, []core.Observation, error) {
	matches := textPortPattern.FindAllStringSubmatch(content, -1)
	var observations []core.Observation
	for _, match := range matches {
		detail := fmt.Sprintf("%s/%s open %s %s", match[1], match[2], match[3], strings.TrimSpace(match[4]))
		observations = append(observations, core.Observation{
			Title:      "Open service detected",
			Detail:     strings.TrimSpace(detail),
			Category:   "network",
			Severity:   "medium",
			Confidence: 0.95,
			Source: map[string]string{
				"tool": "nmap",
			},
			Evidence: []core.EvidenceExcerpt{
				{
					ArtifactID:  artifact.ID,
					Snippet:     strings.TrimSpace(match[0]),
					Location:    artifact.Path,
					Confidence:  0.95,
					Description: "Nmap open port line",
				},
			},
		})
	}
	return artifact, observations, nil
}

func parseXML(artifact core.Artifact, data []byte) (core.Artifact, []core.Observation, error) {
	var run xmlRun
	if err := xml.Unmarshal(data, &run); err != nil {
		return artifact, nil, err
	}

	var observations []core.Observation
	for _, host := range run.Hosts {
		address := "unknown"
		if len(host.Addresses) > 0 {
			address = host.Addresses[0].Addr
		}
		for _, ports := range host.Ports {
			for _, port := range ports.Ports {
				if port.State.State != "open" {
					continue
				}
				service := strings.TrimSpace(strings.Join([]string{port.Service.Name, port.Service.Product, port.Service.Version, port.Service.ExtraInfo}, " "))
				detail := fmt.Sprintf("%s %s/%s %s", address, port.PortID, port.Protocol, strings.TrimSpace(service))
				observations = append(observations, core.Observation{
					Title:      "Open service detected",
					Detail:     strings.TrimSpace(detail),
					Category:   "network",
					Severity:   "medium",
					Confidence: 0.97,
					Source: map[string]string{
						"tool": "nmap",
						"host": address,
					},
					Evidence: []core.EvidenceExcerpt{
						{
							ArtifactID:  artifact.ID,
							Snippet:     strings.TrimSpace(detail),
							Location:    artifact.Path,
							Confidence:  0.97,
							Description: "Nmap XML open port record",
						},
					},
				})
			}
		}
	}

	return artifact, observations, nil
}

func truncate(in string, max int) string {
	if len(in) <= max {
		return in
	}
	return strings.TrimSpace(in[:max]) + "..."
}
