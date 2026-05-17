package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/onkar717/visual-eyes/cli/internal/tui"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Interactive live dashboard — metrics, alerts, logs, RCA",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := tea.NewProgram(tui.New(api), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}
