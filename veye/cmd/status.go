package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cluster health snapshot",
	RunE:  runStatus,
}

func runStatus(_ *cobra.Command, _ []string) error {
	health, err := api.Health()
	if err != nil {
		return err
	}
	snap, _ := api.Snapshot()
	k8s, _ := api.K8s()
	alerts, _ := api.Alerts("firing")

	fmt.Println()

	// Header
	statusStyle := styles.Good
	if health.Status != "healthy" {
		statusStyle = styles.Bad
	}
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		styles.Title.Render("VisualEyes"),
		styles.Mute.Render("  ·  backend "),
		statusStyle.Render(health.Status),
		styles.Mute.Render("  ·  uptime "),
		styles.ValStyle.Render(health.Uptime),
	)
	fmt.Println(styles.Box.Render(header))
	fmt.Println()

	// System metrics
	fmt.Println(styles.SectionHeader.Render("  SYSTEM METRICS"))
	if snap != nil {
		printMetricRow("CPU usage",    snap.Metrics["cpu"]["usage"], "%")
		printMetricRow("Memory used",  snap.Metrics["memory"]["usage_percent"], "%")
		printMetricRow("Disk used",    snap.Metrics["disk"]["usage_percent"], "%")
		printMetricRow("Load (1m)",    snap.Metrics["load"]["1min"], "")
		printMetricRow("Net recv/s",   snap.Metrics["network"]["bytes_recv_per_sec"], " B/s")
	} else {
		fmt.Println(styles.Mute.Render("  no system metrics yet"))
	}

	// Kubernetes
	fmt.Println()
	fmt.Println(styles.SectionHeader.Render("  KUBERNETES"))
	if k8s != nil {
		m := k8s.Metrics
		krows := []struct{ k, v string }{
			{"Nodes ready",  fmt.Sprintf("%d / %d", m.Nodes.Ready, m.Nodes.Total)},
			{"Pods running", fmt.Sprintf("%d / %d", m.Pods.Running, m.Pods.Total)},
			{"Node CPU",     fmt.Sprintf("%.3f cores", m.Resources.CPU.Usage)},
			{"Node Memory",  fmt.Sprintf("%.1f GB", m.Resources.Memory.Usage/1e9)},
		}
		for _, r := range krows {
			fmt.Printf("  %s  %s\n",
				styles.KeyStyle.Width(18).Render(r.k),
				styles.ValStyle.Render(r.v),
			)
		}
	} else {
		fmt.Println(styles.Mute.Render("  no kubernetes metrics yet"))
	}

	// Alerts
	fmt.Println()
	fmt.Println(styles.SectionHeader.Render("  ALERTS"))
	if len(alerts) == 0 {
		fmt.Println(styles.Good.Render("  ✓ No firing alerts   system is healthy"))
	} else {
		for _, a := range alerts {
			fmt.Printf("  %s  %s\n", styles.SeverityBadge(a.Severity), styles.ValStyle.Render(a.Message))
		}
		fmt.Println()
		fmt.Println(styles.Mute.Render(fmt.Sprintf("  %d alert(s) firing  ·  run 'veye alerts' for details", len(alerts))))
	}

	fmt.Println()
	fmt.Println(styles.HelpBar.Render("  veye alerts · veye logs · veye rca <id> · veye watch"))
	fmt.Println()
	return nil
}

func printMetricRow(label string, mv client.MetricValue, unit string) {
	if mv.Timestamp == "" {
		return
	}
	val := fmt.Sprintf("%.2f%s", mv.Value, unit)
	fmt.Printf("  %s  %s\n",
		styles.KeyStyle.Width(18).Render(label),
		styles.ValStyle.Render(val),
	)
}
