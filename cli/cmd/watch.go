package cmd

import "github.com/spf13/cobra"

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Interactive live dashboard (coming in commit 4)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
