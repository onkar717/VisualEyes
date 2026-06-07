// Package notifications provides pluggable alert delivery integrations.
package notifications

import (
	"errors"

	"github.com/onkar717/visual-eyes/backend/models"
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
