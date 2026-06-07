// Package rca implements the AI-driven Root Cause Analysis engine for VisualEyes.
// It consumes fired alerts, assembles rich context, runs a multi-stage LLM
// pipeline, auto-executes safe fixes, and persists the full result.
package rca

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/storage"
)

// Processor orchestrates the full RCA pipeline for a single alert:
//  1. Build AlertContext (metrics + logs + sibling alerts)
//  2. Run multi-stage LLM pipeline (triage → diagnosis → remediation → commander)
//  3. Auto-execute safe commands (kubectl delete pod, kubectl rollout restart)
//  4. Persist RCAResult; update Alert.RCAStatus
type Processor struct {
	contextBuilder      *ContextBuilder
	pipeline            *Pipeline
	executor            *Executor
	rcaStore            storage.RCAStore
	alertStore          storage.AlertStore
	incidentStore       storage.IncidentStore       // optional
	remediationLogStore storage.RemediationLogStore // optional
	agentTimeoutSeconds int                         // per-stage LLM timeout; 0 = no limit
}

// SetIncidentStore injects the incident store after construction.
func (p *Processor) SetIncidentStore(s storage.IncidentStore) { p.incidentStore = s }

// SetRemediationLogStore injects the remediation log store after construction.
func (p *Processor) SetRemediationLogStore(s storage.RemediationLogStore) {
	p.remediationLogStore = s
}

