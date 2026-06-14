package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
	"github.com/onkar717/visual-eyes/veye/internal/tui"
	"github.com/spf13/cobra"
)

var (
	watchApply    bool
	watchInterval int
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Interactive live dashboard   metrics, alerts, logs, RCA",
	Long: `Launches the interactive Bubbletea live dashboard.

With --apply: disables the TUI and instead runs a simple monitoring loop
that prompts to apply remediation whenever a SEV1/SEV2 alert has a completed
RCA result. Use --interval to control scan frequency (default 300s).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if watchApply {
			return runWatchLoop()
		}
		p := tea.NewProgram(tui.New(api), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	watchCmd.Flags().BoolVar(&watchApply, "apply", false, "Run remediation loop instead of TUI (prompt on SEV1/2 findings)")
	watchCmd.Flags().IntVar(&watchInterval, "interval", 300, "Scan interval in seconds (used with --apply)")
}

// runWatchLoop is a simple continuous scan+remediate loop (like reference clarctl watch --apply).
func runWatchLoop() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reader := bufio.NewReader(os.Stdin)
	scanCount := 0

	fmt.Printf("  %s  Continuous watch with auto-remediation (interval: %ds, Ctrl+C to stop)\n\n",
		styles.SevWarning.Render("→"), watchInterval)

	for {
		if ctx.Err() != nil {
			fmt.Println("\n  Watcher stopped.")
			return nil
		}

		scanCount++
		ts := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
		fmt.Printf("\n  %s  Scan #%d  ·  %s\n",
			styles.Mute.Render("──"),
			scanCount,
			styles.Mute.Render(ts),
		)

		// ── Cluster health snapshot ───────────────────────────────────────────
		clusters, _ := api.ListClusters()
		for _, c := range clusters {
			scoreStyle := styles.Good
			if c.HealthScore < 70 {
				scoreStyle = styles.SevWarning
			}
			if c.HealthScore < 40 {
				scoreStyle = styles.Bad
			}
			fmt.Printf("  %s  %s  nodes %d/%d  pods %d run / %d crash / %d fail\n",
				scoreStyle.Render(fmt.Sprintf("%.0f/100", c.HealthScore)),
				styles.ValStyle.Render(c.Name),
				c.ReadyNodes, c.TotalNodes,
				c.RunningPods, c.CrashloopPods, c.FailedPods,
			)
		}

		// ── Firing alerts ─────────────────────────────────────────────────────
		firingAlerts, err := api.Alerts("firing")
		if err != nil {
			fmt.Printf("  %s fetch alerts: %v\n", styles.Bad.Render("✗"), err)
		} else if len(firingAlerts) == 0 {
			fmt.Println("  " + styles.Good.Render("✓ No firing alerts."))
		} else {
			fmt.Printf("  %s  %d alert(s) firing\n", styles.Bad.Render("!"), len(firingAlerts))

			for _, a := range firingAlerts {
				sev := string(a.Severity)
				if sev != "critical" && sev != "warning" {
					continue
				}
				if a.RCAStatus != "done" {
					continue
				}

				rca, err := api.RCA(a.ID)
				if err != nil || rca.Status != "done" || rca.RootCause == "" {
					continue
				}

				var cmds []client.FixCommand
				_ = json.Unmarshal([]byte(rca.Commands), &cmds)
				if len(cmds) == 0 {
					continue
				}

				fmt.Println()
				fmt.Println(styles.InnerBox.Render(
					styles.SeverityBadge(sev)+" "+styles.ValStyle.Render(a.Message)+"\n"+
						styles.Mute.Render("  Root cause: "+rca.RootCause),
				))
				fmt.Println()
				fmt.Println("  " + styles.KeyStyle.Render("Remediation steps:"))

				applied, skipped, failed := 0, 0, 0
				for i, c := range cmds {
					safety := styles.Good.Render("auto-safe")
					if !c.IsAutoSafe {
						safety = styles.DestructiveBadge.Render("DESTRUCTIVE")
					}
					fmt.Printf("\n  Step %d [%s]: %s\n", i+1, safety, styles.ValStyle.Render(c.Command))

					for {
						fmt.Print("  Execute? [y/N/dry]: ")
						line, _ := reader.ReadString('\n')
						line = strings.TrimSpace(strings.ToLower(line))

						switch line {
						case "dry", "d":
							fmt.Printf("  %s [DRY RUN] %s\n", styles.SevWarning.Render("~"), c.Command)
							continue
						case "y", "yes":
							if !c.IsAutoSafe {
								fmt.Printf("  %s\n", styles.Mute.Render("Skipped — not auto-safe. Use 'veye apply --force'."))
								skipped++
							} else {
								out, err := runCommand(c.Command)
								if err != nil {
									fmt.Printf("  %s %s\n", styles.Bad.Render("✗"), err)
									failed++
								} else {
									fmt.Printf("  %s\n", styles.Good.Render("✓ done"))
									if strings.TrimSpace(out) != "" {
										fmt.Printf("    %s\n", styles.Mute.Render(strings.TrimSpace(out)))
									}
									applied++
								}
							}
						default:
							fmt.Printf("  %s\n", styles.Mute.Render("Skipped."))
							skipped++
						}
						break
					}
				}
				fmt.Printf("\n  Applied: %s  Skipped: %s  Failed: %s\n",
					styles.Good.Render(fmt.Sprintf("%d", applied)),
					styles.Mute.Render(fmt.Sprintf("%d", skipped)),
					styles.Bad.Render(fmt.Sprintf("%d", failed)),
				)
			}
		}

		// ── Recent incidents ──────────────────────────────────────────────────
		inc, _ := api.FullIncidents(5, "", "OPEN", 24)
		if inc != nil && len(inc.Incidents) > 0 {
			fmt.Printf("\n  %s\n", styles.SectionHeader.Render("  Open Incidents (last 24h)"))
			for _, i := range inc.Incidents {
				fmt.Printf("  %s  %s  %s\n",
					sevStyle(i.Severity).Render(i.Severity),
					styles.ValStyle.Render(i.IncidentCode),
					styles.Mute.Render(i.RootCause),
				)
			}
		}

		fmt.Printf("\n  %s\n", styles.Mute.Render(fmt.Sprintf("Next scan in %ds...", watchInterval)))

		select {
		case <-ctx.Done():
			fmt.Println("\n  Watcher stopped.")
			return nil
		case <-time.After(time.Duration(watchInterval) * time.Second):
		}
	}
}
