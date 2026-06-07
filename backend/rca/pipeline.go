package rca

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// Stage prompts
const triageSystemPrompt = `You are an expert SRE Triage Specialist.
Analyse the Kubernetes alert data and classify the incident.
Output ONLY valid JSON — no markdown, no code fences.

Schema:
{
  "severity": "SEV1|SEV2|SEV3|SEV4",
  "category": "crashloop|oom|high_cpu|high_memory|disk|network|other",
  "title": "concise one-line incident title",
  "has_issue": true|false,
  "affected_services": ["service1", "service2"],
  "confidence": 0-100,
  "summary": "2-3 sentences describing the situation"
}

SEV levels: SEV1=service down, SEV2=major degradation, SEV3=minor, SEV4=healthy.`

const diagnosisSystemPrompt = `You are an expert Kubernetes Root Cause Analyst.
Given triage data and metric/log context, identify the root cause and contributing factors.
Output ONLY valid JSON — no markdown, no code fences.

Schema:
{
  "root_cause": "1-2 sentence precise technical root cause",
  "contributing_factors": ["factor1", "factor2", "factor3"],
  "evidence": ["evidence point 1", "evidence point 2"],
  "confidence": 0-100
}`

const remediationSystemPrompt = `You are a senior SRE Remediation Engineer.
Produce an ordered, EXECUTABLE remediation plan. Every command MUST change cluster state.
NEVER suggest kubectl describe/get/logs as remediation — those are diagnostic.

Output ONLY valid JSON — no markdown, no code fences.

Schema:
{
  "commands": [
    {
      "command": "kubectl ...",
      "description": "what this does and why",
      "is_auto_safe": true|false,
      "risk": "low|medium|high"
    }
  ]
}

AUTO-SAFE RULES — is_auto_safe=true ONLY for:
  kubectl delete pod ...
  kubectl rollout restart ...
Everything else: is_auto_safe=false.`

