package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/onkar717/visual-eyes/cli/internal/styles"
	"github.com/spf13/cobra"
)

var (
	logsPod       string
	logsNamespace string
	logsContainer string
	logsLimit     int
	logsFollow    bool
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream pod logs from the VisualEyes backend",
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().StringVarP(&logsPod, "pod", "p", "", "Pod name filter (prefix match)")
	logsCmd.Flags().StringVarP(&logsNamespace, "namespace", "n", "", "Namespace filter (empty = all)")
	logsCmd.Flags().StringVarP(&logsContainer, "container", "c", "", "Container filter")
	logsCmd.Flags().IntVarP(&logsLimit, "limit", "l", 200, "Max lines to fetch per request")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Poll for new lines every 5s (Ctrl+C to quit)")
}

func runLogs(_ *cobra.Command, _ []string) error {
	if logsFollow {
		return followLogs()
	}
	return fetchAndPrintLogs(0)
}

func fetchAndPrintLogs(lastID uint) error {
	logs, err := api.Logs(logsPod, logsNamespace, logsContainer, logsLimit)
	if err != nil {
		return err
	}

	printed := 0
	for _, l := range logs {
		if l.ID <= lastID {
			continue
		}
		printLogLine(l.Timestamp, l.Pod, l.Namespace, l.Container, l.Stream, l.Line)
		printed++
	}

	if printed == 0 && lastID == 0 {
		fmt.Println(styles.Mute.Render("  no logs found — ship some with the K8s agent or POST /api/pod-logs"))
	}
	return nil
}

func followLogs() error {
	fmt.Println(styles.Mute.Render("  following logs — Ctrl+C to quit"))
	fmt.Println()

	var lastID uint

	for {
		logs, err := api.Logs(logsPod, logsNamespace, logsContainer, 500)
		if err != nil {
			fmt.Println(styles.Bad.Render("  error: " + err.Error()))
		} else {
			for _, l := range logs {
				if l.ID <= lastID {
					continue
				}
				printLogLine(l.Timestamp, l.Pod, l.Namespace, l.Container, l.Stream, l.Line)
				lastID = l.ID
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func printLogLine(ts, pod, ns, container, stream, line string) {
	// Parse and format timestamp.
	t, err := time.Parse(time.RFC3339Nano, ts)
	timeStr := ts
	if err == nil {
		timeStr = t.Format("15:04:05.000")
	}

	// Colour the stream label.
	var streamLabel string
	if strings.ToLower(stream) == "stderr" {
		streamLabel = styles.Bad.Render("err")
	} else {
		streamLabel = styles.Mute.Render("out")
	}

	// Colour the log line based on stream.
	var lineStr string
	if strings.ToLower(stream) == "stderr" {
		lineStr = styles.SevWarning.Render(line)
	} else {
		lineStr = line
	}

	// Build pod label: namespace/pod[container].
	podLabel := fmt.Sprintf("%s/%s", ns, pod)
	if container != "" {
		podLabel += "[" + container + "]"
	}

	fmt.Printf("%s  %s  %s  %s\n",
		styles.Mute.Render(timeStr),
		streamLabel,
		styles.SevInfo.Render(truncate(podLabel, 38)),
		lineStr,
	)
}
