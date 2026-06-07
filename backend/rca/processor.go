// Package rca implements the AI-driven Root Cause Analysis engine for VisualEyes.
// It consumes fired alerts, assembles rich context, calls Claude, executes safe
// fixes autonomously, and stores full results for the UI to display.
package rca

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/onkar717/visual-eyes/backend/storage"
)

// Processor orchestrates the full RCA pipeline for a single alert:
//  1. Build AlertContext (metrics + logs + sibling alerts)
//  2. Call Claude to get explanation + commands
//  3. Auto-execute safe commands (kubectl delete pod, kubectl rollout restart)
//  4. Persist RCAResult; update Alert.RCAStatus
type Processor struct {
	contextBuilder *ContextBuilder
	claudeClient   *ClaudeClient
	executor       *Executor
	rcaStore       storage.RCAStore
	alertStore     storage.AlertStore
}

// NewProcessor builds a Processor from its dependencies.
func NewProcessor(
	cb *ContextBuilder,
	cc *ClaudeClient,
	ex *Executor,
	rcaStore storage.RCAStore,
	alertStore storage.AlertStore,
) *Processor {
	return &Processor{
		contextBuilder: cb,
		claudeClient:   cc,
		executor:       ex,
		rcaStore:       rcaStore,
		alertStore:     alertStore,
	}
}

// RunWorker reads alerts from alertCh and calls Process for each one.
// It runs concurrently (workerCount goroutines) and respects ctx cancellation.
func (p *Processor) RunWorker(ctx context.Context, alertCh <-chan models.Alert, workerCount int) {
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case alert, ok := <-alertCh:
					if !ok {
						return
					}
					p.Process(ctx, alert)
				}
			}
		}()
	}
	wg.Wait()
}

// Process is the full RCA pipeline for a single alert. It is safe to call
// concurrently from multiple goroutines.
func (p *Processor) Process(ctx context.Context, alert models.Alert) {
	log := slog.With("alert_id", alert.ID, "rule", alert.RuleName)
	log.Info("rca processing started")

	// Mark alert as "running" so the UI shows a spinner.
	p.updateAlertRCAStatus(alert.ID, "running")

	// 1. Build context.
	ac := p.contextBuilder.Build(alert)

	// 2. Create a pending RCAResult immediately so the UI can start polling.
	result := &models.RCAResult{
		AlertID:   alert.ID,
		Status:    "pending",
		Model:     p.claudeClient.model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := p.rcaStore.SaveRCAResult(result); err != nil {
		log.Error("failed to save initial rca result", "error", err)
		p.updateAlertRCAStatus(alert.ID, "failed")
		return
	}
	p.linkRCAToAlert(alert.ID, result.ID)

	// 3. Call Claude.
	resp, inputTokens, err := p.claudeClient.Analyze(ctx, ac)
	if err != nil {
		log.Error("claude rca failed", "error", err)
		result.Status = "failed"
		result.Explanation = "RCA failed: " + err.Error()
		result.UpdatedAt = time.Now()
		p.rcaStore.UpdateRCAResult(result)
		p.updateAlertRCAStatus(alert.ID, "failed")
		return
	}

	// 4. Prepare commands with initial status.
	fixCmds := resp.ToFixCommands()

	// 5. Auto-execute safe commands.
	for i, cmd := range fixCmds {
		if !cmd.IsAutoSafe {
			continue
		}
		output, err := p.executor.Execute(ctx, cmd.Command)
		if err != nil {
			fixCmds[i].Status = models.RemediationFailed
			fixCmds[i].ExecError = err.Error()
			log.Warn("auto-execute failed", "command", cmd.Command, "error", err)
		} else {
			fixCmds[i].Status = models.RemediationExecuted
			fixCmds[i].Output = output
			log.Info("auto-executed fix", "command", cmd.Command)
		}
	}

	// 6. Serialise commands and save final RCAResult.
	cmdsJSON, err := json.Marshal(fixCmds)
	if err != nil {
		log.Error("failed to marshal remediation commands", "error", err)
		cmdsJSON = []byte("[]")
	}
	result.Explanation = resp.Explanation
	result.RootCause = resp.RootCause
	result.Commands = string(cmdsJSON)
	result.Status = "done"
	result.InputTokens = inputTokens
	result.UpdatedAt = time.Now()

	if err := p.rcaStore.UpdateRCAResult(result); err != nil {
		log.Error("failed to update rca result", "error", err)
	}

	p.updateAlertRCAStatus(alert.ID, "done")
	log.Info("rca processing complete", "confidence", resp.Confidence, "commands", len(fixCmds))
}

func (p *Processor) updateAlertRCAStatus(alertID uint, status string) {
	a, err := p.alertStore.GetAlertByID(alertID)
	if err != nil {
		slog.Warn("updateAlertRCAStatus: alert not found", "alert_id", alertID, "error", err)
		return
	}
	a.RCAStatus = status
	if err := p.alertStore.UpdateAlert(a); err != nil {
		slog.Error("updateAlertRCAStatus: failed to update", "alert_id", alertID, "status", status, "error", err)
	}
}

func (p *Processor) linkRCAToAlert(alertID, rcaID uint) {
	a, err := p.alertStore.GetAlertByID(alertID)
	if err != nil {
		slog.Warn("linkRCAToAlert: alert not found", "alert_id", alertID, "error", err)
		return
	}
	a.RCAID = &rcaID
	if err := p.alertStore.UpdateAlert(a); err != nil {
		slog.Error("linkRCAToAlert: failed to update", "alert_id", alertID, "rca_id", rcaID, "error", err)
	}
}
