package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
	"github.com/spf13/cobra"
)

var (
	apiURL string
	api    *client.Client
)

const bannerArt = `
 ██╗   ██╗██╗███████╗██╗   ██╗ █████╗ ██╗     ███████╗██╗   ██╗███████╗███████╗
 ██║   ██║██║██╔════╝██║   ██║██╔══██╗██║     ██╔════╝╚██╗ ██╔╝██╔════╝██╔════╝
 ██║   ██║██║███████╗██║   ██║███████║██║     █████╗   ╚████╔╝ █████╗  ███████╗
 ╚██╗ ██╔╝██║╚════██║██║   ██║██╔══██║██║     ██╔══╝    ╚██╔╝  ██╔══╝  ╚════██║
  ╚████╔╝ ██║███████║╚██████╔╝██║  ██║███████╗███████╗   ██║   ███████╗███████║
   ╚═══╝  ╚═╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝╚══════╝   ╚═╝   ╚══════╝╚══════╝`

// PrintBanner renders the big VisualEyes ASCII art banner with cyan border.
func PrintBanner() {
	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Render("  Cloud-Native Observability · AI-Powered RCA · Real-Time Monitoring")
	fmt.Println()
	fmt.Println(styles.BannerBox.Render(bannerArt + "\n\n" + subtitle))
	fmt.Println()
}

var rootCmd = &cobra.Command{
	Use:   "veye",
	Short: "VisualEyes CLI   terminal client for your monitoring backend",
	Long: styles.BannerBox.Render(bannerArt) + "\n\n" +
		styles.Mute.Render("  Connect to a running VisualEyes backend and inspect metrics, alerts, logs and RCA results."),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		api = client.New(apiURL)
		if cmd.Name() != "help" {
			if _, err := api.Health(); err != nil {
				return fmt.Errorf("cannot reach VisualEyes backend at %s: %w\nHint: is the server running? (./bin/server)", apiURL, err)
			}
		}
		return nil
	},
}

// Execute is the entry-point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api", "http://localhost:8080", "VisualEyes backend URL")
	rootCmd.AddCommand(statusCmd, alertsCmd, logsCmd, rcaCmd, watchCmd, scanCmd, incidentsCmd, applyCmd, showCmd, reportCmd, clustersCmd)
}
