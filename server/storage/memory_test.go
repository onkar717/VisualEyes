package storage

import (
	"testing"
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
)

// helpers
func metric(name string, value float64, tags map[string]string, ts time.Time) models.Metric {
	return models.Metric{Name: name, Value: value, Tags: tags, Timestamp: ts}
}

func newAlert(rule string, status models.AlertStatus) *models.Alert {
	return &models.Alert{
		RuleName: rule,
		Status:   status,
		FiredAt:  time.Now(),
		Severity: models.SeverityWarning,
	}
}

// MetricStore
func TestMemoryStore_StoreAndGetMetrics(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()

	if err := s.StoreMetrics([]models.Metric{
		metric("cpu", 50, nil, now),
		metric("mem", 70, nil, now),
	}); err != nil {
		t.Fatal(err)
	}

	all := s.GetAllMetrics()
	if len(all) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(all))
	}
}

func TestMemoryStore_GetAllMetrics_LatestOnly(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()

	_ = s.StoreMetrics([]models.Metric{
		metric("cpu", 40, nil, now.Add(-2*time.Second)),
		metric("cpu", 60, nil, now.Add(-time.Second)),
		metric("cpu", 80, nil, now),
	})

	all := s.GetAllMetrics()
	if len(all) != 1 {
		t.Fatalf("expected 1 (latest) metric, got %d", len(all))
	}
	if all[0].Value != 80 {
		t.Errorf("expected latest value 80, got %.0f", all[0].Value)
	}
}

