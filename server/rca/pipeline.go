package rca

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// Per-stage token budgets specialist stages need more room than triage.
const (
	tokTriage      = 1024
	tokMetrics     = 1600
	tokLogs        = 1600
	tokInfra       = 1400
	tokRemediation = 2000
	tokCommander   = 3500
)

// ── Stage 1: Triage ──────────────────────────────────────────────────────────

const triageSystemPrompt = `## ROLE
Senior SRE Triage Lead, 10+ years incident response, 24/7 on-call.

## GOAL
Classify this alert in under 30 seconds. Determine real incident vs noise.

## EXPERTISE
- SEV1 = complete service outage. SEV2 = major degradation (>20% users). SEV3 = minor (<5% impact, single component). SEV4 = healthy/noise.
- K8s failure modes: CrashLoopBackOff, OOMKilled, ImagePullBackOff, Evicted, Pending scheduling, NodeNotReady.
- Distinguishing gradual resource creep from sudden spikes. Never over-classify.

## RULES
- Set has_issue=false ONLY if metric is within 10% of threshold AND no corroborating K8s events.
- If K8s Warning events mention OOMKilling, BackOff, or FailedScheduling → at least SEV2.
- CrashloopPods > 0 → at least SEV2.

Output ONLY valid JSON:
{
  "severity": "SEV1|SEV2|SEV3|SEV4",
  "category": "crashloop|oom|high_cpu|high_memory|disk|image_pull|node_not_ready|pending|network|other",
  "title": "concise one-line incident title",
  "has_issue": true|false,
  "affected_services": ["service1"],
  "confidence": 0-100,
  "summary": "2-3 sentences describing exactly what is happening and why it matters"
}`

// ── Stage 2: Metrics Analysis ─────────────────────────────────────────────────

const metricsSystemPrompt = `## ROLE
Kubernetes Metrics & Telemetry Analyst. Expert in kubelet/cAdvisor metrics, Prometheus PromQL, and resource profiling.

## GOAL
Quantify resource pressure. Confirm or refute the triage classification with metric evidence.

## EXPERTISE
- Memory leaks: gradual rise over hours. OOM: sudden spike then drop/restart.
- CPU throttling vs actual usage vs load average they tell different stories.
- Disk: inode exhaustion vs byte exhaustion behave differently.
- Restart count spikes = CrashLoop signal even without explicit event.

## TASK
1. Examine metric samples and anomaly detections in context.
2. Identify which resource shows highest pressure and its trend direction.
3. State whether metrics CONFIRM or CONTRADICT the triage severity.
4. If Prometheus is AVAILABLE in context, provide 2-3 targeted PromQL queries.
5. If only kubelet/agent samples present, extract signal from those only.

Output ONLY valid JSON:
{
  "pressure_level": "low|medium|high|critical",
  "primary_pressure": "cpu|memory|disk|network|none",
  "observations": ["specific finding 1", "specific finding 2", "specific finding 3"],
  "anomalies_found": ["anomaly description"],
  "trend": "rising|stable|spike|degrading|oscillating",
  "confirms_triage": true|false,
  "root_cause_likely": "resource_exhaustion|memory_leak|cpu_spike|disk_full|external|other",
  "suggested_promql": ["query1 with explanation", "query2"]
}`

// ── Stage 3: Log Analysis ─────────────────────────────────────────────────────

const logSystemPrompt = `## ROLE
Log Analysis & Pattern Mining Expert. Specialist in distributed system failure signatures, stack trace parsing, and error correlation.

## GOAL
Extract actionable root-cause signal from log lines. Identify the FIRST failure, not just symptoms.

## EXPERTISE
- OOMKilled: look for "Killed process", "Out of memory", kernel OOM messages in previous container logs.
- CrashLoop: PREVIOUS container logs contain the crash reason always check prev logs first.
- Connection errors: distinguish transient (retry succeeded) from persistent (repeated, escalating).
- Config errors: "no such file", "permission denied", "invalid config", env var missing.
- Application panics: Go "panic:", Java stack traces, Python tracebacks.

## TASK
1. Scan pre-classified log patterns first (highest signal).
2. Check PREVIOUS container logs for crash evidence.
3. Identify the EARLIEST error timestamp (first failure, not latest).
4. Classify error type is this app bug, config error, resource exhaustion, or network?
5. Extract top 3-5 specific error messages (exact strings, not paraphrased).

Output ONLY valid JSON:
{
  "top_errors": ["exact error message 1", "exact error message 2"],
  "error_class": "application_bug|config_error|resource_exhaustion|network|oom|crash|unknown",
  "crash_evidence": ["specific crash indicator 1"],
  "first_seen": "HH:MM:SS or empty if unknown",
  "confirms_triage_category": true|false,
  "pattern_summary": "2 sentences: what the logs prove and what they rule out",
  "confidence": 0-100
}`

