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
	PrintBanner()

	health, err := api.Health()
	if err != nil {
		return err
	}
	snap, _ := api.Snapshot()
	k8s, _ := api.K8s()
	alerts, _ := api.Alerts("firing")

	// Backend status bar
	statusStyle := styles.Good
	if health.Status != "healthy" {
		statusStyle = styles.Bad
	}
	backendLine := lipgloss.JoinHorizontal(lipgloss.Left,
		styles.KeyStyle.Render("backend  "),
		statusStyle.Render("● "+health.Status),
		styles.Mute.Render("   uptime "),
		styles.ValStyle.Render(health.Uptime),
		styles.Mute.Render("   alerts "),
		alertCountLabel(len(alerts)),
	)
	fmt.Println(styles.Box.Render(backendLine))
	fmt.Println()

	// System metrics
	fmt.Println(styles.SectionHeader.Render("  🖥️  SYSTEM METRICS"))
	fmt.Println()
	if snap != nil {
		printMetricRow("CPU usage",   snap.Metrics["cpu"]["usage"], "%", 90, 75)
		printMetricRow("Memory used", snap.Metrics["memory"]["usage_percent"], "%", 90, 75)
		printMetricRow("Disk used",   snap.Metrics["disk"]["usage_percent"], "%", 90, 75)
		printMetricRow("Load (1m)",   snap.Metrics["load"]["1min"], "", 8, 4)
		printMetricRow("Net recv/s",  snap.Metrics["network"]["bytes_recv_per_sec"], " B/s", 0, 0)
	} else {
		fmt.Println(styles.Mute.Render("  no system metrics yet"))
	}

	// Kubernetes
	fmt.Println()
	fmt.Println(styles.SectionHeader.Render("  ☸️  KUBERNETES"))
	fmt.Println()
	if k8s != nil {
		m := k8s.Metrics
		nodeStyle := styles.Good
		if m.Nodes.Ready < m.Nodes.Total {
			nodeStyle = styles.Bad
		}
		podStyle := styles.Good
		if m.Pods.Running < m.Pods.Total {
			podStyle = styles.SevWarning
		}
		type krow struct {
			k string
			v string
			s lipgloss.Style
		}
		rows := []krow{
			{"Nodes ready",  fmt.Sprintf("%d / %d", m.Nodes.Ready, m.Nodes.Total), nodeStyle},
			{"Pods running", fmt.Sprintf("%d / %d", m.Pods.Running, m.Pods.Total), podStyle},
			{"Node CPU",     fmt.Sprintf("%.3f cores", m.Resources.CPU.Usage), styles.ValStyle},
			{"Node Memory",  fmt.Sprintf("%.1f GB", m.Resources.Memory.Usage/1e9), styles.ValStyle},
		}
		for _, r := range rows {
			fmt.Printf("  %s  %s\n",
				styles.KeyStyle.Width(18).Render(r.k),
				r.s.Render(r.v),
			)
		}
	} else {
		fmt.Println(styles.Mute.Render("  no kubernetes metrics yet"))
	}

	// Alerts
	fmt.Println()
	fmt.Println(styles.SectionHeader.Render("  🚨  ALERTS"))
	fmt.Println()
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
	fmt.Println(styles.HelpBar.Render("  veye alerts · veye logs · veye rca <id> · veye watch · veye scan"))
	fmt.Println()
	return nil
}

func alertCountLabel(n int) string {
	if n == 0 {
		return styles.Good.Render("none")
	}
	return styles.Bad.Render(fmt.Sprintf("%d firing", n))
}

func printMetricRow(label string, mv client.MetricValue, unit string, critThreshold, warnThreshold float64) {
	if mv.Timestamp == "" {
		return
	}
	val := fmt.Sprintf("%.2f%s", mv.Value, unit)
	valStyle := styles.ValStyle
	if critThreshold > 0 && mv.Value >= critThreshold {
		valStyle = styles.Bad
	} else if warnThreshold > 0 && mv.Value >= warnThreshold {
		valStyle = styles.SevWarning
	}
	fmt.Printf("  %s  %s\n",
		styles.KeyStyle.Width(18).Render(label),
		valStyle.Render(val),
	)
}
