package nessus

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"injectctl/internal/core"
)

type xmlClientData struct {
	Hosts []xmlReportHost `xml:"Report>ReportHost"`
}

type xmlReportHost struct {
	Name  string          `xml:"name,attr"`
	Items []xmlReportItem `xml:"ReportItem"`
}

type xmlReportItem struct {
	Port         int    `xml:"port,attr"`
	Severity     int    `xml:"severity,attr"`
	PluginName   string `xml:"pluginName,attr"`
	PluginOutput string `xml:"plugin_output"`
	Solution     string `xml:"solution"`
	Synopsis     string `xml:"synopsis"`
	Protocol     string `xml:"protocol,attr"`
	PluginFamily string `xml:"pluginFamily,attr"`
}

func Parse(artifact core.Artifact) (core.Artifact, []core.Observation, error) {
	data, err := os.ReadFile(artifact.Path)
	if err != nil {
		return artifact, nil, err
	}
	artifact.Parser = "nessus"
	artifact.ExtractedText = truncate(string(data), 16000)

	if strings.HasSuffix(strings.ToLower(artifact.Path), ".csv") {
		return parseCSV(artifact, data)
	}
	return parseXML(artifact, data)
}

func parseCSV(artifact core.Artifact, data []byte) (core.Artifact, []core.Observation, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return artifact, nil, err
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}

	var observations []core.Observation
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return artifact, nil, err
		}

		host := field(record, index, "host")
		port := field(record, index, "port")
		name := field(record, index, "name")
		synopsis := field(record, index, "synopsis")
		solution := field(record, index, "solution")
		risk := strings.ToLower(field(record, index, "risk"))
		if name == "" {
			continue
		}

		detail := strings.TrimSpace(fmt.Sprintf("%s:%s %s %s", host, port, name, synopsis))
		observations = append(observations, core.Observation{
			Title:      "Vulnerability detected",
			Detail:     truncate(detail, 700),
			Category:   "vulnerability",
			Severity:   normalizeSeverity(risk),
			Confidence: 0.96,
			Source: map[string]string{
				"tool": "nessus",
				"host": host,
			},
			Evidence: []core.EvidenceExcerpt{
				{
					ArtifactID:  artifact.ID,
					Snippet:     truncate(strings.TrimSpace(strings.Join([]string{name, synopsis, solution}, " | ")), 350),
					Location:    artifact.Path,
					Confidence:  0.96,
					Description: "Nessus CSV row",
				},
			},
		})
	}
	return artifact, observations, nil
}

func parseXML(artifact core.Artifact, data []byte) (core.Artifact, []core.Observation, error) {
	var clientData xmlClientData
	if err := xml.Unmarshal(data, &clientData); err != nil {
		return artifact, nil, err
	}

	var observations []core.Observation
	for _, host := range clientData.Hosts {
		for _, item := range host.Items {
			title := strings.TrimSpace(item.PluginName)
			if title == "" {
				continue
			}
			detail := strings.TrimSpace(fmt.Sprintf("%s:%d %s %s", host.Name, item.Port, title, item.Synopsis))
			observations = append(observations, core.Observation{
				Title:      "Vulnerability detected",
				Detail:     truncate(detail, 700),
				Category:   "vulnerability",
				Severity:   normalizeSeverity(strconv.Itoa(item.Severity)),
				Confidence: 0.97,
				Source: map[string]string{
					"tool":   "nessus",
					"host":   host.Name,
					"family": item.PluginFamily,
				},
				Evidence: []core.EvidenceExcerpt{
					{
						ArtifactID:  artifact.ID,
						Snippet:     truncate(strings.TrimSpace(strings.Join([]string{title, item.Synopsis, item.Solution, item.PluginOutput}, " | ")), 350),
						Location:    artifact.Path,
						Confidence:  0.97,
						Description: "Nessus XML report item",
					},
				},
			})
		}
	}
	return artifact, observations, nil
}

func field(record []string, index map[string]int, name string) string {
	position, ok := index[name]
	if !ok || position >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[position])
}

func normalizeSeverity(in string) string {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "4", "critical":
		return "critical"
	case "3", "high":
		return "high"
	case "2", "medium":
		return "medium"
	case "1", "low":
		return "low"
	default:
		return "info"
	}
}

func truncate(in string, max int) string {
	if len(in) <= max {
		return in
	}
	return strings.TrimSpace(in[:max]) + "..."
}