// ── Stage 4: Infra Diagnosis ──────────────────────────────────────────────────

const infraSystemPrompt = `## ROLE
Kubernetes Infrastructure Diagnostician. Expert in cluster-level constraints, scheduler behaviour, and resource governance.

## GOAL
Identify infrastructure-level blockers that caused or contributed to the incident. Determine whether root cause is infra-driven or application-driven.

## EXPERTISE
- ResourceQuota exhaustion: silent pod scheduling failures.
- PVC not Bound: blocks any pod that mounts it from starting.
- HPA at max replicas: cluster cannot auto-heal under load.
- Node pressure: CPU/memory > 85% triggers evictions and throttling.
- Taint/toleration mismatch: pods stuck Pending with no visible error.
- Deployment replica mismatch: desired > ready = rolling update stuck or crash.

## TASK
1. Check resource quota and PVC status from context.
2. Check node pressure any node > 85% CPU or > 90% memory is a risk.
3. Check HPA if at max replicas, scaling cannot rescue the service.
4. Check deployment replica mismatches indicates ongoing crash or bad rollout.
5. Decide: is this primarily an INFRA problem (cluster can't support workload) or APP problem (workload is misbehaving)?

Output ONLY valid JSON:
{
  "quota_issues": ["namespace X: cpu quota at 98%"],
  "scheduling_blocks": ["PVC my-pvc not Bound"],
  "node_issues": ["node-1: cpu=91% CRITICAL"],
  "hpa_blocked": ["hpa my-app at max 10/10 replicas"],
  "deployment_issues": ["my-deploy: desired=3 ready=1"],
  "is_infra_root_cause": true|false,
  "primary_block": "one sentence: the single most critical infra constraint",
  "summary": "2 sentences: infra state and its contribution to the incident"
}`

// ── Stage 5: Remediation ──────────────────────────────────────────────────────

const remediationSystemPrompt = `## ROLE
Senior SRE Remediation Engineer. Expert in ordered, safe Kubernetes remediation. You have executed thousands of production incidents.

## GOAL
Produce an ordered, IMMEDIATELY EXECUTABLE remediation plan. Every command must change cluster state.

## RULES NON-NEGOTIABLE
- NEVER use kubectl describe / get / logs / explain these are diagnostic, not remediation.
- Use ACTUAL resource names from the alert context. NEVER use placeholders like {pod}, {namespace}, <name>.
- If you don't know the exact name, omit that command entirely rather than guess.
- Order matters: stabilise first (delete crashlooping pod), then fix root cause (adjust limits/config), then verify.
- is_auto_safe=true ONLY for: kubectl delete pod, kubectl rollout restart. Everything else: false.

## RUNBOOK ALIGNMENT
The matched runbook (below) provides known-good commands for this failure category. Adapt them using actual names from the alert context.

Output ONLY valid JSON:
{
  "commands": [
    {
      "command": "kubectl ...",
      "description": "what this fixes and why 1 sentence",
      "is_auto_safe": true|false,
      "risk": "low|medium|high",
      "step": 1
    }
  ],
  "runbook_used": "runbook name or none",
  "estimated_recovery_minutes": 5
}`

// ── Stage 6: Commander ────────────────────────────────────────────────────────

