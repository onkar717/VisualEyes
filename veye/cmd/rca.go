package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
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

	// Alert incident card
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

	// Show live pipeline progress while RCA is running/pending
	switch alert.RCAStatus {
	case "running":
		// Definitely started — stream stages, fallback to spinner if SSE unavailable.
		if err := liveStageProgress(id, alert); err != nil {
			if err2 := waitForRCA(id, alert); err2 != nil {
				return err2
			}
		}
	case "pending":
		// May just be queued — try stream briefly; timeout → show pending msg.
		liveStageProgress(id, alert) //nolint:errcheck — idle timeout unblocks naturally
	}

	switch alert.RCAStatus {
	case "failed":
		fmt.Println(styles.Bad.Render("  ✗ RCA analysis failed."))
		fmt.Println()
		return nil
	case "", "pending":
		fmt.Println(styles.Mute.Render("  RCA not yet triggered."))
		fmt.Println(styles.Mute.Render("  Set ANTHROPIC_API_KEY and rca.enabled=true to enable Claude analysis."))
		fmt.Println()
		return nil
	}

	// Fetch full RCA result
	rca, err := api.RCA(id)
	if err != nil {
		return fmt.Errorf("could not fetch RCA for alert %d: %w", id, err)
	}

	// Root cause box
	rootContent := styles.SectionHeader.Render("🔍  ROOT CAUSE") + "\n\n"
	for _, line := range wrap(rca.RootCause, 76) {
		rootContent += "  " + styles.ValStyle.Render(line) + "\n"
	}
	fmt.Println(styles.InnerBox.Render(strings.TrimRight(rootContent, "\n")))
	fmt.Println()

	// Analysis box
	analysisContent := styles.SectionHeader.Render("📋  ANALYSIS") + "\n\n"
	for _, line := range wrap(rca.Explanation, 76) {
		analysisContent += "  " + line + "\n"
	}
	fmt.Println(styles.InnerBox.Render(strings.TrimRight(analysisContent, "\n")))
	fmt.Println()

	// Remediation commands box
	var cmds []client.FixCommand
	if rca.Commands != "" {
		_ = json.Unmarshal([]byte(rca.Commands), &cmds)
	}

	if len(cmds) > 0 {
		cmdContent := styles.SectionHeader.Render("⚡  REMEDIATION COMMANDS") + "\n\n"
		for i, cmd := range cmds {
			safetyLabel := ""
			if cmd.IsAutoSafe {
				safetyLabel = styles.Good.Render(" [auto-safe]")
			} else {
				safetyLabel = styles.DestructiveBadge.Render(" [DESTRUCTIVE]")
			}
			statusLabel := cmdStatusLabel(cmd.Status)

			cmdContent += fmt.Sprintf("  %s  %s%s  %s\n",
				styles.Mute.Render(fmt.Sprintf("[%d]", i+1)),
				styles.ValStyle.Render(cmd.Command),
				safetyLabel,
				statusLabel,
			)
			if cmd.Output != "" {
				for _, l := range strings.Split(cmd.Output, "\n") {
					cmdContent += "      " + styles.Good.Render(l) + "\n"
				}
			}
			if cmd.Error != "" {
				cmdContent += "      " + styles.Bad.Render("error: "+cmd.Error) + "\n"
			}
		}
		fmt.Println(styles.InnerBox.Render(strings.TrimRight(cmdContent, "\n")))
		fmt.Println()
	}

	// Footer
	updAt, _ := time.Parse(time.RFC3339Nano, rca.UpdatedAt)
	fmt.Println(styles.Mute.Render(fmt.Sprintf("  model: %s  ·  %d input tokens  ·  analysed %s ago",
		rca.Model, rca.InputTokens, humanDuration(time.Since(updAt)))))
	fmt.Println()
	return nil
}

