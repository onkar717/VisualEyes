package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/storage"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func newTestHandler(t *testing.T) (*Handler, *storage.MemoryStore) {
	t.Helper()
	ms := storage.NewMemoryStore()
	h, err := NewHandler(ms, ms, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	h.SetAlertStore(ms)
	return h, ms
}

func firingAlert(id uint, rcaStatus string) *models.Alert {
	return &models.Alert{
		ID:         id,
		RuleName:   "test-rule",
		Severity:   models.SeverityCritical,
		Status:     models.AlertFiring,
		Message:    "test alert",
		ResourceID: "test-pod",
		Namespace:  "default",
		Value:      95,
		Threshold:  80,
		FiredAt:    time.Now(),
		RCAStatus:  rcaStatus,
	}
}

// ── HandleScanAll ─────────────────────────────────────────────────────────────

func TestHandleScanAll_MethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler(t)
	h.rcaTrigger = make(chan models.Alert, 10)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/rca/scan-all", nil)
	h.HandleScanAll(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestHandleScanAll_NoTrigger_Returns503(t *testing.T) {
	h, _ := newTestHandler(t)
	// rcaTrigger intentionally not wired

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/rca/scan-all", nil)
	h.HandleScanAll(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 when RCA not wired, got %d", rec.Code)
	}
}

func TestHandleScanAll_NoFiringAlerts_Returns202(t *testing.T) {
	h, _ := newTestHandler(t)
	h.rcaTrigger = make(chan models.Alert, 10)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/rca/scan-all", nil)
	h.HandleScanAll(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["triggered"].(float64) != 0 {
		t.Errorf("want triggered=0, got %v", resp["triggered"])
	}
}

func TestHandleScanAll_PendingAlerts_Queued(t *testing.T) {
	h, ms := newTestHandler(t)
	ms.SaveAlert(firingAlert(1, "pending"))
	ms.SaveAlert(firingAlert(2, ""))

	ch := make(chan models.Alert, 10)
	h.rcaTrigger = ch

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/rca/scan-all", nil)
	h.HandleScanAll(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if int(resp["triggered"].(float64)) != 2 {
		t.Errorf("want triggered=2, got %v", resp["triggered"])
	}
	if len(ch) != 2 {
		t.Errorf("want 2 alerts in channel, got %d", len(ch))
	}
}

func TestHandleScanAll_SkipsRunningAndDone(t *testing.T) {
	h, ms := newTestHandler(t)
	ms.SaveAlert(firingAlert(1, "done"))
	ms.SaveAlert(firingAlert(2, "running"))
	ms.SaveAlert(firingAlert(3, "pending"))

	ch := make(chan models.Alert, 10)
	h.rcaTrigger = ch

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/rca/scan-all", nil)
	h.HandleScanAll(rec, req)

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if int(resp["triggered"].(float64)) != 1 {
		t.Errorf("want triggered=1, got %v", resp["triggered"])
	}
	if int(resp["already_processing"].(float64)) != 2 {
		t.Errorf("want already_processing=2, got %v", resp["already_processing"])
	}
}

func TestHandleScanAll_DryRun_DoesNotQueue(t *testing.T) {
	h, ms := newTestHandler(t)
	ms.SaveAlert(firingAlert(1, "pending"))
	ms.SaveAlert(firingAlert(2, ""))

	ch := make(chan models.Alert, 10)
	h.rcaTrigger = ch

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/rca/scan-all?dry_run=true", nil)
	h.HandleScanAll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for dry-run, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["dry_run"] != true {
		t.Error("want dry_run=true in response")
	}
	if int(resp["would_trigger"].(float64)) != 2 {
		t.Errorf("want would_trigger=2, got %v", resp["would_trigger"])
	}
	if len(ch) != 0 {
		t.Errorf("dry-run must not write to channel, got %d queued", len(ch))
	}
}

func TestHandleScanAll_FullChannelCountsAsSkipped(t *testing.T) {
	h, ms := newTestHandler(t)
	ms.SaveAlert(firingAlert(1, "pending"))
	ms.SaveAlert(firingAlert(2, "pending"))

	// Channel capacity 1 — second alert will be dropped to skipped.
	ch := make(chan models.Alert, 1)
	h.rcaTrigger = ch

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/rca/scan-all", nil)
	h.HandleScanAll(rec, req)

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	triggered := int(resp["triggered"].(float64))
	skipped := int(resp["already_processing"].(float64))
	if triggered+skipped != 2 {
		t.Errorf("want triggered+skipped=2, got %d+%d", triggered, skipped)
	}
	if triggered != 1 {
		t.Errorf("want triggered=1 (channel cap 1), got %d", triggered)
	}
}

// ── HandleScan ────────────────────────────────────────────────────────────────

func TestHandleScan_MethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	h.HandleScan(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestHandleScan_NoAlerts_ZeroIssues(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scan", nil)
	h.HandleScan(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if int(resp["issueCount"].(float64)) != 0 {
		t.Errorf("want 0 issues, got %v", resp["issueCount"])
	}
	if resp["overall"] != "healthy" {
		t.Errorf("want overall=healthy, got %v", resp["overall"])
	}
}

func TestHandleScan_WithFiringAlerts_ReturnsIssues(t *testing.T) {
	h, ms := newTestHandler(t)
	ms.SaveAlert(firingAlert(1, "pending"))
	ms.SaveAlert(firingAlert(2, "done"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scan", nil)
	h.HandleScan(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	count := int(resp["issueCount"].(float64))
	if count < 2 {
		t.Errorf("want ≥2 issues for 2 firing alerts, got %d", count)
	}
}

// ── Healthz ───────────────────────────────────────────────────────────────────

func TestHealthz_ReturnsOK(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.Healthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "healthy" {
		t.Errorf("want status=healthy, got %v", resp["status"])
	}
}
