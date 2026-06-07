package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/cli/internal/client"
	"github.com/onkar717/visual-eyes/cli/internal/styles"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Proactive cluster health check — surfaces issues before they page you",
	Long:  "Queries the VisualEyes backend for active alerts and metric thresholds, then prints a prioritised list of findings.",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := api.Scan()
		if err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}
		printScanResult(result)
		// Exit non-zero when critical issues found so CI/scripts can act on it.
		if result.Overall == "critical" {
			os.Exit(2)
		}
		return nil
	},
}

func printScanResult(r *client.ScanResult) {
	// Header
	overallStyle := styles.Good
	overallIcon := "✓"
	switch r.Overall {
	case "critical":
		overallStyle = styles.Bad
		overallIcon = "✗"
	case "degraded":
		overallStyle = styles.SevWarning
		overallIcon = "!"
	}

	fmt.Println()
	fmt.Println(styles.Title.Render("  VisualEyes Cluster Scan"))
	fmt.Printf("  %s  %s\n\n",
		overallStyle.Render(overallIcon),
		overallStyle.Bold(true).Render(strings.ToUpper(r.Overall)),
	)

	// Summary bar
	summaryBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#303060")).
		Padding(0, 2).
		Width(62)

	summary := fmt.Sprintf(
		"%s  CPU %s   MEM %s   DISK %s\n%s  Active alerts: %s   Critical: %s   Warning: %s",
		styles.KeyStyle.Render("Resources"),
		formatPercent(r.Summary.CPUPercent),
		formatPercent(r.Summary.MemoryPercent),
		formatPercent(r.Summary.DiskPercent),
		styles.KeyStyle.Render("Alerts   "),
		styles.ValStyle.Render(fmt.Sprintf("%d", r.Summary.ActiveAlerts)),
		styles.Bad.Render(fmt.Sprintf("%d", r.Summary.CriticalAlerts)),
		styles.SevWarning.Render(fmt.Sprintf("%d", r.Summary.WarningAlerts)),
	)
	fmt.Println(summaryBox.Render(summary))
	fmt.Println()

	// Issues
	if len(r.Issues) == 0 {
		fmt.Println(styles.Good.Render("  No issues found. Everything looks healthy."))
		fmt.Println()
		return
	}

	fmt.Println(styles.SectionHeader.Render(fmt.Sprintf("  Findings (%d)", r.IssueCount)))
	fmt.Println()

	// Sort: critical first (they're already ordered by active alerts, then metrics).
	for _, issue := range r.Issues {
		badge := styles.SeverityBadge(issue.Severity)
		cat := styles.KeyStyle.Render(fmt.Sprintf("%-8s", issue.Category))
		res := styles.Mute.Render(issue.Resource)

		line := fmt.Sprintf("  %s %s %s", badge, cat, issue.Message)
		if issue.Value != "" {
			line += styles.Mute.Render("  " + issue.Value)
		}
		fmt.Println(line)
		fmt.Printf("         %s\n\n", res)
	}

	fmt.Printf("  %s  %s\n\n",
		styles.Mute.Render("Scanned at"),
		styles.Mute.Render(r.Timestamp),
	)
}

func formatPercent(v float64) string {
	if v == 0 {
		return styles.Mute.Render("n/a")
	}
	s := fmt.Sprintf("%.1f%%", v)
	switch {
	case v >= 90:
		return styles.Bad.Render(s)
	case v >= 75:
		return styles.SevWarning.Render(s)
	default:
		return styles.Good.Render(s)
	}
}
