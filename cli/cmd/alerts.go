package cmd

import "github.com/spf13/cobra"

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "List firing alerts (coming in commit 2)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
