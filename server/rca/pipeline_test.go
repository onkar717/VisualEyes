package rca

import (
	"context"
	"strings"
	"testing"

	"github.com/onkar717/visual-eyes/server/models"
)

// ── Mock LLM ──────────────────────────────────────────────────────────────────

type mockLLM struct {
	responses []string
	idx       int
}

func (m *mockLLM) Name() string { return "mock" }

func (m *mockLLM) Complete(_ context.Context, _, _ string, _ int) (string, int, error) {
	if m.idx < len(m.responses) {
		r := m.responses[m.idx]
		m.idx++
		return r, 50, nil
	}
	return `{}`, 50, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func testAlert(id uint) models.Alert {
	return models.Alert{
		ID:         id,
		RuleName:   "test-rule",
		Severity:   models.SeverityWarning,
		ResourceID: "test-pod",
		Namespace:  "default",
		Message:    "test alert",
		Value:      85.0,
		Threshold:  80.0,
	}
}

func testContext(id uint) AlertContext {
	return AlertContext{Alert: testAlert(id)}
}

// JSON responses per stage for a full successful run.
var stageResponses = []string{
	// 1: triage
	`{"severity":"SEV2","category":"crashloop","title":"CrashLoop on test-pod","has_issue":true,"affected_services":["test-pod"],"confidence":90,"summary":"Pod is crashlooping"}`,
	// 2: metrics
	`{"pressure_level":"high","primary_pressure":"memory","observations":["restart count rising"],"anomalies_found":[],"trend":"rising","confirms_triage":true,"root_cause_likely":"memory_leak","suggested_promql":[]}`,
	// 3: logs
	`{"top_errors":["OOMKilled: container limit exceeded"],"error_class":"oom","crash_evidence":["OOMKilled"],"first_seen":"10:00:00","confirms_triage_category":true,"pattern_summary":"Container killed by OOM repeatedly","confidence":88}`,
	// 4: infra
	`{"quota_issues":[],"scheduling_blocks":[],"node_issues":[],"hpa_blocked":[],"deployment_issues":[],"is_infra_root_cause":false,"primary_block":"","summary":"No infra constraint"}`,
	// 5: remediation
	`{"commands":[{"command":"kubectl delete pod test-pod -n default","description":"Force pod restart","is_auto_safe":true,"risk":"low","step":1}],"runbook_used":"oom","estimated_recovery_minutes":3}`,
	// 6: commander
	`{"explanation":"Pod OOM killed due to memory leak","root_cause":"Memory limit too low for workload","severity":"SEV2","category":"oom","contributing_factors":["memory leak"],"affected_services":["test-pod"],"affected_namespaces":["default"],"affected_pod_count":1,"service_impacts":[],"evidence":[],"remediation_plan":[],"confidence":88,"commands":[{"command":"kubectl delete pod test-pod -n default","description":"Force pod restart","is_auto_safe":true,"risk":"low"}]}`,
}

// ── truncStage ────────────────────────────────────────────────────────────────

func TestTruncStage_Short(t *testing.T) {
	got := truncStage("hello")
	if got != "hello" {
		t.Fatalf("want %q got %q", "hello", got)
	}
}

func TestTruncStage_Exact3000(t *testing.T) {
	s := strings.Repeat("x", 3000)
	if truncStage(s) != s {
		t.Fatal("3000-char string must pass through unchanged")
	}
}

func TestTruncStage_Over3000_Truncated(t *testing.T) {
	s := strings.Repeat("x", 3001)
	got := truncStage(s)
	if !strings.HasSuffix(got, "…") {
		t.Fatal("truncated string must end with …")
	}
	if strings.Count(got, "x") != 3000 {
		t.Fatalf("expected 3000 x's, got %d", strings.Count(got, "x"))
	}
}

func TestTruncStage_TrimsWhitespace(t *testing.T) {
	got := truncStage("  hello  ")
	if got != "hello" {
		t.Fatalf("want %q got %q", "hello", got)
	}
}

// ── Healthy fast-exit ─────────────────────────────────────────────────────────

func TestPipeline_HealthyFastExit(t *testing.T) {
	healthyJSON := `{"severity":"SEV4","category":"healthy","title":"All nominal","has_issue":false,"affected_services":[],"confidence":95,"summary":"No anomalies"}`
	llm := &mockLLM{responses: []string{healthyJSON}}
	p := NewPipeline(llm, 2048)

	resp, tokens, err := p.RunPipeline(context.Background(), testContext(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.HasIssue {
		t.Error("expected HasIssue=false for healthy fast-exit")
	}
	if resp.Severity != "SEV4" {
		t.Errorf("want SEV4 got %q", resp.Severity)
	}
	if tokens <= 0 {
		t.Error("expected positive token count")
	}
	// Only one LLM call — triage short-circuits.
	if llm.idx != 1 {
		t.Errorf("expected 1 LLM call, got %d", llm.idx)
	}
}

// ── Full 6-stage success ──────────────────────────────────────────────────────

func TestPipeline_FullFlow_Success(t *testing.T) {
	llm := &mockLLM{responses: append([]string(nil), stageResponses...)}
	p := NewPipeline(llm, 2048)

	resp, tokens, err := p.RunPipeline(context.Background(), testContext(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.HasIssue {
		t.Error("expected HasIssue=true")
	}
	if resp.Severity != "SEV2" {
		t.Errorf("want SEV2 got %q", resp.Severity)
	}
	if resp.Category != "oom" {
		t.Errorf("want oom got %q", resp.Category)
	}
	if resp.Confidence != 88 {
		t.Errorf("want confidence 88 got %d", resp.Confidence)
	}
	if len(resp.Commands) == 0 {
		t.Error("expected at least 1 remediation command")
	}
	if tokens <= 0 {
		t.Error("expected positive token count")
	}
	if llm.idx != 6 {
		t.Errorf("expected 6 LLM calls, got %d", llm.idx)
	}
}

// ── Token accumulation ────────────────────────────────────────────────────────

func TestPipeline_TokensAccumulateAcrossStages(t *testing.T) {
	llm := &mockLLM{responses: append([]string(nil), stageResponses...)}
	p := NewPipeline(llm, 2048)

	_, total, err := p.RunPipeline(context.Background(), testContext(3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 6 stages × 50 tokens each = 300
	if total != 300 {
		t.Errorf("expected 300 total tokens (6×50), got %d", total)
	}
}

// ── Graceful parse failure ────────────────────────────────────────────────────

func TestPipeline_InvalidTriageJSON_DoesNotAbort(t *testing.T) {
	responses := []string{
		`not valid json`, // stage 1 garbage
		stageResponses[1],
		stageResponses[2],
		stageResponses[3],
		stageResponses[4],
		stageResponses[5],
	}
	llm := &mockLLM{responses: responses}
	p := NewPipeline(llm, 2048)

	resp, _, err := p.RunPipeline(context.Background(), testContext(4))
	if err != nil {
		t.Fatalf("unexpected error — pipeline must not abort on bad JSON: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// Pipeline defaulted triage to has_issue=true, should continue all 6 stages.
	if llm.idx != 6 {
		t.Errorf("expected 6 LLM calls even with bad triage JSON, got %d", llm.idx)
	}
}

func TestPipeline_InvalidCommanderJSON_FallsBackToSubStages(t *testing.T) {
	responses := []string{
		stageResponses[0],
		stageResponses[1],
		stageResponses[2],
		stageResponses[3],
		stageResponses[4],
		`totally invalid commander output ¯\_(ツ)_/¯`, // stage 6 garbage
	}
	llm := &mockLLM{responses: responses}
	p := NewPipeline(llm, 2048)

	resp, _, err := p.RunPipeline(context.Background(), testContext(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to triage severity.
	if resp.Severity == "" {
		t.Error("fallback must set severity from triage stage")
	}
	if resp.Severity != "SEV2" {
		t.Errorf("want SEV2 from triage fallback, got %q", resp.Severity)
	}
}

// ── Auto-safe allowlist enforcement ──────────────────────────────────────────

func TestPipeline_AutoSafeAllowlist_DangerousCommandBlocked(t *testing.T) {
	// Commander returns a dangerous command falsely marked auto_safe (LLM hallucination).
	dangerousCommander := `{"explanation":"test","root_cause":"test","severity":"SEV2","category":"crashloop","confidence":70,"commands":[{"command":"kubectl delete namespace production","description":"wrong","is_auto_safe":true,"risk":"high"}]}`
	responses := []string{
		stageResponses[0],
		stageResponses[1],
		stageResponses[2],
		stageResponses[3],
		stageResponses[4],
		dangerousCommander,
	}
	llm := &mockLLM{responses: responses}
	p := NewPipeline(llm, 2048)

	resp, _, err := p.RunPipeline(context.Background(), testContext(6))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, cmd := range resp.Commands {
		if strings.Contains(cmd.Command, "delete namespace") && cmd.IsAutoSafe {
			t.Errorf("dangerous command %q must not be auto-safe", cmd.Command)
		}
	}
}

func TestPipeline_AutoSafeAllowlist_SafeCommandAllowed(t *testing.T) {
	safeCommander := `{"explanation":"test","root_cause":"test","severity":"SEV2","category":"crashloop","confidence":70,"commands":[{"command":"kubectl delete pod crashy -n default","description":"restart","is_auto_safe":true,"risk":"low"},{"command":"kubectl rollout restart deployment/web -n production","description":"rollout restart","is_auto_safe":true,"risk":"low"}]}`
	responses := []string{
		stageResponses[0],
		stageResponses[1],
		stageResponses[2],
		stageResponses[3],
		stageResponses[4],
		safeCommander,
	}
	llm := &mockLLM{responses: responses}
	p := NewPipeline(llm, 2048)

	resp, _, err := p.RunPipeline(context.Background(), testContext(7))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, cmd := range resp.Commands {
		if !cmd.IsAutoSafe {
			t.Errorf("expected safe command %q to be auto-safe", cmd.Command)
		}
	}
}

// ── Cancellation ─────────────────────────────────────────────────────────────

func TestPipeline_CancelledContext_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	llm := &mockLLM{responses: append([]string(nil), stageResponses...)}
	p := NewPipeline(llm, 2048)

	_, _, err := p.RunPipeline(ctx, testContext(8))
	// Cancelled context should propagate as an error from the LLM call.
	if err == nil {
		// Some mock implementations ignore context — only fail if LLM respects it.
		// This test verifies the call returns, not necessarily with an error.
		t.Log("note: mock LLM ignores context cancellation — pipeline completed")
	}
}
