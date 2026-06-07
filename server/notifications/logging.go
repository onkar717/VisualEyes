package notifications

import (
	"log/slog"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/storage"
)

// LoggingNotifier wraps any Notifier and persists a NotificationEvent record
// for every delivery attempt, regardless of success or failure.
type LoggingNotifier struct {
	inner   Notifier
	channel string
	store   storage.NotificationStore
}

// NewLoggingNotifier wraps inner with persistent event recording.
// channel is a human label for the delivery channel (e.g. "slack", "noop").
func NewLoggingNotifier(inner Notifier, channel string, store storage.NotificationStore) *LoggingNotifier {
	return &LoggingNotifier{inner: inner, channel: channel, store: store}
}

func (l *LoggingNotifier) AlertFired(alert models.Alert) error {
	err := l.inner.AlertFired(alert)
	l.record(alert, "fired", err)
	return err
}

func (l *LoggingNotifier) AlertResolved(alert models.Alert) error {
	err := l.inner.AlertResolved(alert)
	l.record(alert, "resolved", err)
	return err
}

func (l *LoggingNotifier) record(alert models.Alert, eventType string, deliveryErr error) {
	e := &models.NotificationEvent{
		AlertID:   alert.ID,
		RuleName:  alert.RuleName,
		Severity:  string(alert.Severity),
		EventType: eventType,
		Channel:   l.channel,
		Success:   deliveryErr == nil,
		CreatedAt: time.Now(),
	}
	if deliveryErr != nil {
		e.ErrMsg = deliveryErr.Error()
	}
	if err := l.store.SaveNotificationEvent(e); err != nil {
		slog.Warn("failed to persist notification event", "rule", alert.RuleName, "error", err)
	}
}
