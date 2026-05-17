package rca

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// safeCommandPrefixes is the strict allowlist for auto-execution.
// Any command not matching one of these prefixes requires manual approval.
var safeCommandPrefixes = []string{
	"kubectl delete pod ",
	"kubectl rollout restart ",
}

// isSafe returns true only if the command starts with an allowlisted prefix.
func isSafe(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	for _, prefix := range safeCommandPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// Executor runs shell commands with a timeout and captures output.
type Executor struct {
	timeout time.Duration
}

// NewExecutor creates an Executor with the given per-command timeout.
func NewExecutor(timeout time.Duration) *Executor {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Executor{timeout: timeout}
}

// Execute runs cmd in a shell subprocess.
// It validates against the auto-safe allowlist before executing.
// Returns (stdout+stderr, error).
func (e *Executor) Execute(cmd string) (string, error) {
	if !isSafe(cmd) {
		return "", fmt.Errorf("command not in auto-safe allowlist: %q", cmd)
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	// Split command into binary + args — avoid shell injection by not using sh -c.
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	slog.Info("auto-executing safe command", "command", cmd)

	c := exec.CommandContext(ctx, parts[0], parts[1:]...)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out

	err := c.Run()
	output := strings.TrimSpace(out.String())

	if err != nil {
		return output, fmt.Errorf("command failed: %w\noutput: %s", err, output)
	}

	slog.Info("command executed successfully", "command", cmd, "output", output)
	return output, nil
}
