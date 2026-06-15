// Package rca implements the AI-driven Root Cause Analysis engine for VisualEyes.
// It consumes fired alerts, assembles rich context, runs a multi-stage LLM
// pipeline, auto-executes safe fixes, and persists the full result.
package rca

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/storage"
)

// retryWaits defines backoff delays for LLM rate-limit errors (15 / 30 / 60 s).
var retryWaits = []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second}

// isRateLimitError detects 429 / rate-limit responses from any LLM provider.
func isRateLimitError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "ratelimit") ||
		strings.Contains(msg, "quota exceeded") ||
		strings.Contains(msg, "too many requests")
}

// Processor orchestrates the full RCA pipeline for a single alert:
//  1. Build AlertContext (metrics + logs + sibling alerts)
//  2. Run CrewAI Python pipeline if AI_SRE_URL is set and service is reachable
//  3. Fall back to Go multi-stage LLM pipeline if Python service unavailable
//  4. Auto-execute safe commands (kubectl delete pod, kubectl rollout restart)
//  5. Persist RCAResult; update Alert.RCAStatus
type Processor struct {
	contextBuilder      *ContextBuilder
	pipeline            *Pipeline
	pythonClient        *PythonClient  // optional nil when AI_SRE_URL not set
	executor            *Executor
	rcaStore            storage.RCAStore
	alertStore          storage.AlertStore
	incidentStore       storage.IncidentStore       // optional
	remediationLogStore storage.RemediationLogStore // optional
	agentTimeoutSeconds int                         // per-stage LLM timeout; 0 = no limit
	autoRemediate       bool                        // execute auto-safe commands when true
	dryRun              bool                        // log commands but never execute when true
}

// SetIncidentStore injects the incident store after construction.
func (p *Processor) SetIncidentStore(s storage.IncidentStore) { p.incidentStore = s }

// SetRemediationLogStore injects the remediation log store after construction.
func (p *Processor) SetRemediationLogStore(s storage.RemediationLogStore) {
	p.remediationLogStore = s
}

// SetAutoRemediate controls whether auto-safe commands are actually executed.
// When false, safe commands are identified but skipped (dry-run behaviour).
func (p *Processor) SetAutoRemediate(v bool) { p.autoRemediate = v }

// SetDryRun disables all command execution when true, overriding AutoRemediate.
func (p *Processor) SetDryRun(v bool) { p.dryRun = v }

