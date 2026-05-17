package storage

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
)

// MemoryStore is a fully in-memory implementation of MetricStore, QueryableStore,
// AlertStore, LogStore, and RCAStore. It is used as a fallback when PostgreSQL
// is unreachable (dev / no-DB mode).
type MemoryStore struct {
	mu sync.RWMutex

	// metrics: name → time-ordered slice (oldest first, capped at maxPerKey)
	metrics   map[string][]models.Metric
	maxPerKey int

	// alerts
	alerts   []*models.Alert
	alertSeq uint

	// logs
	logs []*models.PodLog
	logSeq uint

	// rca results: alertID → result
	rcaResults map[uint]*models.RCAResult
	rcaSeq     uint
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		metrics:    make(map[string][]models.Metric),
		maxPerKey:  200,
		rcaResults: make(map[uint]*models.RCAResult),
	}
}

// ─── MetricStore ──────────────────────────────────────────────────────────────

func (s *MemoryStore) StoreMetrics(ms []models.Metric) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range ms {
		if m.Timestamp.IsZero() {
			m.Timestamp = time.Now()
		}
		sl := s.metrics[m.Name]
		sl = append(sl, m)
		if len(sl) > s.maxPerKey {
			sl = sl[len(sl)-s.maxPerKey:]
		}
		s.metrics[m.Name] = sl
	}
	return nil
}

func (s *MemoryStore) GetAllMetrics() []models.Metric {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Metric, 0, len(s.metrics))
	for _, sl := range s.metrics {
		if len(sl) > 0 {
			out = append(out, sl[len(sl)-1]) // most recent
		}
	}
	return out
}

// ─── QueryableStore ───────────────────────────────────────────────────────────

func (s *MemoryStore) QueryByName(name string, since time.Time, limit int) ([]models.Metric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sl := s.metrics[name]
	var out []models.Metric
	for i := len(sl) - 1; i >= 0; i-- {
		if sl[i].Timestamp.Before(since) {
			break
		}
		out = append([]models.Metric{sl[i]}, out...)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *MemoryStore) QueryByTags(tags map[string]string, since time.Time, limit int) ([]models.Metric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []models.Metric
	for _, sl := range s.metrics {
		for i := len(sl) - 1; i >= 0; i-- {
			m := sl[i]
			if m.Timestamp.Before(since) {
				break
			}
			if tagsMatch(m.Tags, tags) {
				out = append(out, m)
				if limit > 0 && len(out) >= limit {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

func tagsMatch(src, filter map[string]string) bool {
	for k, v := range filter {
		if sv, ok := src[k]; !ok || sv != v {
			return false
		}
	}
	return true
}

// ─── AlertStore ───────────────────────────────────────────────────────────────

func (s *MemoryStore) SaveAlert(a *models.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alertSeq++
	a.ID = s.alertSeq
	cp := *a
	s.alerts = append(s.alerts, &cp)
	return nil
}

func (s *MemoryStore) UpdateAlert(a *models.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.alerts {
		if existing.ID == a.ID {
			cp := *a
			s.alerts[i] = &cp
			return nil
		}
	}
	return fmt.Errorf("alert %d not found", a.ID)
}

func (s *MemoryStore) GetActiveAlerts() ([]models.Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []models.Alert
	for _, a := range s.alerts {
		if a.Status == "firing" {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (s *MemoryStore) GetAlertHistory(limit int) ([]models.Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Alert, 0, len(s.alerts))
	for i := len(s.alerts) - 1; i >= 0; i-- {
		out = append(out, *s.alerts[i])
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *MemoryStore) GetAlertByID(id uint) (*models.Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.alerts {
		if a.ID == id {
			cp := *a
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("alert %d not found", id)
}

// ─── LogStore ─────────────────────────────────────────────────────────────────

func (s *MemoryStore) StoreLogs(logs []models.PodLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range logs {
		s.logSeq++
		logs[i].ID = s.logSeq
		cp := logs[i]
		s.logs = append(s.logs, &cp)
	}
	// cap at 10 000 lines
	if len(s.logs) > 10000 {
		s.logs = s.logs[len(s.logs)-10000:]
	}
	return nil
}

func (s *MemoryStore) GetLogs(pod, namespace string, limit int) ([]models.PodLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []models.PodLog
	for i := len(s.logs) - 1; i >= 0; i-- {
		l := s.logs[i]
		if (pod == "" || strings.EqualFold(l.Pod, pod)) &&
			(namespace == "" || strings.EqualFold(l.Namespace, namespace)) {
			out = append([]models.PodLog{*l}, out...)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryStore) GetLogsSince(pod, namespace string, since time.Time) ([]models.PodLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []models.PodLog
	for _, l := range s.logs {
		if !l.Timestamp.After(since) {
			continue
		}
		if (pod == "" || strings.EqualFold(l.Pod, pod)) &&
			(namespace == "" || strings.EqualFold(l.Namespace, namespace)) {
			out = append(out, *l)
		}
	}
	return out, nil
}

// ─── RCAStore ─────────────────────────────────────────────────────────────────

func (s *MemoryStore) SaveRCAResult(r *models.RCAResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rcaSeq++
	r.ID = s.rcaSeq
	cp := *r
	s.rcaResults[r.AlertID] = &cp
	return nil
}

func (s *MemoryStore) UpdateRCAResult(r *models.RCAResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *r
	s.rcaResults[r.AlertID] = &cp
	return nil
}

func (s *MemoryStore) GetRCAResult(alertID uint) (*models.RCAResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.rcaResults[alertID]
	if !ok {
		return nil, fmt.Errorf("no rca result for alert %d", alertID)
	}
	cp := *r
	return &cp, nil
}
