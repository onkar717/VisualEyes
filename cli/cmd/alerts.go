package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/cli/internal/styles"
	"github.com/spf13/cobra"
)

var (
	alertsStatus string
	alertsWatch  bool
)

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "List alerts with severity, resource, and RCA status",
	RunE:  runAlerts,
}

func init() {
	alertsCmd.Flags().StringVarP(&alertsStatus, "status", "s", "firing", "Filter: firing | all")
	alertsCmd.Flags().BoolVarP(&alertsWatch, "watch", "w", false, "Auto-refresh every 15s (Ctrl+C to quit)")
}

func runAlerts(_ *cobra.Command, _ []string) error {
	if alertsWatch {
		for {
			printAlertsTable()
			fmt.Println(styles.Mute.Render("  refreshing in 15s — Ctrl+C to quit"))
			time.Sleep(15 * time.Second)
		}
	}
	printAlertsTable()
	return nil
}

func printAlertsTable() {
	alerts, err := api.Alerts(alertsStatus)
	if err != nil {
		fmt.Println(styles.Bad.Render("  error: " + err.Error()))
		return
	}

	fmt.Println()
	header := fmt.Sprintf("  ALERTS  (%d %s)", len(alerts), alertsStatus)
	fmt.Println(styles.SectionHeader.Render(header))
	fmt.Println()

	if len(alerts) == 0 {
		fmt.Println(styles.Good.Render("  ✓ No alerts"))
		fmt.Println()
		return
	}

	// Column widths
	colSev  := 8
	colRule := 28
	colRes  := 18
	colVal  := 16
	colRCA  := 10

	// Header row
	sep := styles.Mute.Render(strings.Repeat("─", colSev+colRule+colRes+colVal+colRCA+8))
	hdr := lipgloss.JoinHorizontal(lipgloss.Left,
		styles.KeyStyle.Width(colSev).Render("SEV"),
		styles.KeyStyle.Width(colRule).Render("RULE"),
		styles.KeyStyle.Width(colRes).Render("RESOURCE"),
		styles.KeyStyle.Width(colVal).Render("VALUE / THRESH"),
		styles.KeyStyle.Width(colRCA).Render("RCA"),
	)
	fmt.Printf("  %s\n", hdr)
	fmt.Printf("  %s\n", sep)

	for _, a := range alerts {
		sev   := styles.SeverityBadge(a.Severity)
		rule  := styles.ValStyle.Width(colRule).Render(truncate(a.RuleName, colRule-1))
		res   := styles.Mute.Width(colRes).Render(truncate(a.ResourceID, colRes-1))
		val   := styles.ValStyle.Width(colVal).Render(fmt.Sprintf("%.2f / %.2f", a.Value, a.Threshold))
		rca   := rcaStatusLabel(a.RCAStatus)

		// Fired-at relative time
		t, terr := time.Parse(time.RFC3339Nano, a.FiredAt)
		fired := ""
		if terr == nil {
			fired = styles.Mute.Render("  fired " + humanDuration(time.Since(t)) + " ago")
		}

		row := lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Width(colSev).Render(sev),
			rule, res, val,
			lipgloss.NewStyle().Width(colRCA).Render(rca),
		)
		fmt.Printf("  %s%s\n", row, fired)
	}

	fmt.Println()
	fmt.Println(styles.HelpBar.Render("  veye rca <id> to see Claude's analysis  ·  veye watch for live dashboard"))
	fmt.Println()
}

func rcaStatusLabel(s string) string {
	switch s {
	case "done":
		return styles.Good.Render("done ✓")
	case "running":
		return styles.SevWarning.Render("running…")
	case "pending":
		return styles.Mute.Render("pending")
	case "failed":
		return styles.Bad.Render("failed ✗")
	default:
		return styles.Mute.Render("—")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
