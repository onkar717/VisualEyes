// Package notifications provides pluggable alert delivery integrations.
package notifications

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

// Notifier delivers alert events to an external channel.
type Notifier interface {
	AlertFired(alert models.Alert) error
	AlertResolved(alert models.Alert) error
}

// Noop discards all notifications. Default when no channels are configured.
type Noop struct{}

func (Noop) AlertFired(models.Alert) error    { return nil }
func (Noop) AlertResolved(models.Alert) error { return nil }

// MultiNotifier fans out a single notification to multiple channels in parallel.
// All errors are joined and returned together so a failure in one channel
// does not prevent delivery to others.
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier wraps multiple Notifiers into a single fan-out Notifier.
func NewMultiNotifier(ns ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: ns}
}

func (m *MultiNotifier) AlertFired(alert models.Alert) error {
	return m.fanOut(func(n Notifier) error { return n.AlertFired(alert) })
}

func (m *MultiNotifier) AlertResolved(alert models.Alert) error {
	return m.fanOut(func(n Notifier) error { return n.AlertResolved(alert) })
}

func (m *MultiNotifier) fanOut(fn func(Notifier) error) error {
	var errs []error
	for _, n := range m.notifiers {
		if err := fn(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// SeverityFilteredNotifier wraps a Notifier and only fires when the alert
// severity is in the allowlist. Empty allowlist means all severities pass.
type SeverityFilteredNotifier struct {
	inner      Notifier
	severities map[string]bool // lower-case severity values that are allowed
}

// NewSeverityFilter wraps n and restricts delivery to the given severities.
// Pass an empty slice to allow all severities.
func NewSeverityFilter(n Notifier, severities []string) *SeverityFilteredNotifier {
	set := make(map[string]bool, len(severities))
	for _, s := range severities {
		set[strings.ToLower(s)] = true
	}
	return &SeverityFilteredNotifier{inner: n, severities: set}
}

func (f *SeverityFilteredNotifier) allowed(alert models.Alert) bool {
	if len(f.severities) == 0 {
		return true
	}
	return f.severities[strings.ToLower(string(alert.Severity))]
}

func (f *SeverityFilteredNotifier) AlertFired(alert models.Alert) error {
	if !f.allowed(alert) {
		return nil
	}
	return f.inner.AlertFired(alert)
}

func (f *SeverityFilteredNotifier) AlertResolved(alert models.Alert) error {
	if !f.allowed(alert) {
		return nil
	}
	return f.inner.AlertResolved(alert)
}

// DedupNotifier suppresses re-delivery of the same alert within a time window.
// Prevents notification spam when the same alert repeatedly fires.
type DedupNotifier struct {
	inner     Notifier
	window    time.Duration
	mu        sync.Mutex
	lastFired map[string]time.Time // alert key → last delivery time
}

// NewDedupNotifier wraps n and suppresses AlertFired re-delivery within window.
func NewDedupNotifier(n Notifier, window time.Duration) *DedupNotifier {
	return &DedupNotifier{inner: n, window: window, lastFired: make(map[string]time.Time)}
}

func (d *DedupNotifier) alertKey(alert models.Alert) string {
	return fmt.Sprintf("%d", alert.ID)
}

func (d *DedupNotifier) AlertFired(alert models.Alert) error {
	key := d.alertKey(alert)
	d.mu.Lock()
	last, ok := d.lastFired[key]
	if ok && time.Since(last) < d.window {
		d.mu.Unlock()
		return nil
	}
	d.lastFired[key] = time.Now()
	d.mu.Unlock()
	return d.inner.AlertFired(alert)
}

func (d *DedupNotifier) AlertResolved(alert models.Alert) error {
	d.mu.Lock()
	delete(d.lastFired, d.alertKey(alert))
	d.mu.Unlock()
	return d.inner.AlertResolved(alert)
}
