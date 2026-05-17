package cmd

import "github.com/spf13/cobra"

var rcaCmd = &cobra.Command{
	Use:   "rca <alert-id>",
	Short: "Show AI root cause analysis for an alert (coming in commit 2)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
