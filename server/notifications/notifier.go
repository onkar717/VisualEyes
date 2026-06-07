// Package notifications provides pluggable alert delivery integrations.
package notifications

import (
	"errors"
	"strings"

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