const commanderSystemPrompt = `## ROLE
Incident Commander. You synthesise all six specialist agent reports into the definitive incident brief that goes to the on-call engineer.

## GOAL
Produce a single, authoritative incident report. The on-call engineer reads this first it must be precise, actionable, and free of contradictions.

## SYNTHESIS RULES
- Root cause must be ONE sentence the precise technical cause, not a restatement of the symptom.
- Explanation must be 2-3 sentences: what happened, why, and what the impact is.
- If metrics agent and log agent disagree on root cause log agent wins (logs are ground truth).
- If infra agent found is_infra_root_cause=true infra block goes into root_cause, app is contributing_factor.
- Commands: use ONLY from the remediation agent output. Use ACTUAL names. NEVER use placeholders.
- Confidence: weight triage(20%) + metrics(25%) + logs(30%) + infra(15%) + remediation(10%).

Output ONLY valid JSON:
{
  "explanation": "2-3 plain-English sentences: what happened, why, impact",
  "root_cause": "1 precise technical sentence",
  "severity": "SEV1|SEV2|SEV3|SEV4",
  "category": "crashloop|oom|high_cpu|high_memory|disk|image_pull|node_not_ready|pending|network|other",
  "contributing_factors": ["factor1", "factor2"],
  "affected_services": ["actual-service-name"],
  "affected_namespaces": ["ns1"],
  "affected_pod_count": 0,
  "service_impacts": [
    {
      "service": "actual-service-name",
      "namespace": "actual-namespace",
      "impact_level": "down|degraded|at_risk",
      "affected_pods": ["pod-name"],
      "error_rate_pct": 0.0,
      "p99_latency_ms": 0.0
    }
  ],
  "evidence": [
    {
      "type": "metric|log|event|node|runbook",
      "source": "prometheus|kubectl logs|k8s events|node exporter",
      "description": "one-line human-readable summary",
      "metric_name": "",
      "metric_value": 0.0,
      "pod_name": "",
      "namespace": ""
    }
  ],
  "remediation_plan": [
    {
      "step_number": 1,
      "description": "what this step does and why",
      "command": "kubectl ...",
      "is_destructive": false,
      "is_automated": true
    }
  ],
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

// ── Intermediate structured types ─────────────────────────────────────────────

type triageStage struct {
	Severity         string   `json:"severity"`
	Category         string   `json:"category"`
	Title            string   `json:"title"`
	HasIssue         bool     `json:"has_issue"`
	AffectedServices []string `json:"affected_services"`
	Confidence       int      `json:"confidence"`
	Summary          string   `json:"summary"`
}

type metricsStage struct {
	PressureLevel   string   `json:"pressure_level"`
	PrimaryPressure string   `json:"primary_pressure"`
	Observations    []string `json:"observations"`
	AnomaliesFound  []string `json:"anomalies_found"`
	Trend           string   `json:"trend"`
	ConfirmsTriage  bool     `json:"confirms_triage"`
	RootCauseLikely string   `json:"root_cause_likely"`
	SuggestedPromQL []string `json:"suggested_promql"`
}

type logStage struct {
	TopErrors              []string `json:"top_errors"`
	ErrorClass             string   `json:"error_class"`
	CrashEvidence          []string `json:"crash_evidence"`
	FirstSeen              string   `json:"first_seen"`
	ConfirmsTriageCategory bool     `json:"confirms_triage_category"`
	PatternSummary         string   `json:"pattern_summary"`
	Confidence             int      `json:"confidence"`
}

type infraStage struct {
	QuotaIssues     []string `json:"quota_issues"`
	SchedulingBlocks []string `json:"scheduling_blocks"`
	NodeIssues      []string `json:"node_issues"`
	HPABlocked      []string `json:"hpa_blocked"`
	DeploymentIssues []string `json:"deployment_issues"`
	IsInfraRootCause bool    `json:"is_infra_root_cause"`
	PrimaryBlock    string   `json:"primary_block"`
	Summary         string   `json:"summary"`
}

type remediationStage struct {
	Commands                []FixCommand `json:"commands"`
	RunbookUsed             string       `json:"runbook_used"`
	EstimatedRecoveryMinutes int         `json:"estimated_recovery_minutes"`
}

// ── Pipeline ──────────────────────────────────────────────────────────────────

// Pipeline runs a 6-stage sequential RCA analysis.
// Each stage is a specialist agent receives structured output from all prior
// stages, producing higher-fidelity signal than single-stage or raw-text chaining.
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

// RunPipeline executes all 6 specialist stages and returns the final RCAResponse.
func (p *Pipeline) RunPipeline(ctx context.Context, ac AlertContext) (*RCAResponse, int, error) {
	total := 0
	alertCtx := ac.Format()

	// ── Stage 1: Triage ───────────────────────────────────────────────────────
	slog.Info("rca stage 1/6: triage", "alert_id", ac.Alert.ID)
	PublishStageStart(ac.Alert.ID, 1, "Triage")

	triageRaw, tok, err := p.llm.Complete(ctx, triageSystemPrompt,
		"Alert context:\n\n"+alertCtx, tokTriage)
	if err != nil {
		PublishStageFailed(ac.Alert.ID, 1, "Triage")
		return nil, 0, fmt.Errorf("stage 1 triage: %w", err)
	}
	total += tok

	var triage triageStage
	if err := json.Unmarshal([]byte(stripFences(triageRaw)), &triage); err != nil {
		slog.Warn("triage parse failed using defaults", "err", err)
		triage = triageStage{Severity: "SEV3", Category: "other", HasIssue: true, Confidence: 40}
	}
	PublishStageDone(ac.Alert.ID, 1, "Triage", fmt.Sprintf("%s · %s", triage.Severity, triage.Category))

	// Healthy fast-exit skip 5 LLM calls when triage says no issue.
	if !triage.HasIssue {
		slog.Info("rca triage: no issue healthy cluster short-circuit", "alert_id", ac.Alert.ID)
		for i := 2; i <= 6; i++ {
			labels := []string{"", "Triage", "Metrics", "Logs", "Infra", "Remediation", "Commander"}
			PublishStageStart(ac.Alert.ID, i, labels[i])
			PublishStageDone(ac.Alert.ID, i, labels[i], "skipped · cluster healthy")
		}
		return &RCAResponse{
			HasIssue:    false,
			Severity:    "SEV4",
			Category:    "healthy",
			Explanation: "All monitored systems are operating within normal parameters.",
			RootCause:   "No anomaly detected by triage agent.",
			Confidence:  triage.Confidence,
			RawOutput:   triageRaw,
		}, total, nil
	}

	// ── Stage 2: Metrics Analysis ─────────────────────────────────────────────
	slog.Info("rca stage 2/6: metrics", "alert_id", ac.Alert.ID)
	PublishStageStart(ac.Alert.ID, 2, "Metrics")

	metricsUser := fmt.Sprintf(
		"## TRIAGE RESULT\n%s\n\n## ALERT + METRIC DATA\n%s",
		truncStage(triageRaw), alertCtx,
	)
	metricsRaw, tok, err := p.llm.Complete(ctx, metricsSystemPrompt, metricsUser, tokMetrics)
	if err != nil {
		PublishStageFailed(ac.Alert.ID, 2, "Metrics")
		return nil, total, fmt.Errorf("stage 2 metrics: %w", err)
	}
	total += tok

	var metrics metricsStage
	if err := json.Unmarshal([]byte(stripFences(metricsRaw)), &metrics); err != nil {
		slog.Warn("metrics parse failed continuing with raw", "err", err)
	}
	metricsDetail := metrics.PressureLevel
	if metrics.PrimaryPressure != "" {
		metricsDetail += " · " + metrics.PrimaryPressure
	}
	PublishStageDone(ac.Alert.ID, 2, "Metrics", metricsDetail)

	// ── Stage 3: Log Analysis ─────────────────────────────────────────────────
	slog.Info("rca stage 3/6: logs", "alert_id", ac.Alert.ID)
	PublishStageStart(ac.Alert.ID, 3, "Logs")

	logUser := fmt.Sprintf(
		"## TRIAGE RESULT\n%s\n\n## METRICS FINDINGS\n%s\n\n## PRE-CLASSIFIED LOG PATTERNS\n%s\n\n## ALERT + LOGS\n%s",
		truncStage(triageRaw),
		truncStage(metricsRaw),
		ac.LogClassification.Summary,
		alertCtx,
	)
	logRaw, tok, err := p.llm.Complete(ctx, logSystemPrompt, logUser, tokLogs)
	if err != nil {
		PublishStageFailed(ac.Alert.ID, 3, "Logs")
		return nil, total, fmt.Errorf("stage 3 logs: %w", err)
	}
	total += tok

	var logs logStage
	if err := json.Unmarshal([]byte(stripFences(logRaw)), &logs); err != nil {
		slog.Warn("log analysis parse failed continuing with raw", "err", err)
	}
	logDetail := logs.ErrorClass
	if logs.Confidence > 0 {
		logDetail += fmt.Sprintf(" · %d%% confidence", logs.Confidence)
	}
	PublishStageDone(ac.Alert.ID, 3, "Logs", logDetail)

	// ── Stage 4: Infra Diagnosis ──────────────────────────────────────────────
	slog.Info("rca stage 4/6: infra", "alert_id", ac.Alert.ID)
	PublishStageStart(ac.Alert.ID, 4, "Infra")

	infraUser := fmt.Sprintf(
		"## TRIAGE RESULT\n%s\n\n## METRICS FINDINGS\n%s\n\n## LOG FINDINGS\n%s\n\n## ALERT + INFRA DATA\n%s",
		truncStage(triageRaw),
		truncStage(metricsRaw),
		truncStage(logRaw),
		alertCtx,
	)
	infraRaw, tok, err := p.llm.Complete(ctx, infraSystemPrompt, infraUser, tokInfra)
	if err != nil {
		PublishStageFailed(ac.Alert.ID, 4, "Infra")
		return nil, total, fmt.Errorf("stage 4 infra: %w", err)
	}
	total += tok

	var infra infraStage
	if err := json.Unmarshal([]byte(stripFences(infraRaw)), &infra); err != nil {
		slog.Warn("infra parse failed continuing with raw", "err", err)
	}
	infraDetail := "app-driven"
	if infra.IsInfraRootCause {
		infraDetail = "infra-driven"
	}
	PublishStageDone(ac.Alert.ID, 4, "Infra", infraDetail)

	// ── Stage 5: Remediation ──────────────────────────────────────────────────
	slog.Info("rca stage 5/6: remediation", "alert_id", ac.Alert.ID, "category", triage.Category)
	PublishStageStart(ac.Alert.ID, 5, "Remediation")

	rb := SelectRunbook(triage.Category)
	remUser := fmt.Sprintf(
		"## TRIAGE RESULT\n%s\n\n## METRICS FINDINGS\n%s\n\n## LOG FINDINGS\n%s\n\n## INFRA FINDINGS\n%s\n\n## MATCHED RUNBOOK\n%s\n\n## ACTUAL RESOURCE NAMES (use verbatim)\npod/resource: %s\nnamespace: %s",
		truncStage(triageRaw),
		truncStage(metricsRaw),
		truncStage(logRaw),
		truncStage(infraRaw),
		RunbookSummary(rb),
		ac.Alert.ResourceID,
		ac.Alert.Namespace,
	)
	remRaw, tok, err := p.llm.Complete(ctx, remediationSystemPrompt, remUser, tokRemediation)
	if err != nil {
		PublishStageFailed(ac.Alert.ID, 5, "Remediation")
		return nil, total, fmt.Errorf("stage 5 remediation: %w", err)
	}
	total += tok

	var rem remediationStage
	if err := json.Unmarshal([]byte(stripFences(remRaw)), &rem); err != nil {
		slog.Warn("remediation parse failed", "err", err)
	}
	PublishStageDone(ac.Alert.ID, 5, "Remediation", fmt.Sprintf("%d commands", len(rem.Commands)))

	// ── Stage 6: Commander ────────────────────────────────────────────────────
	slog.Info("rca stage 6/6: commander", "alert_id", ac.Alert.ID)
	PublishStageStart(ac.Alert.ID, 6, "Commander")

	// Commander receives structured summaries from all 5 agents clean signal, not raw truncated text.
	cmdUser := fmt.Sprintf(
		"## ACTUAL RESOURCE NAMES\npod/resource: %s\nnamespace: %s\n\n## AGENT 1 TRIAGE\n%s\n\n## AGENT 2 METRICS\n%s\n\n## AGENT 3 LOGS\n%s\n\n## AGENT 4 INFRA\n%s\n\n## AGENT 5 REMEDIATION\n%s\n\n## ORIGINAL ALERT\n%s",
		ac.Alert.ResourceID, ac.Alert.Namespace,
		truncStage(triageRaw),
		truncStage(metricsRaw),
		truncStage(logRaw),
		truncStage(infraRaw),
		truncStage(remRaw),
		alertCtx,
	)
	finalRaw, tok, err := p.llm.Complete(ctx, commanderSystemPrompt, cmdUser, tokCommander)
	if err != nil {
		PublishStageFailed(ac.Alert.ID, 6, "Commander")
		return nil, total, fmt.Errorf("stage 6 commander: %w", err)
	}
	total += tok

	var resp RCAResponse
	if err := json.Unmarshal([]byte(stripFences(finalRaw)), &resp); err != nil {
		slog.Warn("commander parse failed building from sub-stages", "err", err)
		resp = RCAResponse{
			HasIssue:         true,
			Explanation:      triage.Summary,
			RootCause:        logs.PatternSummary,
			Confidence:       triage.Confidence,
			Severity:         triage.Severity,
			Category:         triage.Category,
			AffectedServices: triage.AffectedServices,
			Commands:         rem.Commands,
			RunbookUsed:      rem.RunbookUsed,
		}
		// Fallback root cause from infra if logs empty.
		if resp.RootCause == "" && infra.PrimaryBlock != "" {
			resp.RootCause = infra.PrimaryBlock
		}
	}

	resp.RawOutput = finalRaw
	resp.HasIssue = true

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
	if resp.RunbookUsed == "" {
		resp.RunbookUsed = rem.RunbookUsed
	}

	// Enforce auto-safe allowlist regardless of what LLM said.
	for i := range resp.Commands {
		resp.Commands[i].IsAutoSafe = isSafe(resp.Commands[i].Command)
	}

	PublishStageDone(ac.Alert.ID, 6, "Commander",
		fmt.Sprintf("confidence: %d%%", resp.Confidence))

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
	if len(s) > 3000 {
		return s[:3000] + "…"
	}
	return s
}
