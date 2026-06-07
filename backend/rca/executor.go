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
// Read-only diagnostic commands run automatically.
// State-changing commands (delete/restart) also run automatically for
// common self-healing operations. Anything else is rejected.
var safeCommandPrefixes = []string{
	// Read-only diagnostics — always safe
	"kubectl get ",
	"kubectl describe ",
	"kubectl logs ",
	"kubectl top ",
	"kubectl rollout status ",
	// Self-healing — auto-execute for common recoverable failures
	"kubectl delete pod ",
	"kubectl rollout restart ",
}

// shellMetachars are characters that indicate an attempt to chain or redirect
// commands. Even though Execute does not use a shell, we reject them as a
// defence-in-depth measure against unexpected allowlist bypasses.
var shellMetachars = []string{"&&", "||", ";", "|", ">", "<", "`", "$("}

// isSafe returns true only if the command starts with an allowlisted prefix
// AND contains no shell metacharacters.
func isSafe(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	for _, meta := range shellMetachars {
		if strings.Contains(trimmed, meta) {
			return false
		}
	}
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

// Execute runs cmd in a shell subprocess under the given parent context.
// It validates against the auto-safe allowlist before executing.
// Returns (stdout+stderr, error).
func (e *Executor) Execute(ctx context.Context, cmd string) (string, error) {
	if !isSafe(cmd) {
		return "", fmt.Errorf("command not in auto-safe allowlist: %q", cmd)
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
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
