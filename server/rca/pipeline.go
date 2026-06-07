package rca

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// --- Stage 1: Triage ---
const triageSystemPrompt = `You are an expert SRE Triage Specialist.
Analyse the alert and classify the incident. Output ONLY valid JSON.

Schema:
{
  "severity": "SEV1|SEV2|SEV3|SEV4",
  "category": "crashloop|oom|high_cpu|high_memory|disk|image_pull|node_not_ready|pending|network|other",
  "title": "concise one-line incident title",
  "has_issue": true|false,
  "affected_services": ["service1"],
  "confidence": 0-100,
  "summary": "2-3 sentences describing the situation"
}

SEV: SEV1=service down, SEV2=major degradation, SEV3=minor, SEV4=healthy.`

// --- Stage 2: Metrics Analysis ---
const metricsSystemPrompt = `You are a Kubernetes Metrics & Telemetry Analyst.
Given triage classification and raw metric samples, analyse resource pressure.
Output a concise prose summary (max 200 words) covering:
1. Which resources show highest pressure (CPU/memory/disk)
2. Trend direction (rising, stable, spike)
3. Whether resource exhaustion is the likely root cause
4. Estimated restart rate from counters if available`

// --- Stage 3: Log Analysis ---
const logSystemPrompt = `You are a Log Analysis & Pattern Mining Expert.
Given triage, metrics summary, and pod log lines, identify root-cause error patterns.
Output a concise prose summary (max 200 words) covering:
1. Top error messages (exceptions, panics, OOM, connection errors)
2. Error classification: application bug / config error / resource exhaustion / network
3. First occurrence timestamp if determinable
4. For CrashLoopBackOff: focus on PREVIOUS container logs`

// --- Stage 4: Infra Diagnosis ---
const infraSystemPrompt = `You are a Kubernetes Infrastructure Diagnostician.
Based on triage, metrics, and log evidence, diagnose infrastructure constraints.
Output a concise prose summary (max 200 words) covering:
1. Resource quota hits in any namespace
2. Scheduling constraints (taints, affinity, PVC binding)
3. Node health — any nodes NotReady or cordoned?
4. Whether root cause is infra-driven or application-driven`

// --- Stage 5: Remediation ---
const remediationSystemPrompt = `You are a senior SRE Remediation Engineer.
Produce an ordered, EXECUTABLE remediation plan informed by the runbook and prior analysis.
Every command MUST change cluster state. NEVER use kubectl describe/get/logs.

Output ONLY valid JSON:
{
  "commands": [
    {
      "command": "kubectl ...",
      "description": "what this fixes and why",
      "is_auto_safe": true|false,
      "risk": "low|medium|high"
    }
  ],
  "runbook_used": "runbook name or none"
}

AUTO-SAFE RULES — is_auto_safe=true ONLY for:
  kubectl delete pod ...
  kubectl rollout restart ...
Everything else: is_auto_safe=false.`

// --- Stage 6: Commander ---
const commanderSystemPrompt = `You are the Incident Commander.
Synthesise all six stage outputs into the definitive incident report.
Output ONLY valid JSON:

{
  "explanation": "2-3 plain-English sentences describing what is happening",
  "root_cause": "1-2 sentence precise root cause",
  "severity": "SEV1|SEV2|SEV3|SEV4",
  "category": "crashloop|oom|high_cpu|high_memory|disk|image_pull|node_not_ready|pending|network|other",
  "contributing_factors": ["factor1", "factor2"],
  "affected_services": ["svc1"],
  "confidence": 0-100,
  "commands": [
    {
      "command": "kubectl ...",
      "description": "one-line description",
      "is_auto_safe": true|false,
      "risk": "low|medium|high"
    }
  ]
}`

// Pipeline runs a 6-stage sequential RCA analysis.
// Each stage receives structured output from all prior stages, producing
// higher-fidelity signal than single-stage or raw-text chaining.
type Pipeline struct {
	llm       LLMProvider
	maxTokens int
}

func NewPipeline(llm LLMProvider, maxTokens int) *Pipeline {
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	return &Pipeline{llm: llm, maxTokens: maxTokens}
}

type triageStage struct {
	Severity         string   `json:"severity"`
	Category         string   `json:"category"`
	Title            string   `json:"title"`
	HasIssue         bool     `json:"has_issue"`
	AffectedServices []string `json:"affected_services"`
	Confidence       int      `json:"confidence"`
	Summary          string   `json:"summary"`
}

type remediationStage struct {
	Commands    []FixCommand `json:"commands"`
	RunbookUsed string       `json:"runbook_used"`
}

