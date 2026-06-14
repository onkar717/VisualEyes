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

var showCmd = &cobra.Command{
	Use:     "show <incident-id|INC-code>",
	Short:   "Show full detail for a single incident",
	Example: "  veye show 42\n  veye show INC-A3F2B1C4",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		arg := args[0]
		var (
			inc *client.Incident
			err error
		)
		if strings.HasPrefix(arg, "INC-") {
			inc, err = api.GetIncidentByCode(arg)
			if err != nil {
				return fmt.Errorf("fetch incident %q: %w", arg, err)
			}
		} else {
			var id uint
			if _, scanErr := fmt.Sscanf(arg, "%d", &id); scanErr != nil {
				return fmt.Errorf("invalid argument %q — use a numeric ID or INC-XXXX code", arg)
			}
			inc, err = api.GetIncident(id)
			if err != nil {
				return fmt.Errorf("fetch incident %d: %w", id, err)
			}
		}
		return renderIncidentDetail(inc)
	},
}

func renderIncidentDetail(inc *client.Incident) error {
	fmt.Println()

	sevSt := sevStyle(inc.Severity)
	statusSt := statusStyleInc(inc.Status)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#303060")).
		Padding(0, 2).Width(72)

	header := fmt.Sprintf("%s  %s  %s  %s",
		styles.ValStyle.Bold(true).Render(inc.IncidentCode),
		sevSt.Render(inc.Severity),
		statusSt.Render(inc.Status),
		styles.Mute.Render(inc.Category),
	)
	fmt.Println(box.Render(header))
	fmt.Println()

	title := inc.Title
	if title == "" {
		title = inc.Category
	}
	fmt.Println(styles.Title.Render("  " + title))
	fmt.Println()

	kv := func(k, v string) {
		fmt.Printf("  %s  %s\n", styles.KeyStyle.Width(20).Render(k), styles.ValStyle.Render(v))
	}
	kv("Severity", inc.Severity)
	kv("Status", inc.Status)
	kv("Category", inc.Category)
	kv("Confidence", fmt.Sprintf("%d%%", inc.ConfidenceScore))
	kv("Detected at", inc.DetectedAt)
	if inc.MitigatedAt != "" {
		kv("Mitigated at", inc.MitigatedAt)
	}
	if inc.ResolvedAt != "" {
		kv("Resolved at", inc.ResolvedAt)
	}
	if inc.MTTRSeconds != nil {
		kv("MTTR", formatMTTR(float64(*inc.MTTRSeconds)))
	}
	if inc.AlertID > 0 {
		kv("Alert ID", fmt.Sprintf("%d", inc.AlertID))
	}
	fmt.Println()

	if inc.RootCause != "" {
		fmt.Println(styles.SectionHeader.Render("  Root Cause"))
		fmt.Printf("  %s\n\n", wordWrap(inc.RootCause, 70))
	}

	if inc.ContributingFactors != "" && inc.ContributingFactors != "null" {
		var factors []string
		if err := json.Unmarshal([]byte(inc.ContributingFactors), &factors); err == nil && len(factors) > 0 {
			fmt.Println(styles.SectionHeader.Render("  Contributing Factors"))
			for _, f := range factors {
				fmt.Printf("  %s %s\n", styles.Mute.Render("•"), f)
			}
			fmt.Println()
		}
	}

	if inc.AffectedServices != "" && inc.AffectedServices != "null" {
		var svcs []string
		if err := json.Unmarshal([]byte(inc.AffectedServices), &svcs); err == nil && len(svcs) > 0 {
			fmt.Println(styles.SectionHeader.Render("  Affected Services"))
			fmt.Printf("  %s\n\n", strings.Join(svcs, ", "))
		}
	}

	fmt.Printf("  %s\n\n",
		styles.Mute.Render("veye incidents  to see all incidents  |  veye watch  for live dashboard"))
	return nil
}

func wordWrap(s string, width int) string {
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
