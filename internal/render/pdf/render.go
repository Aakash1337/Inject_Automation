package pdf

import (
	"strings"

	"injectctl/internal/core"

	"github.com/jung-kurt/gofpdf"
)

func WriteAssessment(path string, result *core.AssessmentResult) error {
	pdf := newDoc()
	pdf.AddPage()
	writeHeader(pdf, result.Config.Title, result.Config.Client, result.Config.Environment, result.Config.Classification)
	writeSection(pdf, "Status", result.Status)
	if result.ErrorReport != nil {
		writeSection(pdf, "Generation Error", strings.Join([]string{
			"Stage: " + result.ErrorReport.Stage,
			"Message: " + result.ErrorReport.Message,
			"Recommendations: " + strings.Join(result.ErrorReport.Recommendations, "; "),
		}, "\n"))
	}
	writeSection(pdf, "Executive Summary", result.Draft.ExecutiveSummary)
	if len(result.Draft.Findings) == 0 {
		writeSection(pdf, "Findings", "No findings were generated. Review the observations section.")
	} else {
		for _, finding := range result.Draft.Findings {
			writeSection(pdf, finding.Title+" ("+finding.Severity+")", strings.Join([]string{
				finding.Description,
				"Impact: " + finding.Impact,
				"Remediation: " + finding.Remediation,
				"Evidence: " + strings.Join(finding.EvidenceRefs, ", "),
			}, "\n"))
		}
	}
	writeObservations(pdf, result.Observations)
	return pdf.OutputFileAndClose(path)
}

func WriteInject(path string, result *core.InjectResult) error {
	pdf := newDoc()
	pdf.AddPage()
	writeHeader(pdf, result.Config.Title, result.Config.Client, result.Config.Environment, result.Config.Classification)
	writeSection(pdf, "Status", result.Status)
	if result.ErrorReport != nil {
		writeSection(pdf, "Generation Error", strings.Join([]string{
			"Stage: " + result.ErrorReport.Stage,
			"Message: " + result.ErrorReport.Message,
			"Recommendations: " + strings.Join(result.ErrorReport.Recommendations, "; "),
		}, "\n"))
	}
	writeSection(pdf, "Scenario Summary", result.Draft.ScenarioSummary)
	if len(result.Draft.Items) == 0 {
		writeSection(pdf, "Injects", "No injects were generated. Review the observations section.")
	} else {
		for _, item := range result.Draft.Items {
			writeSection(pdf, item.Title, strings.Join([]string{
				"Audience: " + item.Audience,
				"Channel: " + item.Channel,
				"Trigger: " + item.Trigger,
				"Expected Action: " + item.ExpectedAction,
				item.Body,
				"Evidence: " + strings.Join(item.EvidenceRefs, ", "),
			}, "\n"))
		}
	}
	writeObservations(pdf, result.Observations)
	return pdf.OutputFileAndClose(path)
}

func newDoc() *gofpdf.Fpdf {
	pdf := gofpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	return pdf
}

func writeHeader(pdf *gofpdf.Fpdf, title, client, environment, classification string) {
	pdf.SetFont("Arial", "B", 18)
	pdf.CellFormat(0, 12, title, "", 1, "", false, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.MultiCell(0, 6, strings.Join([]string{
		"Client: " + client,
		"Environment: " + environment,
		"Classification: " + classification,
	}, "\n"), "", "", false)
	pdf.Ln(2)
}

func writeSection(pdf *gofpdf.Fpdf, title, body string) {
	pdf.SetFont("Arial", "B", 14)
	pdf.MultiCell(0, 8, title, "", "", false)
	if strings.TrimSpace(body) == "" {
		pdf.Ln(1)
		return
	}
	pdf.SetFont("Arial", "", 11)
	pdf.MultiCell(0, 6, body, "", "", false)
	pdf.Ln(2)
}

func writeObservations(pdf *gofpdf.Fpdf, observations []core.Observation) {
	writeSection(pdf, "Observations", "")
	for _, observation := range observations {
		pdf.SetFont("Arial", "B", 11)
		pdf.MultiCell(0, 6, observation.ID+" | "+observation.Title, "", "", false)
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 5, observation.Detail, "", "", false)
		pdf.Ln(1)
	}
}