// RunPipeline executes all 6 stages and returns the final RCAResponse.
func (p *Pipeline) RunPipeline(ctx context.Context, ac AlertContext) (*RCAResponse, int, error) {
	total := 0

	// Stage 1: Triage
	slog.Info("rca stage 1/6: triage", "alert_id", ac.Alert.ID)
	triageRaw, tok, err := p.llm.Complete(ctx, triageSystemPrompt,
		"Alert context:\n\n"+ac.Format(), p.maxTokens)
	if err != nil {
		return nil, 0, fmt.Errorf("stage 1 triage: %w", err)
	}
	total += tok

	var triage triageStage
	if err := json.Unmarshal([]byte(stripFences(triageRaw)), &triage); err != nil {
		slog.Warn("triage parse failed — defaults", "err", err)
		triage = triageStage{Severity: "SEV3", Category: "other", HasIssue: true, Confidence: 40}
	}

	// Stage 2: Metrics Analysis
	slog.Info("rca stage 2/6: metrics analysis", "alert_id", ac.Alert.ID)
	metricsUser := fmt.Sprintf("TRIAGE:\n%s\n\nMETRIC DATA:\n%s",
		truncStage(triageRaw), ac.Format())
	metricsRaw, tok, err := p.llm.Complete(ctx, metricsSystemPrompt, metricsUser, p.maxTokens)
	if err != nil {
		return nil, total, fmt.Errorf("stage 2 metrics: %w", err)
	}
	total += tok

	// Stage 3: Log Analysis
	slog.Info("rca stage 3/6: log analysis", "alert_id", ac.Alert.ID)
	logUser := fmt.Sprintf("TRIAGE:\n%s\n\nMETRICS:\n%s\n\nALERT+LOGS:\n%s",
		truncStage(triageRaw), truncStage(metricsRaw), ac.Format())
	logRaw, tok, err := p.llm.Complete(ctx, logSystemPrompt, logUser, p.maxTokens)
	if err != nil {
		return nil, total, fmt.Errorf("stage 3 logs: %w", err)
	}
	total += tok

	// Stage 4: Infra Diagnosis
	slog.Info("rca stage 4/6: infra diagnosis", "alert_id", ac.Alert.ID)
	infraUser := fmt.Sprintf("TRIAGE:\n%s\n\nMETRICS:\n%s\n\nLOGS:\n%s\n\nALERT:\n%s",
		truncStage(triageRaw), truncStage(metricsRaw), truncStage(logRaw), ac.Format())
	infraRaw, tok, err := p.llm.Complete(ctx, infraSystemPrompt, infraUser, p.maxTokens)
	if err != nil {
		return nil, total, fmt.Errorf("stage 4 infra: %w", err)
	}
	total += tok

	// Stage 5: Remediation (with runbook injection)
	slog.Info("rca stage 5/6: remediation", "alert_id", ac.Alert.ID, "category", triage.Category)
	rb := SelectRunbook(triage.Category)
	runbookContext := RunbookSummary(rb)
	remUser := fmt.Sprintf(
		"TRIAGE:\n%s\n\nMETRICS:\n%s\n\nLOGS:\n%s\n\nINFRA:\n%s\n\nMATCHED RUNBOOK:\n%s",
		truncStage(triageRaw), truncStage(metricsRaw),
		truncStage(logRaw), truncStage(infraRaw), runbookContext,
	)
	remRaw, tok, err := p.llm.Complete(ctx, remediationSystemPrompt, remUser, p.maxTokens)
	if err != nil {
		return nil, total, fmt.Errorf("stage 5 remediation: %w", err)
	}
	total += tok

	var rem remediationStage
	if err := json.Unmarshal([]byte(stripFences(remRaw)), &rem); err != nil {
		slog.Warn("remediation parse failed", "err", err)
	}

	// Stage 6: Commander — synthesise all stages
	slog.Info("rca stage 6/6: commander", "alert_id", ac.Alert.ID)
	cmdUser := fmt.Sprintf(
		"TRIAGE:\n%s\n\nMETRICS:\n%s\n\nLOGS:\n%s\n\nINFRA:\n%s\n\nREMEDIATION:\n%s\n\nORIGINAL ALERT:\n%s",
		truncStage(triageRaw), truncStage(metricsRaw), truncStage(logRaw),
		truncStage(infraRaw), truncStage(remRaw), ac.Format(),
	)
	finalRaw, tok, err := p.llm.Complete(ctx, commanderSystemPrompt, cmdUser, p.maxTokens)
	if err != nil {
		return nil, total, fmt.Errorf("stage 6 commander: %w", err)
	}
	total += tok

	var resp RCAResponse
	if err := json.Unmarshal([]byte(stripFences(finalRaw)), &resp); err != nil {
		slog.Warn("commander parse failed — building from sub-stages", "err", err)
		resp = RCAResponse{
			Explanation:         triage.Summary,
			RootCause:           "See log and infra analysis",
			Confidence:          triage.Confidence,
			Severity:            triage.Severity,
			Category:            triage.Category,
			ContributingFactors: nil,
			AffectedServices:    triage.AffectedServices,
			Commands:            rem.Commands,
		}
	}

	// Fill gaps from sub-stages when commander omits fields.
	if resp.Severity == "" {
		resp.Severity = triage.Severity
	}
	if resp.Category == "" {
		resp.Category = triage.Category
	}
	if len(resp.AffectedServices) == 0 {
		resp.AffectedServices = triage.AffectedServices
	}
	if len(resp.Commands) == 0 {
		resp.Commands = rem.Commands
	}

	// Enforce auto-safe allowlist regardless of what LLM said.
	for i := range resp.Commands {
		resp.Commands[i].IsAutoSafe = isSafe(resp.Commands[i].Command)
	}

	slog.Info("rca pipeline complete",
		"alert_id", ac.Alert.ID,
		"severity", resp.Severity,
		"category", resp.Category,
		"confidence", resp.Confidence,
		"commands", len(resp.Commands),
		"total_tokens", total,
	)

	return &resp, total, nil
}

func truncStage(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1500 {
		return s[:1500] + "…"
	}
	return s
}
