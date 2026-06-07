package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
	"github.com/spf13/cobra"
)

var clustersCmd = &cobra.Command{
	Use:   "clusters",
	Short: "List registered clusters and their health snapshots",
	RunE: func(cmd *cobra.Command, args []string) error {
		clusters, err := api.ListClusters()
		if err != nil {
			return fmt.Errorf("fetch clusters: %w", err)
		}

		fmt.Println()
		fmt.Println(styles.Title.Render("  Registered Clusters"))
		fmt.Println()

		if len(clusters) == 0 {
			fmt.Println(styles.Mute.Render("  No clusters registered yet."))
			fmt.Println(styles.Mute.Render("  k8s-agent sends a heartbeat to /api/clusters/heartbeat on startup."))
			fmt.Println()
			return nil
		}

		colName  := lipgloss.NewStyle().Width(20).Foreground(styles.Cyan)
		colScore := lipgloss.NewStyle().Width(8)
		colNodes := lipgloss.NewStyle().Width(12).Foreground(styles.Gray)
		colPods  := lipgloss.NewStyle().Width(30).Foreground(styles.Gray)
		colInc   := lipgloss.NewStyle().Width(10)
		colSeen  := lipgloss.NewStyle().Width(20).Foreground(styles.Gray)

		header := fmt.Sprintf("  %s %s %s %s %s %s",
			colName.Bold(true).Foreground(styles.White).Render("Cluster"),
			colScore.Bold(true).Foreground(styles.White).Render("Score"),
			colNodes.Bold(true).Foreground(styles.White).Render("Nodes"),
			colPods.Bold(true).Foreground(styles.White).Render("Pods"),
			colInc.Bold(true).Foreground(styles.White).Render("Open Inc"),
			colSeen.Bold(true).Foreground(styles.White).Render("Last Seen"),
		)
		fmt.Println(header)
		fmt.Println("  " + strings.Repeat("-", 104))

		for _, c := range clusters {
			scoreStyle := styles.Good
			if c.HealthScore < 70 {
				scoreStyle = styles.SevWarning
			}
			if c.HealthScore < 40 {
				scoreStyle = styles.Bad
			}

			incStyle := styles.Good
			if c.OpenIncidents > 0 {
				incStyle = styles.Bad
			}

			row := fmt.Sprintf("  %s %s %s %s %s %s",
				colName.Render(c.Name),
				colScore.Render(scoreStyle.Render(fmt.Sprintf("%.0f/100", c.HealthScore))),
				colNodes.Render(fmt.Sprintf("%d/%d Ready", c.ReadyNodes, c.TotalNodes)),
				colPods.Render(fmt.Sprintf("%d run / %d pend / %d fail / %d crash",
					c.RunningPods, c.PendingPods, c.FailedPods, c.CrashloopPods)),
				colInc.Render(incStyle.Render(fmt.Sprintf("%d", c.OpenIncidents))),
				colSeen.Render(c.LastSeen.Local().Format("2006-01-02 15:04:05")),
			)
			fmt.Println(row)
		}
		fmt.Println()
		return nil
	},
}