const commanderSystemPrompt = `You are the Incident Commander.
Synthesise all stage outputs into the final incident report.
Output ONLY valid JSON — no markdown, no code fences.

Schema (exact):
{
  "explanation": "2-3 plain-English sentences describing what is happening",
  "root_cause": "1-2 sentence precise root cause",
  "severity": "SEV1|SEV2|SEV3|SEV4",
  "category": "crashloop|oom|high_cpu|high_memory|disk|network|other",
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

// Pipeline
// Pipeline runs a 5-stage sequential RCA analysis, each stage informed by the
// previous. This produces richer, more accurate results than a single LLM call.
type Pipeline struct {
	llm       LLMProvider
	maxTokens int
}

// NewPipeline creates a Pipeline with the given LLM provider.
func NewPipeline(llm LLMProvider, maxTokens int) *Pipeline {
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	return &Pipeline{llm: llm, maxTokens: maxTokens}
}

// triageStage holds Stage 1 output.
type triageStage struct {
	Severity         string   `json:"severity"`
	Category         string   `json:"category"`
	Title            string   `json:"title"`
	HasIssue         bool     `json:"has_issue"`
	AffectedServices []string `json:"affected_services"`
	Confidence       int      `json:"confidence"`
	Summary          string   `json:"summary"`
}

// diagnosisStage holds Stage 2 output.
type diagnosisStage struct {
	RootCause           string   `json:"root_cause"`
	ContributingFactors []string `json:"contributing_factors"`
	Evidence            []string `json:"evidence"`
	Confidence          int      `json:"confidence"`
}

// RunPipeline executes all 5 stages and returns the final RCAResponse.
// Total input tokens across all stages are summed.
func (p *Pipeline) RunPipeline(ctx context.Context, ac AlertContext) (*RCAResponse, int, error) {
	totalTokens := 0

	// Stage 1: Triage
	slog.Info("rca pipeline stage 1/5: triage", "alert_id", ac.Alert.ID)
	triageUser := fmt.Sprintf("Alert context:\n\n%s", ac.Format())
	triageRaw, tok, err := p.llm.Complete(ctx, triageSystemPrompt, triageUser, p.maxTokens)
	if err != nil {
		return nil, 0, fmt.Errorf("stage 1 triage: %w", err)
	}
	totalTokens += tok

	var triage triageStage
	if err := json.Unmarshal([]byte(stripFences(triageRaw)), &triage); err != nil {
		slog.Warn("triage stage parse failed — using defaults", "error", err)
		triage = triageStage{Severity: "SEV3", Category: "other", HasIssue: true, Confidence: 40}
	}

	// Stage 2: Diagnosis
	slog.Info("rca pipeline stage 2/5: diagnosis", "alert_id", ac.Alert.ID)
	diagUser := fmt.Sprintf(
		"TRIAGE:\n%s\n\nORIGINAL ALERT CONTEXT:\n%s",
		triageRaw, ac.Format(),
	)
	diagRaw, tok, err := p.llm.Complete(ctx, diagnosisSystemPrompt, diagUser, p.maxTokens)
	if err != nil {
		return nil, totalTokens, fmt.Errorf("stage 2 diagnosis: %w", err)
	}
	totalTokens += tok

	var diag diagnosisStage
	if err := json.Unmarshal([]byte(stripFences(diagRaw)), &diag); err != nil {
		slog.Warn("diagnosis stage parse failed", "error", err)
		diag = diagnosisStage{RootCause: "Unable to determine root cause — insufficient signal.", Confidence: 30}
	}

	// Stage 3: Remediation
	slog.Info("rca pipeline stage 3/5: remediation", "alert_id", ac.Alert.ID)
	remUser := fmt.Sprintf(
		"TRIAGE:\n%s\n\nDIAGNOSIS:\n%s\n\nALERT:\n%s",
		truncStage(triageRaw), truncStage(diagRaw), ac.Format(),
	)
	remRaw, tok, err := p.llm.Complete(ctx, remediationSystemPrompt, remUser, p.maxTokens)
	if err != nil {
		return nil, totalTokens, fmt.Errorf("stage 3 remediation: %w", err)
	}
	totalTokens += tok

	// Stage 4: Synthesis (Commander)
	slog.Info("rca pipeline stage 4/5: commander", "alert_id", ac.Alert.ID)
	cmdUser := fmt.Sprintf(
		"TRIAGE:\n%s\n\nDIAGNOSIS:\n%s\n\nREMEDIATION:\n%s\n\nORIGINAL ALERT:\n%s",
		truncStage(triageRaw), truncStage(diagRaw), truncStage(remRaw), ac.Format(),
	)
	finalRaw, tok, err := p.llm.Complete(ctx, commanderSystemPrompt, cmdUser, p.maxTokens)
	if err != nil {
		return nil, totalTokens, fmt.Errorf("stage 4 commander: %w", err)
	}
	totalTokens += tok

	// Parse final output
	var resp RCAResponse
	if err := json.Unmarshal([]byte(stripFences(finalRaw)), &resp); err != nil {
		// Best-effort fallback from parsed stages.
		slog.Warn("commander stage parse failed — building from sub-stages", "error", err)
		resp = RCAResponse{
			Explanation:         triage.Summary,
			RootCause:           diag.RootCause,
			Confidence:          (triage.Confidence + diag.Confidence) / 2,
			Severity:            triage.Severity,
			Category:            triage.Category,
			ContributingFactors: diag.ContributingFactors,
			AffectedServices:    triage.AffectedServices,
		}
	}

	// Ensure severity/category populated from triage if commander omitted them.
	if resp.Severity == "" {
		resp.Severity = triage.Severity
	}
	if resp.Category == "" {
		resp.Category = triage.Category
	}
	if len(resp.AffectedServices) == 0 {
		resp.AffectedServices = triage.AffectedServices
	}
	if len(resp.ContributingFactors) == 0 {
		resp.ContributingFactors = diag.ContributingFactors
	}

	// Enforce auto-safe allowlist.
	for i := range resp.Commands {
		resp.Commands[i].IsAutoSafe = isSafe(resp.Commands[i].Command)
	}

	slog.Info("rca pipeline complete",
		"alert_id", ac.Alert.ID,
		"severity", resp.Severity,
		"category", resp.Category,
		"confidence", resp.Confidence,
		"commands", len(resp.Commands),
		"total_tokens", totalTokens,
	)

	return &resp, totalTokens, nil
}

func truncStage(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1500 {
		return s[:1500] + "…"
	}
	return s
}