// stageEmojis maps stage label → emoji for live progress display.
var stageEmojis = map[string]string{
	"Triage":     "🔍",
	"Metrics":    "📈",
	"Logs":       "📋",
	"Infra":      "🏗",
	"Remediation": "📖",
	"Commander":  "⚡",
}

// liveStageProgress opens an SSE stream and renders live stage progress.
// Returns an error if the stream cannot be established (caller falls back to polling).
func liveStageProgress(alertID uint, alert *client.Alert) error {
	ch, err := api.StreamRCAProgress(alertID)
	if err != nil {
		return err
	}

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	fmt.Println(styles.SectionHeader.Render("  🤖  AI PIPELINE  —  6-STAGE ANALYSIS"))
	fmt.Println()

	currentLabel := ""
	currentRunning := false
	frame := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// If no stage events arrive within 10s, SSE server closed (RCA disabled/idle).
	idleReset := make(chan struct{}, 1)
	idleExpired := make(chan struct{})
	go func() {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-idleReset:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(10 * time.Second)
			case <-timer.C:
				close(idleExpired)
				return
			}
		}
	}()

	renderRunning := func() {
		if !currentRunning || currentLabel == "" {
			return
		}
		emoji := stageEmojis[currentLabel]
		fmt.Printf("\r  %s  %s %-12s  %s     ",
			styles.SevWarning.Render(frames[frame%len(frames)]),
			emoji,
			styles.SectionHeader.Render(currentLabel),
			styles.Mute.Render("running…"),
		)
	}

	done := false
	for !done {
		select {
		case ev, ok := <-ch:
			if !ok {
				done = true
				break
			}
			select {
			case idleReset <- struct{}{}:
			default:
			}
			emoji := stageEmojis[ev.Label]
			if emoji == "" {
				emoji = "●"
			}
			switch ev.Status {
			case "running":
				if currentRunning {
					fmt.Println()
				}
				currentLabel = ev.Label
				currentRunning = true
				renderRunning()
			case "done":
				currentRunning = false
				currentLabel = ""
				detail := ""
				if ev.Detail != "" {
					detail = "   " + styles.Mute.Render(ev.Detail)
				}
				fmt.Printf("\r  %s  %s %-12s  %s%s\n",
					styles.Good.Render("✓"),
					emoji,
					styles.Good.Render(ev.Label),
					styles.Mute.Render(fmt.Sprintf("%.1fs", ev.Elapsed)),
					detail,
				)
			case "failed":
				currentRunning = false
				currentLabel = ""
				fmt.Printf("\r  %s  %s %-12s  %s\n",
					styles.Bad.Render("✗"),
					emoji,
					styles.Bad.Render(ev.Label),
					styles.Mute.Render(fmt.Sprintf("%.1fs", ev.Elapsed)),
				)
			}
		case <-ticker.C:
			frame++
			renderRunning()
		case <-idleExpired:
			done = true
		}
	}

	if currentRunning {
		fmt.Println()
	}
	fmt.Println()

	// Refresh alert status after stream closes
	updated, err := api.AlertByID(alertID)
	if err == nil {
		*alert = *updated
	}
	return nil
}

// waitForRCA is a braille-spinner fallback when SSE stream is unavailable.
func waitForRCA(id uint, alert *client.Alert) error {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	pollEvery := 20
	for i := 0; ; i++ {
		label := "RCA queued"
		if alert.RCAStatus == "running" {
			label = "Claude AI analysing"
		}
		fmt.Printf("\r  %s  %s  ",
			styles.SevWarning.Render(frames[i%len(frames)]),
			styles.Mute.Render(label+"…"),
		)
		time.Sleep(150 * time.Millisecond)
		if i%pollEvery == 0 && i > 0 {
			updated, err := api.AlertByID(id)
			if err == nil {
				*alert = *updated
			}
			if alert.RCAStatus == "done" || alert.RCAStatus == "failed" {
				fmt.Println()
				return nil
			}
		}
	}
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
