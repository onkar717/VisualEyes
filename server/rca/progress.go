package rca

import (
	"sync"
	"time"
)

// StageEvent carries real-time progress for a single pipeline stage.
type StageEvent struct {
	AlertID   uint      `json:"alertId"`
	Stage     int       `json:"stage"`             // 1-6
	Label     string    `json:"label"`             // Triage, Metrics, Logs, Infra, Remediation, Commander
	Status    string    `json:"status"`            // running | done | failed
	Detail    string    `json:"detail,omitempty"`  // e.g. "SEV2 · crashloop"
	Elapsed   float64   `json:"elapsed,omitempty"` // seconds (set on done/failed)
	Timestamp time.Time `json:"timestamp"`
}

type progressHub struct {
	mu      sync.Mutex
	subs    map[uint][]chan StageEvent
	history map[uint][]StageEvent
	starts  map[uint]map[int]time.Time
}

var globalHub = &progressHub{
	subs:    make(map[uint][]chan StageEvent),
	history: make(map[uint][]StageEvent),
	starts:  make(map[uint]map[int]time.Time),
}

func (h *progressHub) recordStart(alertID uint, stage int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.starts[alertID] == nil {
		h.starts[alertID] = make(map[int]time.Time)
	}
	h.starts[alertID][stage] = time.Now()
}

func (h *progressHub) elapsed(alertID uint, stage int) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.starts[alertID]; ok {
		if t, ok := m[stage]; ok {
			return time.Since(t).Seconds()
		}
	}
	return 0
}

func (h *progressHub) publish(ev StageEvent) {
	h.mu.Lock()
	h.history[ev.AlertID] = append(h.history[ev.AlertID], ev)
	subs := append([]chan StageEvent(nil), h.subs[ev.AlertID]...)
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (h *progressHub) subscribe(alertID uint) (<-chan StageEvent, func()) {
	ch := make(chan StageEvent, 16)
	h.mu.Lock()
	h.subs[alertID] = append(h.subs[alertID], ch)
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		subs := h.subs[alertID]
		newSubs := make([]chan StageEvent, 0, len(subs))
		for _, c := range subs {
			if c != ch {
				newSubs = append(newSubs, c)
			}
		}
		h.subs[alertID] = newSubs
		h.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (h *progressHub) getHistory(alertID uint) []StageEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]StageEvent, len(h.history[alertID]))
	copy(result, h.history[alertID])
	return result
}

// PublishStageStart records start time and emits a "running" event.
func PublishStageStart(alertID uint, stage int, label string) {
	globalHub.recordStart(alertID, stage)
	globalHub.publish(StageEvent{
		AlertID:   alertID,
		Stage:     stage,
		Label:     label,
		Status:    "running",
		Timestamp: time.Now(),
	})
}

// PublishStageDone emits a "done" event with elapsed time and optional detail.
func PublishStageDone(alertID uint, stage int, label, detail string) {
	globalHub.publish(StageEvent{
		AlertID:   alertID,
		Stage:     stage,
		Label:     label,
		Status:    "done",
		Detail:    detail,
		Elapsed:   globalHub.elapsed(alertID, stage),
		Timestamp: time.Now(),
	})
}

// PublishStageFailed emits a "failed" event.
func PublishStageFailed(alertID uint, stage int, label string) {
	globalHub.publish(StageEvent{
		AlertID:   alertID,
		Stage:     stage,
		Label:     label,
		Status:    "failed",
		Elapsed:   globalHub.elapsed(alertID, stage),
		Timestamp: time.Now(),
	})
}

// SubscribeStage returns a live channel of StageEvents and a cancel func.
func SubscribeStage(alertID uint) (<-chan StageEvent, func()) {
	return globalHub.subscribe(alertID)
}

// StageHistory returns all recorded StageEvents for the alert.
func StageHistory(alertID uint) []StageEvent {
	return globalHub.getHistory(alertID)
}

// IsDone returns true when stage 6 (Commander) has a done or failed event,
// meaning the pipeline finished and no further events will be published.
func IsDone(alertID uint) bool {
	for _, ev := range globalHub.getHistory(alertID) {
		if ev.Stage == 6 && (ev.Status == "done" || ev.Status == "failed") {
			return true
		}
		if ev.Status == "failed" {
			return true
		}
	}
	return false
}
