package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
	"github.com/spf13/cobra"
)

var (
	incidentLimit    int
	incidentSeverity string
	incidentStatus   string
	incidentAlertID  uint
	incidentHours    int
)

var incidentsCmd = &cobra.Command{
	Use:   "incidents",
	Short: "Show incident history — SEV1-4 lifecycle, MTTR, root cause",
	Long:  "Displays structured incidents with severity classification, status lifecycle, MTTR, and AI-generated root cause. Use --severity/--status to filter.",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := api.FullIncidents(incidentLimit, incidentSeverity, incidentStatus, incidentHours)
		if err != nil {
			return fmt.Errorf("fetch incidents: %w", err)
		}
		printIncidentsFull(resp)
		return nil
	},
}

func init() {
	incidentsCmd.Flags().IntVarP(&incidentLimit, "limit", "n", 25, "Maximum incidents to show")
	incidentsCmd.Flags().StringVar(&incidentSeverity, "severity", "", "Filter by severity (SEV1, SEV2, SEV3, SEV4)")
	incidentsCmd.Flags().StringVar(&incidentStatus, "status", "", "Filter by status (OPEN, INVESTIGATING, MITIGATED, RESOLVED)")
	incidentsCmd.Flags().UintVar(&incidentAlertID, "alert-id", 0, "Show notification events for a specific alert ID")
	incidentsCmd.Flags().IntVar(&incidentHours, "hours", 0, "Only show incidents from the last N hours (0 = all time)")
}

func printIncidentsFull(resp *client.IncidentListResponse) {
	fmt.Println()
	fmt.Println(styles.Title.Render("  Incident History"))
	fmt.Println()

	if len(resp.Incidents) == 0 {
		fmt.Println(styles.Mute.Render("  No incidents recorded yet."))
		fmt.Println(styles.Mute.Render("  Incidents are created automatically when RCA completes for a fired alert."))
		fmt.Println()
		return
	}

	// MTTR summary bar
	if resp.MTTRCount > 0 {
		mttrBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#303060")).
			Padding(0, 2).Width(70)

		mttrLine := fmt.Sprintf("%s  %s   %s  %s",
			styles.KeyStyle.Render("Avg MTTR"),
			styles.ValStyle.Render(formatMTTR(resp.MTTRAvgSeconds)),
			styles.KeyStyle.Render("Incidents mitigated"),
			styles.ValStyle.Render(fmt.Sprintf("%d", resp.MTTRCount)),
		)
		if len(resp.MTTRBySeverity) > 0 {
			bySev := "  by severity:"
			for _, sev := range []string{"SEV1", "SEV2", "SEV3", "SEV4"} {
				if v, ok := resp.MTTRBySeverity[sev]; ok {
					bySev += fmt.Sprintf("  %s %s", styles.KeyStyle.Render(sev), styles.ValStyle.Render(formatMTTR(v)))
				}
			}
			mttrLine += "\n" + bySev
		}
		fmt.Println(mttrBox.Render(mttrLine))
		fmt.Println()
	}

	// Column styles
	colCode   := lipgloss.NewStyle().Width(14).Foreground(styles.Cyan)
	colSev    := lipgloss.NewStyle().Width(6)
	colStatus := lipgloss.NewStyle().Width(14)
	colCat    := lipgloss.NewStyle().Width(12).Foreground(styles.Gray)
	colConf   := lipgloss.NewStyle().Width(7).Foreground(styles.Gray)
	colTitle  := lipgloss.NewStyle().Width(32).Foreground(styles.White)

	header := fmt.Sprintf("  %s %s %s %s %s %s",
		colCode.Bold(true).Foreground(styles.White).Render("Code"),
		colSev.Bold(true).Foreground(styles.White).Render("SEV"),
		colStatus.Bold(true).Foreground(styles.White).Render("Status"),
		colCat.Bold(true).Foreground(styles.White).Render("Category"),
		colConf.Bold(true).Foreground(styles.White).Render("Conf%"),
		colTitle.Bold(true).Render("Title"),
	)
	fmt.Println(header)
	fmt.Println("  " + strings.Repeat("-", 88))

	for _, inc := range resp.Incidents {
		sevStyle := sevStyle(inc.Severity)
		statusStyle := statusStyleInc(inc.Status)

		title := inc.Title
		if title == "" {
			title = inc.Category
		}
		if len(title) > 30 {
			title = title[:27] + "..."
		}

		mttrStr := ""
		if inc.MTTRSeconds != nil {
			mttrStr = "  " + styles.Mute.Render("MTTR: "+formatMTTR(float64(*inc.MTTRSeconds)))
		}

		row := fmt.Sprintf("  %s %s %s %s %s %s%s",
			colCode.Render(inc.IncidentCode),
			colSev.Render(sevStyle.Render(inc.Severity)),
			colStatus.Render(statusStyle.Render(inc.Status)),
			colCat.Render(inc.Category),
			colConf.Render(fmt.Sprintf("%d%%", inc.ConfidenceScore)),
			colTitle.Render(title),
			mttrStr,
		)
		fmt.Println(row)

		// Root cause snippet if available.
		if inc.RootCause != "" {
			rc := inc.RootCause
			if len(rc) > 100 {
				rc = rc[:97] + "..."
			}
			fmt.Printf("  %s %s\n", styles.Mute.Render("  └-"), styles.Mute.Render(rc))
		}

		// Contributing factors.
		if inc.ContributingFactors != "" && inc.ContributingFactors != "null" {
			var factors []string
			if err := json.Unmarshal([]byte(inc.ContributingFactors), &factors); err == nil && len(factors) > 0 {
				factorStr := strings.Join(factors, " · ")
				if len(factorStr) > 100 {
					factorStr = factorStr[:97] + "..."
				}
				fmt.Printf("  %s %s\n", styles.Mute.Render("     factors:"), styles.Mute.Render(factorStr))
			}
		}

		fmt.Println()
	}

	fmt.Printf("  %s %s\n\n",
		styles.Mute.Render("Showing"),
		styles.Mute.Render(fmt.Sprintf("%d incidents", resp.Count)),
	)
}

func sevStyle(sev string) lipgloss.Style {
	switch sev {
	case "SEV1":
		return styles.Bad.Bold(true)
	case "SEV2":
		return styles.SevWarning.Bold(true)
	case "SEV3":
		return styles.SevInfo.Bold(true)
	default:
		return styles.Good.Bold(true)
	}
}

func statusStyleInc(s string) lipgloss.Style {
	switch s {
	case "OPEN":
		return styles.Bad
	case "INVESTIGATING":
		return styles.SevWarning
	case "MITIGATED":
		return styles.SevInfo
	case "RESOLVED":
		return styles.Good
	default:
		return styles.Mute
	}
}

func formatMTTR(secs float64) string {
	if secs < 60 {
		return fmt.Sprintf("%.0fs", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%.1fm", secs/60)
	}
	return fmt.Sprintf("%.1fh", secs/3600)
}