// NewProcessor builds a Processor from its dependencies.
func NewProcessor(
	cb *ContextBuilder,
	llm LLMProvider,
	ex *Executor,
	rcaStore storage.RCAStore,
	alertStore storage.AlertStore,
	agentTimeoutSeconds int,
) *Processor {
	maxTokens := 2048
	if claude, ok := llm.(*ClaudeClient); ok {
		maxTokens = claude.maxTokens / 2 // per stage budget
	}
	return &Processor{
		contextBuilder:      cb,
		pipeline:            NewPipeline(llm, maxTokens),
		executor:            ex,
		rcaStore:            rcaStore,
		alertStore:          alertStore,
		agentTimeoutSeconds: agentTimeoutSeconds,
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

// Process is the full RCA pipeline for a single alert. Safe to call concurrently.
func (p *Processor) Process(ctx context.Context, alert models.Alert) {
	log := slog.With("alert_id", alert.ID, "rule", alert.RuleName)
	log.Info("rca processing started")

	p.updateAlertRCAStatus(alert.ID, "running")

	// 1. Build context.
	ac := p.contextBuilder.Build(alert)

	// 2. Create a pending RCAResult immediately so the UI can poll.
	result := &models.RCAResult{
		AlertID:   alert.ID,
		Status:    "pending",
		Model:     p.pipeline.llm.Name(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := p.rcaStore.SaveRCAResult(result); err != nil {
		log.Error("failed to save initial rca result", "error", err)
		p.updateAlertRCAStatus(alert.ID, "failed")
		return
	}
	p.linkRCAToAlert(alert.ID, result.ID)

	// 3. Run multi-stage pipeline with optional timeout (6 stages × per-stage timeout).
	pipelineCtx := ctx
	if p.agentTimeoutSeconds > 0 {
		var cancel context.CancelFunc
		pipelineCtx, cancel = context.WithTimeout(ctx,
			time.Duration(p.agentTimeoutSeconds*6)*time.Second)
		defer cancel()
	}
	resp, inputTokens, err := p.pipeline.RunPipeline(pipelineCtx, ac)
	if err != nil {
		log.Error("rca pipeline failed", "error", err)
		result.Status = "failed"
		result.Explanation = "RCA failed: " + err.Error()
		result.UpdatedAt = time.Now()
		p.rcaStore.UpdateRCAResult(result)
		p.updateAlertRCAStatus(alert.ID, "failed")
		return
	}

	// 4. Prepare commands with initial status.
	fixCmds := resp.ToFixCommands()

	// 5. Auto-execute safe commands and write an audit log entry per step.
	for i, cmd := range fixCmds {
		if !cmd.IsAutoSafe {
			continue
		}
		start := time.Now()
		output, execErr := p.executor.Execute(ctx, cmd.Command)
		durationMs := time.Since(start).Milliseconds()

		if execErr != nil {
			fixCmds[i].Status = models.RemediationFailed
			fixCmds[i].ExecError = execErr.Error()
			log.Warn("auto-execute failed", "command", cmd.Command, "error", execErr)
		} else {
			fixCmds[i].Status = models.RemediationExecuted
			fixCmds[i].Output = output
			log.Info("auto-executed fix", "command", cmd.Command)
		}

		if p.remediationLogStore != nil {
			entry := &models.RemediationLogEntry{
				AlertID:    alert.ID,
				StepNumber: i + 1,
				Command:    cmd.Command,
				Status:     models.RemediationStatus(fixCmds[i].Status),
				Output:     output,
				DurationMs: durationMs,
				ExecutedAt: start,
			}
			if execErr != nil {
				entry.ExecError = execErr.Error()
			}
			_ = p.remediationLogStore.SaveRemediationLog(entry)
		}
	}

	// 6. Marshal all enriched fields and persist.
	cmdsJSON, _ := json.Marshal(fixCmds)
	factorsJSON, _ := json.Marshal(resp.ContributingFactors)
	servicesJSON, _ := json.Marshal(resp.AffectedServices)

	result.Explanation          = resp.Explanation
	result.RootCause            = resp.RootCause
	result.Commands             = string(cmdsJSON)
	result.ConfidenceScore      = resp.Confidence
	result.Severity             = resp.Severity
	result.Category             = resp.Category
	result.ContributingFactors  = string(factorsJSON)
	result.AffectedServices     = string(servicesJSON)
	result.Status               = "done"
	result.InputTokens          = inputTokens
	result.UpdatedAt            = time.Now()

	if err := p.rcaStore.UpdateRCAResult(result); err != nil {
		log.Error("failed to update rca result", "error", err)
	}

	p.updateAlertRCAStatus(alert.ID, "done")

	// 7. Create/update Incident record from RCA output.
	if p.incidentStore != nil {
		p.upsertIncident(alert, result)
	}

	log.Info("rca processing complete",
		"severity", resp.Severity,
		"category", resp.Category,
		"confidence", resp.Confidence,
		"commands", len(fixCmds),
		"tokens", inputTokens,
	)
}

func (p *Processor) upsertIncident(alert models.Alert, result *models.RCAResult) {
	// Check if an incident already exists for this alert.
	existing, _ := p.incidentStore.GetIncidentByAlertID(alert.ID)
	if existing != nil {
		// Update existing incident with fresh RCA data.
		existing.RootCause           = result.RootCause
		existing.ContributingFactors  = result.ContributingFactors
		existing.AffectedServices     = result.AffectedServices
		existing.ConfidenceScore      = result.ConfidenceScore
		existing.Severity             = models.SeverityFromRCA(result.Severity)
		existing.Category             = result.Category
		if existing.Status == models.IncidentOpen {
			existing.Status = models.IncidentInvestigating
		}
		existing.RCAID = &result.ID
		if err := p.incidentStore.UpdateIncident(existing); err != nil {
			slog.Error("upsertIncident: update failed", "alert_id", alert.ID, "error", err)
		}
		return
	}

	title := result.RootCause
	if len(title) > 80 {
		title = title[:77] + "..."
	}
	if title == "" {
		title = alert.RuleName
	}

	now := time.Now()
	inc := &models.Incident{
		IncidentCode:        models.NewIncidentCode(),
		AlertID:             alert.ID,
		RCAID:               &result.ID,
		Title:               title,
		Severity:            models.SeverityFromRCA(result.Severity),
		Category:            result.Category,
		Status:              models.IncidentInvestigating,
		RootCause:           result.RootCause,
		ContributingFactors: result.ContributingFactors,
		AffectedServices:    result.AffectedServices,
		ConfidenceScore:     result.ConfidenceScore,
		DetectedAt:          alert.FiredAt,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := p.incidentStore.SaveIncident(inc); err != nil {
		slog.Error("upsertIncident: save failed", "alert_id", alert.ID, "error", err)
		return
	}
	slog.Info("incident created", "code", inc.IncidentCode, "severity", inc.Severity, "alert_id", alert.ID)
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