// NewProcessor builds a Processor from its dependencies.
// If AI_SRE_URL is set the processor will call the Python CrewAI service first,
// falling back to the Go pipeline when the service is unreachable.
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

	var pyClient *PythonClient
	if os.Getenv("AI_SRE_URL") != "" {
		// Build callback URL pointing at our own /internal/rca/stage-event endpoint.
		selfURL := os.Getenv("VISUAL_EYES_SELF_URL")
		if selfURL == "" {
			selfURL = "http://localhost:8080"
		}
		pyClient = NewPythonClient(selfURL + "/internal/rca/stage-event")
		slog.Info("python AI-SRE client enabled", "url", os.Getenv("AI_SRE_URL"))
	}

	return &Processor{
		contextBuilder:      cb,
		pipeline:            NewPipeline(llm, maxTokens),
		pythonClient:        pyClient,
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

	// 3. Run RCA pipeline try Python CrewAI service first, fall back to Go pipeline.
	scanStart := time.Now()
	pipelineCtx := ctx
	if p.agentTimeoutSeconds > 0 {
		var cancel context.CancelFunc
		// Python service has its own per-agent timeout; multiply by 2 for safety margin.
		multiplier := 6
		if p.pythonClient != nil {
			multiplier = 12
		}
		pipelineCtx, cancel = context.WithTimeout(ctx,
			time.Duration(p.agentTimeoutSeconds*multiplier)*time.Second)
		defer cancel()
	}

	var resp *RCAResponse
	var inputTokens int

	// Try Python CrewAI pipeline first when available.
	if p.pythonClient != nil {
		if p.pythonClient.IsAvailable(pipelineCtx) {
			log.Info("rca: using Python CrewAI pipeline")
			var pyErr error
			resp, inputTokens, pyErr = p.pythonClient.RunPipeline(pipelineCtx, ac)
			if pyErr != nil {
				log.Warn("rca: Python pipeline failed, falling back to Go pipeline", "error", pyErr)
				resp = nil
			}
		} else {
			log.Warn("rca: Python AI-SRE service unavailable, using Go pipeline")
		}
	}

	// Go pipeline fallback (or primary when Python not configured).
	if resp == nil {
		for attempt := 0; attempt < 3; attempt++ {
			var rcaErr error
			resp, inputTokens, rcaErr = p.pipeline.RunPipeline(pipelineCtx, ac)
			if rcaErr == nil {
				break
			}
			if isRateLimitError(rcaErr) && attempt < 2 {
				wait := retryWaits[attempt]
				log.Warn("rca rate-limit   retrying", "attempt", attempt+1, "wait", wait)
				select {
				case <-time.After(wait):
					continue
				case <-pipelineCtx.Done():
					rcaErr = pipelineCtx.Err()
				}
			}
			log.Error("rca pipeline failed", "error", rcaErr, "attempt", attempt+1)
			result.Status = "failed"
			result.Explanation = "RCA failed: " + rcaErr.Error()
			result.UpdatedAt = time.Now()
			p.rcaStore.UpdateRCAResult(result)
			p.updateAlertRCAStatus(alert.ID, "failed")
			return
		}
	}
	scanDuration := time.Since(scanStart).Seconds()

	// 4. Healthy-cluster short-circuit   record a resolved no-op incident.
	if !resp.HasIssue {
		log.Info("rca: no issue   healthy cluster", "alert_id", alert.ID)
		result.Explanation = resp.Explanation
		result.RootCause   = resp.RootCause
		result.Commands    = "[]"
		result.Severity    = "SEV4"
		result.Category    = "healthy"
		result.Status      = "done"
		result.InputTokens = inputTokens
		result.UpdatedAt   = time.Now()
		_ = p.rcaStore.UpdateRCAResult(result)
		p.updateAlertRCAStatus(alert.ID, "done")
		if p.incidentStore != nil {
			p.createHealthyIncident(alert, result, scanDuration, resp.RawOutput)
		}
		return
	}

	// 5. Prepare commands with initial status.
	fixCmds := resp.ToFixCommands()

	// 6. Auto-execute safe commands and write an audit log entry per step.
	// Skipped entirely when DryRun=true or AutoRemediate=false.
	autoRemediated := false
	for i, cmd := range fixCmds {
		if !cmd.IsAutoSafe {
			continue
		}
		if p.dryRun || !p.autoRemediate {
			log.Info("remediation skipped (dry-run or auto-remediate disabled)", "command", cmd.Command)
			fixCmds[i].Status = models.RemediationPending
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
			autoRemediated = true
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

	// 7. Marshal all enriched fields and persist.
	cmdsJSON, _ := json.Marshal(fixCmds)
	factorsJSON, _ := json.Marshal(resp.ContributingFactors)
	servicesJSON, _ := json.Marshal(resp.AffectedServices)

	result.Explanation         = resp.Explanation
	result.RootCause           = resp.RootCause
	result.Commands            = string(cmdsJSON)
	result.ConfidenceScore     = resp.Confidence
	result.Severity            = resp.Severity
	result.Category            = resp.Category
	result.ContributingFactors = string(factorsJSON)
	result.AffectedServices    = string(servicesJSON)
	result.Status              = "done"
	result.InputTokens         = inputTokens
	result.UpdatedAt           = time.Now()

	if err := p.rcaStore.UpdateRCAResult(result); err != nil {
		log.Error("failed to update rca result", "error", err)
	}

	p.updateAlertRCAStatus(alert.ID, "done")

	// 8. Create/update Incident record from RCA output.
	if p.incidentStore != nil {
		p.upsertIncident(alert, result, autoRemediated, scanDuration, resp.RunbookUsed, resp.RawOutput, resp)
	}

	log.Info("rca processing complete",
		"severity", resp.Severity,
		"category", resp.Category,
		"confidence", resp.Confidence,
		"commands", len(fixCmds),
		"auto_remediated", autoRemediated,
		"scan_secs", scanDuration,
		"tokens", inputTokens,
	)
}

func (p *Processor) upsertIncident(
	alert models.Alert,
	result *models.RCAResult,
	autoRemediated bool,
	scanDurationSecs float64,
	runbookUsed string,
	rawOutput string,
	resp *RCAResponse,
) {
	rawSnip := rawOutput
	if len(rawSnip) > 2000 {
		rawSnip = rawSnip[:2000]
	}

	impactsJSON := "[]"
	if len(resp.ServiceImpacts) > 0 {
		if b, err := json.Marshal(resp.ServiceImpacts); err == nil {
			impactsJSON = string(b)
		}
	}

	evidenceJSON := "[]"
	if len(resp.Evidence) > 0 {
		if b, err := json.Marshal(resp.Evidence); err == nil {
			evidenceJSON = string(b)
		}
	}

	remPlanJSON := "[]"
	if len(resp.RemediationPlan) > 0 {
		steps := make([]models.RemediationStep, len(resp.RemediationPlan))
		for i, s := range resp.RemediationPlan {
			steps[i] = models.RemediationStep{
				StepNumber:    s.StepNumber,
				Description:   s.Description,
				Command:       s.Command,
				IsDestructive: s.IsDestructive,
				IsAutomated:   s.IsAutomated,
				Status:        models.StepPending,
			}
		}
		if b, err := json.Marshal(steps); err == nil {
			remPlanJSON = string(b)
		}
	}

	nsJSON := "[]"
	if len(resp.AffectedNamespaces) > 0 {
		if b, err := json.Marshal(resp.AffectedNamespaces); err == nil {
			nsJSON = string(b)
		}
	} else if alert.Namespace != "" {
		nsJSON = `["` + alert.Namespace + `"]`
	}

	podCount := resp.AffectedPodCount
	if podCount == 0 {
		for _, si := range resp.ServiceImpacts {
			podCount += len(si.AffectedPods)
		}
	}

	// Check if an incident already exists for this alert.
	existing, _ := p.incidentStore.GetIncidentByAlertID(alert.ID)
	if existing != nil {
		existing.RootCause           = result.RootCause
		existing.ContributingFactors  = result.ContributingFactors
		existing.AffectedServices    = result.AffectedServices
		existing.ServiceImpacts      = impactsJSON
		existing.EvidenceItems       = evidenceJSON
		existing.RemediationPlan     = remPlanJSON
		existing.AffectedNamespaces  = nsJSON
		existing.AffectedPodCount    = podCount
		existing.ConfidenceScore     = result.ConfidenceScore
		existing.Severity            = models.SeverityFromRCA(result.Severity)
		existing.Category            = result.Category
		existing.AutoRemediated      = autoRemediated
		existing.ScanDurationSecs    = scanDurationSecs
		existing.RunbookUsed         = runbookUsed
		existing.RawAgentOutput      = rawSnip
		if existing.Status == models.IncidentOpen {
			existing.Status = models.IncidentInvestigating
		}
		existing.RCAID = &result.ID
		if err := p.incidentStore.UpdateIncident(existing); err != nil {
			slog.Error("upsertIncident: update failed", "alert_id", alert.ID, "error", err)
		}
		return
	}

	// Dedup: if an open/investigating incident with same category+namespace exists
	// within last 4 hours, update it instead of creating a duplicate.
	if dup, err := p.incidentStore.FindOpenByCategory(result.Category, alert.Namespace, 4); err == nil && dup != nil {
		dup.RootCause           = result.RootCause
		dup.ContributingFactors  = result.ContributingFactors
		dup.AffectedServices    = result.AffectedServices
		dup.ServiceImpacts      = impactsJSON
		dup.EvidenceItems       = evidenceJSON
		dup.RemediationPlan     = remPlanJSON
		dup.AffectedNamespaces  = nsJSON
		dup.AffectedPodCount    = podCount
		dup.ConfidenceScore     = result.ConfidenceScore
		dup.Severity            = models.SeverityFromRCA(result.Severity)
		dup.AutoRemediated      = autoRemediated || dup.AutoRemediated
		dup.ScanDurationSecs    = scanDurationSecs
		dup.RunbookUsed         = runbookUsed
		dup.RawAgentOutput      = rawSnip
		dup.UpdatedAt           = time.Now()
		if err := p.incidentStore.UpdateIncident(dup); err != nil {
			slog.Error("upsertIncident: dedup update failed", "incident_code", dup.IncidentCode, "error", err)
		} else {
			slog.Info("incident deduped   merged into existing", "code", dup.IncidentCode, "alert_id", alert.ID)
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
		ServiceImpacts:      impactsJSON,
		EvidenceItems:       evidenceJSON,
		RemediationPlan:     remPlanJSON,
		AffectedNamespaces:  nsJSON,
		AffectedPodCount:    podCount,
		ConfidenceScore:     result.ConfidenceScore,
		AutoRemediated:      autoRemediated,
		ScanDurationSecs:    scanDurationSecs,
		RunbookUsed:         runbookUsed,
		RawAgentOutput:      rawSnip,
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

// createHealthyIncident records a no-issue IGNORED incident so operators have a
// full audit trail even when the cluster is healthy.
func (p *Processor) createHealthyIncident(
	alert models.Alert,
	result *models.RCAResult,
	scanDurationSecs float64,
	rawOutput string,
) {
	rawSnip := rawOutput
	if len(rawSnip) > 2000 {
		rawSnip = rawSnip[:2000]
	}
	now := time.Now()
	inc := &models.Incident{
		IncidentCode:     models.NewIncidentCode(),
		AlertID:          alert.ID,
		RCAID:            &result.ID,
		Title:            "Cluster Healthy   No Issues Detected",
		Severity:         models.IncidentSEV4,
		Category:         "healthy",
		Status:           models.IncidentIgnored,
		RootCause:        "All monitored systems operating within normal parameters.",
		ConfidenceScore:  result.ConfidenceScore,
		ScanDurationSecs: scanDurationSecs,
		RawAgentOutput:   rawSnip,
		DetectedAt:       alert.FiredAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := p.incidentStore.SaveIncident(inc); err != nil {
		slog.Error("createHealthyIncident: save failed", "alert_id", alert.ID, "error", err)
	}
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
