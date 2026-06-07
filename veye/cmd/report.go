package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/spf13/cobra"
)

var reportIncidentID uint

var reportCmd = &cobra.Command{
	Use:   "report [incident-id]",
	Short: "Print a structured incident report (stdout or --incident-id)",
	Long:  "Generates a printable incident report. Without --incident-id, shows MTTR summary and top-5 open/investigating incidents.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if reportIncidentID > 0 {
			return printSingleReport(reportIncidentID)
		}
		return printSummaryReport()
	},
}

func init() {
	reportCmd.Flags().UintVar(&reportIncidentID, "incident-id", 0, "Generate report for a specific incident")
}

func printSingleReport(id uint) error {
	inc, err := api.GetIncident(id)
	if err != nil {
		return fmt.Errorf("fetch incident %d: %w", id, err)
	}

	sep := strings.Repeat("=", 72)
	thin := strings.Repeat("-", 72)

	fmt.Println(sep)
	fmt.Printf("  VISUALEYES INCIDENT REPORT\n")
	fmt.Printf("  Generated: %s\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	fmt.Println(sep)
	fmt.Println()

	fmt.Printf("  Incident Code   : %s\n", inc.IncidentCode)
	fmt.Printf("  Title           : %s\n", orNone(inc.Title))
	fmt.Printf("  Severity        : %s\n", inc.Severity)
	fmt.Printf("  Status          : %s\n", inc.Status)
	fmt.Printf("  Category        : %s\n", orNone(inc.Category))
	fmt.Printf("  Confidence      : %d%%\n", inc.ConfidenceScore)
	fmt.Println(thin)

	fmt.Printf("  Detected At     : %s\n", inc.DetectedAt)
	if inc.MitigatedAt != "" {
		fmt.Printf("  Mitigated At    : %s\n", inc.MitigatedAt)
	}
	if inc.ResolvedAt != "" {
		fmt.Printf("  Resolved At     : %s\n", inc.ResolvedAt)
	}
	if inc.MTTRSeconds != nil {
		fmt.Printf("  MTTR            : %s\n", formatMTTR(float64(*inc.MTTRSeconds)))
	}
	if inc.AlertID > 0 {
		fmt.Printf("  Source Alert ID : %d\n", inc.AlertID)
	}
	fmt.Println(thin)

	fmt.Println("  ROOT CAUSE")
	fmt.Printf("  %s\n", wrapReport(orNone(inc.RootCause), 68))
	fmt.Println()

	if inc.ContributingFactors != "" && inc.ContributingFactors != "null" {
		var factors []string
		if err := json.Unmarshal([]byte(inc.ContributingFactors), &factors); err == nil && len(factors) > 0 {
			fmt.Println("  CONTRIBUTING FACTORS")
			for i, f := range factors {
				fmt.Printf("  %d. %s\n", i+1, f)
			}
			fmt.Println()
		}
	}

	if inc.AffectedServices != "" && inc.AffectedServices != "null" {
		var svcs []string
		if err := json.Unmarshal([]byte(inc.AffectedServices), &svcs); err == nil && len(svcs) > 0 {
			fmt.Println("  AFFECTED SERVICES")
			fmt.Printf("  %s\n\n", strings.Join(svcs, ", "))
		}
	}

	fmt.Println(sep)
	return nil
}

func printSummaryReport() error {
	resp, err := api.FullIncidents(20, "", "", 0)
	if err != nil {
		return fmt.Errorf("fetch incidents: %w", err)
	}

	sep := strings.Repeat("=", 72)
	thin := strings.Repeat("-", 72)

	fmt.Println(sep)
	fmt.Printf("  VISUALEYES INCIDENT SUMMARY REPORT\n")
	fmt.Printf("  Generated: %s\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	fmt.Println(sep)
	fmt.Println()

	if resp.MTTRCount > 0 {
		fmt.Printf("  MTTR Statistics\n")
		fmt.Printf("  Average MTTR     : %s\n", formatMTTR(resp.MTTRAvgSeconds))
		fmt.Printf("  Incidents tracked: %d\n", resp.MTTRCount)
		fmt.Println(thin)
		fmt.Println()
	}

	if len(resp.Incidents) == 0 {
		fmt.Println("  No incidents recorded.")
		fmt.Println(sep)
		return nil
	}

	// Severity breakdown
	counts := map[string]int{}
	for _, inc := range resp.Incidents {
		counts[inc.Severity]++
	}
	fmt.Println("  SEVERITY BREAKDOWN")
	for _, sev := range []string{"SEV1", "SEV2", "SEV3", "SEV4"} {
		if n := counts[sev]; n > 0 {
			fmt.Printf("  %-6s %d\n", sev, n)
		}
	}
	fmt.Println()
	fmt.Println(thin)

	fmt.Println("  INCIDENT LIST")
	fmt.Println()
	printReportTable(resp.Incidents)

	fmt.Println(sep)
	return nil
}

func printReportTable(incidents []client.Incident) {
	hdr := fmt.Sprintf("  %-14s %-5s %-14s %-12s %s",
		"Code", "SEV", "Status", "MTTR", "Title")
	fmt.Println(hdr)
	fmt.Println("  " + strings.Repeat("-", 68))
	for _, inc := range incidents {
		mttr := "-"
		if inc.MTTRSeconds != nil {
			mttr = formatMTTR(float64(*inc.MTTRSeconds))
		}
		title := inc.Title
		if title == "" {
			title = inc.Category
		}
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		fmt.Printf("  %-14s %-5s %-14s %-12s %s\n",
			inc.IncidentCode, inc.Severity, inc.Status, mttr, title)
	}
	fmt.Println()
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func wrapReport(s string, width int) string {
	words := strings.Fields(s)
	var lines []string
	line := ""
	for _, w := range words {
		if len(line)+len(w)+1 > width {
			lines = append(lines, line)
			line = w
		} else {
			if line != "" {
				line += " "
			}
			line += w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n  ")
}
