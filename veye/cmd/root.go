package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	loadDotEnv()

	defaultURL := "http://localhost:8080"
	if u := os.Getenv("VEYE_API_URL"); u != "" {
		defaultURL = u
	}

	rootCmd.PersistentFlags().StringVar(&apiURL, "api", defaultURL,
		"VisualEyes backend URL  (env: VEYE_API_URL, or set in ~/.veye/.env)")
	rootCmd.AddCommand(statusCmd, alertsCmd, logsCmd, rcaCmd, watchCmd, scanCmd, incidentsCmd, applyCmd, showCmd, reportCmd, clustersCmd)
}

// loadDotEnv reads ~/.veye/.env and sets any unset environment variables.
// Users can persist VEYE_API_URL and other settings here without exporting.
func loadDotEnv() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	f, err := os.Open(filepath.Join(home, ".veye", ".env"))
	if err != nil {
		return // file is optional; absence is not an error
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if k != "" && os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
