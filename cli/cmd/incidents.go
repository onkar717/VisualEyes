package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/cli/internal/client"
	"github.com/onkar717/visual-eyes/cli/internal/styles"
	"github.com/spf13/cobra"
)

var (
	incidentLimit   int
	incidentAlertID uint
)

var incidentsCmd = &cobra.Command{
	Use:   "incidents",
	Short: "Show notification delivery history for alert events",
	Long:  "Displays a table of recent alert fired/resolved notifications, including delivery channel, success/failure, and timestamps.",
	RunE: func(cmd *cobra.Command, args []string) error {
		events, err := api.Incidents(incidentLimit, incidentAlertID)
		if err != nil {
			return fmt.Errorf("fetch incidents: %w", err)
		}
		printIncidents(events)
		return nil
	},
}

func init() {
	incidentsCmd.Flags().IntVarP(&incidentLimit, "limit", "n", 50, "Maximum number of events to show")
	incidentsCmd.Flags().UintVar(&incidentAlertID, "alert-id", 0, "Filter events for a specific alert ID")
}

func printIncidents(events []client.NotificationEvent) {
	fmt.Println()
	fmt.Println(styles.Title.Render("  Notification Incident History"))
	fmt.Println()

	if len(events) == 0 {
		fmt.Println(styles.Mute.Render("  No notification events recorded yet."))
		fmt.Println(styles.Mute.Render("  Events are recorded when alerts fire or resolve."))
		fmt.Println()
		return
	}

	// ── Column widths ─────────────────────────────────────────────────────────
	colID      := lipgloss.NewStyle().Width(6).Foreground(styles.Gray)
	colTime    := lipgloss.NewStyle().Width(22).Foreground(styles.Gray)
	colType    := lipgloss.NewStyle().Width(10)
	colChannel := lipgloss.NewStyle().Width(8).Foreground(styles.Gray)
	colStatus  := lipgloss.NewStyle().Width(8)
	colRule    := lipgloss.NewStyle().Width(28).Foreground(styles.White)

	// Header
	header := fmt.Sprintf("  %s %s %s %s %s %s",
		colID.Bold(true).Render("ID"),
		colTime.Bold(true).Foreground(styles.White).Render("Time"),
		colType.Bold(true).Foreground(styles.White).Render("Event"),
		colChannel.Bold(true).Foreground(styles.White).Render("Channel"),
		colStatus.Bold(true).Foreground(styles.White).Render("Status"),
		colRule.Bold(true).Render("Rule"),
	)
	fmt.Println(header)
	fmt.Println("  " + strings.Repeat("─", 86))

	for _, e := range events {
		// Format event type with color
		eventStyle := styles.Bad
		if e.EventType == "resolved" {
			eventStyle = styles.Good
		}

		// Format status
		statusStr := "✓ ok"
		statusStyle := styles.Good
		if !e.Success {
			statusStr = "✗ fail"
			statusStyle = styles.Bad
		}

		// Trim timestamp to readable form
		ts := e.CreatedAt
		if len(ts) > 19 {
			ts = ts[:19]
		}
		ts = strings.ReplaceAll(ts, "T", " ")

		// Truncate rule name
		rule := e.RuleName
		if len(rule) > 26 {
			rule = rule[:23] + "..."
		}

		row := fmt.Sprintf("  %s %s %s %s %s %s",
			colID.Render(fmt.Sprintf("%d", e.ID)),
			colTime.Render(ts),
			colType.Render(eventStyle.Render(e.EventType)),
			colChannel.Render(e.Channel),
			colStatus.Render(statusStyle.Render(statusStr)),
			colRule.Render(rule),
		)
		fmt.Println(row)

		// Show error detail on next line if delivery failed
		if !e.Success && e.ErrMsg != "" {
			errLine := fmt.Sprintf("  %s %s",
				styles.Mute.Render("       └─ error:"),
				styles.Bad.Render(e.ErrMsg),
			)
			fmt.Println(errLine)
		}
	}

	fmt.Println()
	fmt.Printf("  %s %s\n\n",
		styles.Mute.Render("Showing"),
		styles.Mute.Render(fmt.Sprintf("%d events", len(events))),
	)
}
