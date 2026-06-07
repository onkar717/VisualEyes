package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/onkar717/visual-eyes/cli/internal/client"
	"github.com/onkar717/visual-eyes/cli/internal/styles"
	"github.com/spf13/cobra"
)

var rcaCmd = &cobra.Command{
	Use:   "rca <alert-id>",
	Short: "Show Claude's AI root cause analysis for an alert",
	Args:  cobra.ExactArgs(1),
	RunE:  runRCA,
}

func runRCA(_ *cobra.Command, args []string) error {
	id64, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid alert ID: %q", args[0])
	}
	id := uint(id64)

	alert, err := api.AlertByID(id)
	if err != nil {
		return fmt.Errorf("alert %d not found: %w", id, err)
	}

	fmt.Println()

	// Alert summary
	t, _ := time.Parse(time.RFC3339Nano, alert.FiredAt)
	fired := humanDuration(time.Since(t))

	fmt.Println(styles.Box.Render(
		styles.SeverityBadge(alert.Severity) + "  " +
			styles.Title.Render(alert.RuleName) + "\n" +
			styles.Mute.Render("  "+alert.Message) + "\n" +
			styles.Mute.Render(fmt.Sprintf("  fired %s ago · resource: %s · namespace: %s",
				fired, alert.ResourceID, alert.Namespace)),
	))
	fmt.Println()

	// RCA status
	switch alert.RCAStatus {
	case "", "pending":
		fmt.Println(styles.Mute.Render("  RCA not yet triggered or pending."))
		fmt.Println(styles.Mute.Render("  Set ANTHROPIC_API_KEY and rca.enabled=true to enable Claude analysis."))
		fmt.Println()
		return nil
	case "running":
		fmt.Println(styles.SevWarning.Render("  ⟳ Claude is analysing this alert…"))
		fmt.Println()
		return nil
	case "failed":
		fmt.Println(styles.Bad.Render("  ✗ RCA analysis failed."))
		fmt.Println()
		return nil
	}

	// Fetch RCA result
	rca, err := api.RCA(id)
	if err != nil {
		return fmt.Errorf("could not fetch RCA for alert %d: %w", id, err)
	}

	// Root cause
	fmt.Println(styles.SectionHeader.Render("  ROOT CAUSE"))
	fmt.Println()
	for _, line := range wrap(rca.RootCause, 80) {
		fmt.Println("  " + styles.ValStyle.Render(line))
	}
	fmt.Println()

	// Explanation
	fmt.Println(styles.SectionHeader.Render("  ANALYSIS"))
	fmt.Println()
	for _, line := range wrap(rca.Explanation, 80) {
		fmt.Println("  " + line)
	}
	fmt.Println()

	// Remediation commands
	var cmds []client.FixCommand
	if rca.Commands != "" {
		_ = json.Unmarshal([]byte(rca.Commands), &cmds)
	}

	if len(cmds) > 0 {
		fmt.Println(styles.SectionHeader.Render("  REMEDIATION COMMANDS"))
		fmt.Println()
		for i, cmd := range cmds {
			autoLabel := ""
			if cmd.IsAutoSafe {
				autoLabel = styles.Good.Render(" [auto-safe]")
			} else {
				autoLabel = styles.SevWarning.Render(" [manual]")
			}
			statusLabel := cmdStatusLabel(cmd.Status)

			fmt.Printf("  %s  %s%s  %s\n",
				styles.Mute.Render(fmt.Sprintf("[%d]", i+1)),
				styles.ValStyle.Render(cmd.Command),
				autoLabel,
				statusLabel,
			)
			if cmd.Output != "" {
				for _, l := range strings.Split(cmd.Output, "\n") {
					fmt.Println("      " + styles.Good.Render(l))
				}
			}
			if cmd.Error != "" {
				fmt.Println("      " + styles.Bad.Render("error: "+cmd.Error))
			}
		}
		fmt.Println()
	}

	// Footer
	updAt, _ := time.Parse(time.RFC3339Nano, rca.UpdatedAt)
	fmt.Println(styles.Mute.Render(fmt.Sprintf("  model: %s  ·  %d input tokens  ·  analysed %s ago",
		rca.Model, rca.InputTokens, humanDuration(time.Since(updAt)))))
	fmt.Println()
	return nil
}

func cmdStatusLabel(s string) string {
	switch s {
	case "executed":
		return styles.Good.Render("✓ executed")
	case "failed":
		return styles.Bad.Render("✗ failed")
	case "skipped":
		return styles.Mute.Render("skipped")
	default:
		return styles.Mute.Render("pending")
	}
}

// wrap breaks text into lines of at most width chars.
func wrap(text string, width int) []string {
	words := strings.Fields(text)
	var lines []string
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
		} else if len(line)+1+len(w) <= width {
			line += " " + w
		} else {
			lines = append(lines, line)
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
