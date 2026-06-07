package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/onkar717/visual-eyes/cli/internal/client"
	"github.com/onkar717/visual-eyes/cli/internal/styles"
	"github.com/spf13/cobra"
)

var (
	applyDryRun bool
	applyForce  bool
)

var applyCmd = &cobra.Command{
	Use:   "apply <alert-id>",
	Short: "Execute the RCA remediation plan for an alert",
	Long: `Fetches the RCA result for the given alert and executes each remediation step.

By default only auto-safe commands run (kubectl delete pod, kubectl rollout restart).
Use --force to run all commands regardless of safety classification.
Use --dry-run to print commands without executing them.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var alertID uint
		if _, err := fmt.Sscanf(args[0], "%d", &alertID); err != nil {
			return fmt.Errorf("invalid alert id %q", args[0])
		}

		rca, err := api.RCA(alertID)
		if err != nil {
			return fmt.Errorf("fetch rca for alert %d: %w", alertID, err)
		}

		if rca.Status != "done" {
			return fmt.Errorf("rca status is %q — must be 'done' before applying", rca.Status)
		}

		var cmds []client.FixCommand
		if err := json.Unmarshal([]byte(rca.Commands), &cmds); err != nil || len(cmds) == 0 {
			fmt.Println(styles.Mute.Render("  No remediation commands in RCA result."))
			return nil
		}

		fmt.Println()
		fmt.Printf("%s  %s\n\n",
			styles.Title.Render("  Remediation Plan"),
			styles.Mute.Render(fmt.Sprintf("alert %d", alertID)),
		)

		if applyDryRun {
			fmt.Println(styles.SevWarning.Render("  [DRY RUN] Commands will be printed but not executed."))
			fmt.Println()
		}

		applied, skipped, failed := 0, 0, 0

		for i, c := range cmds {
			skip := !c.IsAutoSafe && !applyForce
			icon := "○"
			if skip {
				icon = "⊘"
			}

			fmt.Printf("  %s Step %d: %s\n",
				styles.Mute.Render(icon),
				i+1,
				styles.ValStyle.Render(c.Command),
			)

			if skip {
				fmt.Printf("      %s\n\n", styles.Mute.Render("Skipped — not auto-safe. Use --force to execute."))
				skipped++
				continue
			}

			if applyDryRun {
				fmt.Printf("      %s\n\n", styles.SevWarning.Render("[dry-run] would execute"))
				skipped++
				continue
			}

			start := time.Now()
			out, err := runCommand(c.Command)
			dur := time.Since(start).Round(time.Millisecond)

			if err != nil {
				fmt.Printf("      %s %s\n\n",
					styles.Bad.Render("✗ failed:"),
					styles.Mute.Render(err.Error()),
				)
				failed++
			} else {
				fmt.Printf("      %s %s\n",
					styles.Good.Render("✓ done"),
					styles.Mute.Render(fmt.Sprintf("(%s)", dur)),
				)
				if strings.TrimSpace(out) != "" {
					fmt.Printf("      %s\n", styles.Mute.Render(strings.TrimSpace(out)))
				}
				fmt.Println()
				applied++
			}
		}

		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("  Applied: %s   Skipped: %s   Failed: %s\n\n",
			styles.Good.Render(fmt.Sprintf("%d", applied)),
			styles.Mute.Render(fmt.Sprintf("%d", skipped)),
			styles.Bad.Render(fmt.Sprintf("%d", failed)),
		)

		if failed > 0 {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Print commands without executing")
	applyCmd.Flags().BoolVar(&applyForce, "force", false, "Execute all commands including non-auto-safe ones")
}

func runCommand(cmdStr string) (string, error) {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
