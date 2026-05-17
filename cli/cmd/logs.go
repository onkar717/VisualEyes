package cmd

import "github.com/spf13/cobra"

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream pod logs (coming in commit 3)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
