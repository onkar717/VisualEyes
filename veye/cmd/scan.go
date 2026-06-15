package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
	"github.com/spf13/cobra"
)

var (
	scanApply  bool
	scanForce  bool
	scanAI     bool
	scanDryRun bool
)

var scanStages = []struct {
	emoji string
	label string
}{
	{"🔍", "Triage"},
	{"📈", "Metrics"},
	{"📋", "Logs"},
	{"🏗", "Infra"},
	{"📖", "Runbook"},
	{"⚡", "Commander"},
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Proactive cluster health check   surfaces issues before they page you",
	Long: `Queries the VisualEyes backend for active alerts and metric thresholds,
then prints a prioritised list of findings.

With --ai: triggers a full 6-stage AI RCA scan for every firing alert that
doesn't already have a completed analysis. Streams live pipeline progress.

With --apply: for each critical finding that has an RCA result, prompt
interactively to execute the remediation plan. Use --force to execute all
commands including non-auto-safe ones.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		PrintBanner()

		if scanAI {
			return runAIScan()
		}

		// Show 6-stage progress animation while fetching
		done := make(chan struct{})
		go showScanAnimation(done)

		result, err := api.Scan()
		close(done)
		time.Sleep(50 * time.Millisecond) // let goroutine flush last frame

		if err != nil {
			fmt.Println()
			return fmt.Errorf("scan failed: %w", err)
		}

		fmt.Print("\r" + strings.Repeat(" ", 72) + "\r") // clear spinner line
		printScanResult(result)

		if scanApply && len(result.Issues) > 0 {
			if err := interactiveRemediate(result.Issues); err != nil {
				fmt.Fprintf(os.Stderr, "remediation error: %v\n", err)
			}
		}

		// Exit non-zero when critical issues found so CI/scripts can act on it.
		if result.Overall == "critical" {
			os.Exit(2)
		}
		return nil
	},
}

// runAIScan triggers full 6-stage LLM analysis for all firing alerts.
func runAIScan() error {
	fmt.Println()

	// Show active LLM model if ai-sre service is reachable.
	if info, err := api.AIInfo(); err == nil && info.LLMModel != "" {
		fmt.Printf("  %s  %s\n",
			styles.KeyStyle.Render("AI Model"),
			styles.ValStyle.Render(info.LLMModel),
		)
	}

	if scanDryRun {
		fmt.Println(styles.SectionHeader.Render("  [DRY RUN] Checking which alerts would be scanned..."))
	} else {
		fmt.Println(styles.SectionHeader.Render("  Triggering AI scan for all firing alerts..."))
	}
	fmt.Println()

	r, err := api.ScanAll(scanDryRun)
	if err != nil {
		return err
	}

	countField := r.Triggered
	if scanDryRun {
		countField = r.WouldTrigger
	}

	if countField == 0 {
		if r.AlreadyProcessing > 0 {
			fmt.Printf("  %s  %d alert(s) already have completed or in-progress RCA.\n",
				styles.Good.Render("✓"), r.AlreadyProcessing)
		} else {
			fmt.Println(styles.Good.Render("  ✓ No firing alerts cluster is healthy."))
		}
		fmt.Println()
		return nil
	}

	verb := "Triggered"
	if scanDryRun {
		verb = "Would trigger"
	}
	fmt.Printf("  %s  %s AI analysis for %d alert(s)",
		styles.SevWarning.Render("→"), verb, countField)
	if r.AlreadyProcessing > 0 {
		fmt.Printf("  %s", styles.Mute.Render(fmt.Sprintf("(%d already processing)", r.AlreadyProcessing)))
	}
	fmt.Println()
	fmt.Println()

	if scanDryRun {
		fmt.Println(styles.Mute.Render("  (dry-run no analysis started)"))
		fmt.Println()
		for _, a := range r.Alerts {
			fmt.Printf("  %s  %s  %s\n",
				styles.SeverityBadge(a.Severity),
				styles.ValStyle.Render(a.Message),
				styles.Mute.Render(fmt.Sprintf("alert #%d · %s", a.ID, a.Resource)),
			)
		}
		fmt.Println()
		return nil
	}

	stageLabels := []string{"", "Triage", "Metrics", "Logs", "Infra", "Remediation", "Commander"}

	for _, a := range r.Alerts {
		sevBadge := styles.SeverityBadge(a.Severity)
		fmt.Printf("  %s  %s\n  %s\n\n",
			sevBadge,
			styles.ValStyle.Render(a.Message),
			styles.Mute.Render(fmt.Sprintf("alert #%d · %s", a.ID, a.Resource)),
		)

		ch, err := api.StreamRCAProgress(a.ID)
		if err != nil {
			fmt.Printf("  %s\n\n", styles.Mute.Render("(could not stream progress RCA running in background)"))
			continue
		}

		stageStatus := make([]string, 7) // index 1-6
		for ev := range ch {
			if ev.Stage < 1 || ev.Stage > 6 {
				continue
			}
			label := stageLabels[ev.Stage]
			switch ev.Status {
			case "start":
				stageStatus[ev.Stage] = styles.SevWarning.Render("…")
			case "done":
				detail := ""
				if ev.Detail != "" {
					detail = "  " + styles.Mute.Render(ev.Detail)
				}
				stageStatus[ev.Stage] = styles.Good.Render("✓")
				fmt.Printf("    %s  %s%s\n",
					stageStatus[ev.Stage],
					styles.KeyStyle.Render(label),
					detail,
				)
			case "failed":
				stageStatus[ev.Stage] = styles.Bad.Render("✗")
				fmt.Printf("    %s  %s\n", stageStatus[ev.Stage], styles.Bad.Render(label+" failed"))
			}
		}

		// Show RCA result summary.
		rca, err := api.RCA(a.ID)
		if err == nil && rca.Status == "done" && rca.RootCause != "" {
			fmt.Println()
			fmt.Printf("    %s  %s\n", styles.KeyStyle.Render("Root cause"), wordWrap(rca.RootCause, 64))
		}
		fmt.Println()
	}

	if scanApply {
		issues := make([]client.ScanIssue, 0, len(r.Alerts))
		for _, a := range r.Alerts {
			issues = append(issues, client.ScanIssue{
				Severity: a.Severity,
				Category: "ai-scan",
				Resource: a.Resource,
				Message:  a.Message,
				AlertID:  a.ID,
			})
		}
		if err := interactiveRemediate(issues); err != nil {
			fmt.Fprintf(os.Stderr, "remediation error: %v\n", err)
		}
	}

	return nil
}

func init() {
	scanCmd.Flags().BoolVar(&scanAI, "ai", false, "Trigger full 6-stage AI RCA scan for all firing alerts")
	scanCmd.Flags().BoolVar(&scanDryRun, "dry-run", false, "Show which alerts would be scanned without starting analysis (use with --ai)")
	scanCmd.Flags().BoolVar(&scanApply, "apply", false, "Interactively prompt to apply remediation for each critical finding")
	scanCmd.Flags().BoolVar(&scanForce, "force", false, "Execute all commands including non-auto-safe ones (use with --apply)")
}

// showScanAnimation renders a 6-stage pipeline progress line until done is closed.
func showScanAnimation(done <-chan struct{}) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	stageIdx := 0
	tick := 0
	advanceEvery := 12 // rotate stage every ~1.8s

	for {
		select {
		case <-done:
			return
		default:
		}

		if tick > 0 && tick%advanceEvery == 0 && stageIdx < len(scanStages)-1 {
			stageIdx++
		}

		line := "  " + styles.SevWarning.Render(frames[tick%len(frames)]) + "  "
		for i, s := range scanStages {
			if i < stageIdx {
				line += styles.Good.Render(s.emoji+" "+s.label+"  ")
			} else if i == stageIdx {
				line += styles.SectionHeader.Render(s.emoji+" "+s.label) + styles.Mute.Render("…  ")
			} else {
				line += styles.Mute.Render(s.emoji+" "+s.label+"  ")
			}
		}
		fmt.Printf("\r%s", line)
		time.Sleep(150 * time.Millisecond)
		tick++
	}
}

// interactiveRemediate walks through critical findings that have an RCA result
// and asks the operator whether to execute each remediation plan.
func interactiveRemediate(issues []client.ScanIssue) error {
	reader := bufio.NewReader(os.Stdin)
	remediated := 0

	for _, issue := range issues {
		if issue.Severity != "critical" || issue.AlertID == 0 {
			continue
		}

		rca, err := api.RCA(uint(issue.AlertID))
		if err != nil || rca.Status != "done" {
			continue
		}

		var cmds []client.FixCommand
		if err := json.Unmarshal([]byte(rca.Commands), &cmds); err != nil || len(cmds) == 0 {
			continue
		}

		fmt.Println()
		fmt.Println(styles.InnerBox.Render(
			styles.Bad.Render("[CRITICAL]") + "  " + styles.ValStyle.Render(issue.Message) + "\n" +
				styles.Mute.Render("  Root cause: "+rca.RootCause),
		))
		fmt.Println()

		fmt.Println()
		fmt.Println("   🔧 " + styles.KeyStyle.Render("PROPOSED REMEDIATION:"))

		applied, skipped, failed := 0, 0, 0
		for i, c := range cmds {
			safety := styles.Good.Render("auto-safe")
			if !c.IsAutoSafe {
				safety = styles.DestructiveBadge.Render("DESTRUCTIVE")
			}
			fmt.Printf("\n   Step %d: [%s]\n   Command: %s\n", i+1, safety, styles.ValStyle.Render(c.Command))
			
			for {
				fmt.Printf("   Execute? [y/N/dry]: ")
				line, _ := reader.ReadString('\n')
				line = strings.TrimSpace(strings.ToLower(line))
				
				if line == "dry" || line == "d" {
					fmt.Printf("   %s [DRY RUN] Would execute: %s\n", styles.SevWarning.Render("~"), c.Command)
					continue
				} else if line == "y" || line == "yes" {
					if !c.IsAutoSafe && !scanForce {
						fmt.Printf("   %s %s\n", styles.Bad.Render("✗"), styles.Mute.Render("skipped   not auto-safe. Use --force to execute."))
						skipped++
						break
					}
					
					start := time.Now()
					out, err := runCommand(c.Command)
					dur := time.Since(start).Round(time.Millisecond)
					if err != nil {
						fmt.Printf("   %s %s\n", styles.Bad.Render("✗"), styles.Mute.Render(err.Error()))
						failed++
					} else {
						fmt.Printf("   %s (%s)\n", styles.Good.Render("✓"), dur)
						if strings.TrimSpace(out) != "" {
							fmt.Printf("          %s\n", styles.Mute.Render(strings.TrimSpace(out)))
						}
						applied++
					}
					break
				} else {
					fmt.Printf("   %s %s\n", styles.Mute.Render("-"), styles.Mute.Render("skipped."))
					skipped++
					break
				}
			}
		}
		fmt.Printf("\n   Applied: %s  Skipped: %s  Failed: %s\n",
			styles.Good.Render(fmt.Sprintf("%d", applied)),
			styles.Mute.Render(fmt.Sprintf("%d", skipped)),
			styles.Bad.Render(fmt.Sprintf("%d", failed)),
		)
		remediated++
	}

	if remediated == 0 {
		fmt.Println()
		fmt.Println(styles.Mute.Render("  No remediable critical findings with completed RCA."))
	}
	return nil
}

func printScanResult(r *client.ScanResult) {
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
		fmt.Println(styles.Good.Render("  ✓ No issues found. Everything looks healthy."))
		fmt.Println()
		return
	}

	fmt.Println(styles.SectionHeader.Render(fmt.Sprintf("  🔍  Findings (%d)", r.IssueCount)))
	fmt.Println()

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