func TestMemoryStore_MaxPerKey(t *testing.T) {
	s := NewMemoryStore()
	s.maxPerKey = 5
	now := time.Now()

	for i := 0; i < 10; i++ {
		_ = s.StoreMetrics([]models.Metric{
			metric("cpu", float64(i), nil, now.Add(time.Duration(i)*time.Second)),
		})
	}

	// QueryByName with large window to get all stored samples.
	got, err := s.QueryByName("cpu", now.Add(-time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("expected max 5 stored, got %d", len(got))
	}
}

// QueryableStore
func TestMemoryStore_QueryByName(t *testing.T) {
	s := NewMemoryStore()
	base := time.Now()

	_ = s.StoreMetrics([]models.Metric{
		metric("cpu", 10, nil, base.Add(-3*time.Second)),
		metric("cpu", 20, nil, base.Add(-2*time.Second)),
		metric("cpu", 30, nil, base.Add(-time.Second)),
	})

	// Query only the last 2 seconds.
	got, err := s.QueryByName("cpu", base.Add(-2*time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
}

func TestMemoryStore_QueryByName_Limit(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()

	for i := 0; i < 10; i++ {
		_ = s.StoreMetrics([]models.Metric{
			metric("cpu", float64(i), nil, now.Add(time.Duration(i)*time.Second)),
		})
	}

	got, err := s.QueryByName("cpu", now.Add(-time.Hour), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 3 {
		t.Errorf("limit=3 but got %d results", len(got))
	}
}

func TestMemoryStore_QueryByTags(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()

	_ = s.StoreMetrics([]models.Metric{
		metric("cpu", 50, map[string]string{"pod": "web-1", "ns": "prod"}, now),
		metric("cpu", 60, map[string]string{"pod": "web-2", "ns": "prod"}, now),
		metric("cpu", 70, map[string]string{"pod": "db-1", "ns": "db"}, now),
	})

	got, err := s.QueryByTags(map[string]string{"ns": "prod"}, now.Add(-time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 prod metrics, got %d", len(got))
	}
}

// AlertStore
func TestMemoryStore_SaveAlert_AssignsID(t *testing.T) {
	s := NewMemoryStore()
	a := newAlert("high-cpu", models.AlertFiring)
	if err := s.SaveAlert(a); err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 {
		t.Error("SaveAlert should assign a non-zero ID")
	}
}

func TestMemoryStore_UpdateAlert(t *testing.T) {
	s := NewMemoryStore()
	a := newAlert("high-cpu", models.AlertFiring)
	_ = s.SaveAlert(a)

	a.Status = models.AlertResolved
	if err := s.UpdateAlert(a); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAlertByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.AlertResolved {
		t.Errorf("expected resolved, got %q", got.Status)
	}
}

func TestMemoryStore_UpdateAlert_NotFound(t *testing.T) {
	s := NewMemoryStore()
	err := s.UpdateAlert(&models.Alert{ID: 999})
	if err == nil {
		t.Error("expected error for non-existent alert")
	}
}

func TestMemoryStore_GetActiveAlerts(t *testing.T) {
	s := NewMemoryStore()
	_ = s.SaveAlert(newAlert("r1", models.AlertFiring))
	_ = s.SaveAlert(newAlert("r2", models.AlertResolved))
	_ = s.SaveAlert(newAlert("r3", models.AlertFiring))

	active, err := s.GetActiveAlerts()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active, got %d", len(active))
	}
}

func TestMemoryStore_GetAlertHistory(t *testing.T) {
	s := NewMemoryStore()
	for i := 0; i < 5; i++ {
		_ = s.SaveAlert(newAlert("rule", models.AlertFiring))
	}

	history, err := s.GetAlertHistory(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(history))
	}
}

func TestMemoryStore_GetAlertByID(t *testing.T) {
	s := NewMemoryStore()
	a := newAlert("high-cpu", models.AlertFiring)
	_ = s.SaveAlert(a)

	got, err := s.GetAlertByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RuleName != "high-cpu" {
		t.Errorf("expected high-cpu, got %q", got.RuleName)
	}

	_, err = s.GetAlertByID(9999)
	if err == nil {
		t.Error("expected error for missing ID")
	}
}

// LogStore
func TestMemoryStore_StoreLogs_AssignsID(t *testing.T) {
	s := NewMemoryStore()
	logs := []models.PodLog{{Pod: "web-1", Namespace: "prod", Line: "started", Timestamp: time.Now()}}
	if err := s.StoreLogs(logs); err != nil {
		t.Fatal(err)
	}
	if logs[0].ID == 0 {
		t.Error("StoreLogs should assign ID")
	}
}

func TestMemoryStore_GetLogs(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()
	_ = s.StoreLogs([]models.PodLog{
		{Pod: "web-1", Namespace: "prod", Line: "line1", Timestamp: now},
		{Pod: "web-1", Namespace: "prod", Line: "line2", Timestamp: now.Add(time.Second)},
		{Pod: "db-1", Namespace: "prod", Line: "db-line", Timestamp: now},
	})

	logs, err := s.GetLogs("web-1", "prod", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs for web-1, got %d", len(logs))
	}
}

func TestMemoryStore_GetLogsSince(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()
	_ = s.StoreLogs([]models.PodLog{
		{Pod: "web-1", Namespace: "prod", Line: "old", Timestamp: now.Add(-10 * time.Second)},
		{Pod: "web-1", Namespace: "prod", Line: "new", Timestamp: now},
	})

	got, err := s.GetLogsSince("web-1", "prod", now.Add(-5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Line != "new" {
		t.Errorf("expected 1 recent log, got %d", len(got))
	}
}

// RCAStore
func TestMemoryStore_RCAResult(t *testing.T) {
	s := NewMemoryStore()

	r := &models.RCAResult{AlertID: 1, Status: "pending"}
	if err := s.SaveRCAResult(r); err != nil {
		t.Fatal(err)
	}
	if r.ID == 0 {
		t.Error("SaveRCAResult should assign ID")
	}

	r.Status = "done"
	r.Explanation = "root cause found"
	if err := s.UpdateRCAResult(r); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetRCAResult(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "done" {
		t.Errorf("expected done, got %q", got.Status)
	}
	if got.Explanation != "root cause found" {
		t.Errorf("unexpected explanation: %q", got.Explanation)
	}
}

func TestMemoryStore_GetRCAResult_NotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.GetRCAResult(99)
	if err == nil {
		t.Error("expected error for missing rca result")
	}
}
