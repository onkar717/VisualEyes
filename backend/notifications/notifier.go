// Package notifications provides alert delivery integrations (Slack, etc.).
package notifications

import "github.com/onkar717/visual-eyes/backend/models"

// Notifier delivers alert events to an external channel.
type Notifier interface {
	AlertFired(alert models.Alert) error
	AlertResolved(alert models.Alert) error
}

// Noop discards all notifications. Used when no notifier is configured.
type Noop struct{}

func (Noop) AlertFired(models.Alert) error    { return nil }
func (Noop) AlertResolved(models.Alert) error { return nil }
