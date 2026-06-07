package storage

import (
	"sync"
	"time"
)

// K8sEvent is the canonical event record stored server-side.
// Mirrors the payload shipped by the k8s-agent events collector.
type K8sEvent struct {
	Namespace  string    `json:"namespace"`
	Kind       string    `json:"kind"`
	Object     string    `json:"object"`
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	Type       string    `json:"type"` // Normal | Warning
	Count      int32     `json:"count"`
	LastSeen   time.Time `json:"lastSeen"`
	SourceNode string    `json:"sourceNode"`
}

// EventBuffer is a thread-safe, fixed-capacity ring buffer for K8s events.
// Oldest events are dropped when capacity is exceeded.
type EventBuffer struct {
	mu       sync.RWMutex
	events   []K8sEvent
	capacity int
}

// NewEventBuffer creates an EventBuffer with the given max capacity.
func NewEventBuffer(capacity int) *EventBuffer {
	if capacity <= 0 {
		capacity = 500
	}
	return &EventBuffer{capacity: capacity}
}

// Store appends events, evicting the oldest when capacity is exceeded.
func (b *EventBuffer) Store(events []K8sEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, events...)
	if len(b.events) > b.capacity {
		b.events = b.events[len(b.events)-b.capacity:]
	}
}

// GetRecent returns up to limit Warning events for the given namespace,
// scanning from newest to oldest. Deduplicates by (Reason, Object)   only
// the most recent occurrence of each pair is returned.
// Pass namespace="" to get all namespaces.
func (b *EventBuffer) GetRecent(namespace string, limit int) []K8sEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	seen := make(map[string]bool)
	var out []K8sEvent
	for i := len(b.events) - 1; i >= 0 && len(out) < limit; i-- {
		ev := b.events[i]
		if ev.Type != "Warning" {
			continue
		}
		if namespace != "" && ev.Namespace != namespace {
			continue
		}
		key := ev.Reason + "/" + ev.Object
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	return out
}
