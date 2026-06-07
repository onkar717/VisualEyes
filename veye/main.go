// veye is the VisualEyes CLI — a terminal client for the VisualEyes monitoring backend.
// It connects to a running veye-server and surfaces metrics, alerts, pod logs,
// and AI-powered root cause analysis directly in your terminal.
//
// Usage:
//
//	veye status                      # health snapshot
//	veye alerts [--status=all]       # alert table
//	veye logs [--pod P] [--follow]   # pod log stream
//	veye rca <alert-id>              # Claude RCA detail
//	veye watch                       # live Bubbletea dashboard
package main

import "github.com/onkar717/visual-eyes/cli/cmd"

func main() {
	cmd.Execute()
}
