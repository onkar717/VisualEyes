// Package alerts implements the anomaly detection engine that evaluates metric
// rules, fires alerts, resolves them when conditions clear, and publishes them
// to a channel consumed by the RCA processor.
package alerts

import (
	"log/slog"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/onkar717/visual-eyes/backend/storage"
)

// Engine polls recent metrics against configured rules at a fixed interval.
// When a threshold is violated it writes an Alert to the store and publishes it
// on the trigger channel for downstream RCA processing. When the condition clears
// the alert is resolved automatically.
type Engine struct {
	store      storage.QueryableStore
	alertStore storage.AlertStore
	rules      []Rule

	evalInterval   time.Duration
	lookbackWindow time.Duration

	// active maps deduplication keys to their current alert IDs so the engine
	// can update (resolve) existing alerts without creating duplicates.
	active map[string]uint
	mu     sync.Mutex

	// trigger is written to when a new alert fires; nil means no downstream.
	trigger chan<- models.Alert

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewEngine builds an Engine from the given stores, rules, and timing parameters.
// trigger may be nil if no downstream processor is needed.
func NewEngine(
	store storage.QueryableStore,
	alertStore storage.AlertStore,
	rules []Rule,
	evalInterval, lookbackWindow time.Duration,
	trigger chan<- models.Alert,
) *Engine {
	return &Engine{
		store:          store,
		alertStore:     alertStore,
		rules:          rules,
		evalInterval:   evalInterval,
		lookbackWindow: lookbackWindow,
		active:         make(map[string]uint),
		trigger:        trigger,
		stopCh:         make(chan struct{}),
	}
}

// Start launches the background evaluation goroutine. Non-blocking.
func (e *Engine) Start() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(e.evalInterval)
		defer ticker.Stop()

		slog.Info("alert engine started",
			"rules", len(e.rules),
			"eval_interval", e.evalInterval,
			"lookback", e.lookbackWindow,
		)

		for {
			select {
			case <-e.stopCh:
				slog.Info("alert engine stopped")
				return
			case <-ticker.C:
				e.evaluate()
			}
		}
	}()
}

// Stop signals the engine to stop and waits for the goroutine to exit.
func (e *Engine) Stop() {
	close(e.stopCh)
	e.wg.Wait()
}

// evaluate runs all rules once and updates alert state.
func (e *Engine) evaluate() {
	since := time.Now().Add(-e.lookbackWindow)

	for _, rule := range e.rules {
		samples, err := e.store.QueryByName(rule.MetricName, since, 50)
		if err != nil {
			slog.Warn("alert eval query failed", "rule", rule.Name, "error", err)
			continue
		}
		if len(samples) == 0 {
			continue
		}

		// Group samples by resource so we generate per-resource alerts.
		byResource := groupByResource(samples)

		for resourceKey, resourceSamples := range byResource {
			resourceID := resourceSamples[0].Tags["pod"]
			if resourceID == "" {
				resourceID = resourceSamples[0].Tags["node"]
			}
			if resourceID == "" {
				resourceID = resourceSamples[0].Tags["hostname"]
			}
			if resourceID == "" {
				resourceID = resourceKey
			}
			namespace := Namespace(resourceSamples[0].Tags)

			violated, value := isViolated(rule, resourceSamples)
			dedupeKey := rule.DedupeKey(resourceID, namespace)

			e.mu.Lock()
			existingID, alreadyFiring := e.active[dedupeKey]
			e.mu.Unlock()

			if violated && !alreadyFiring {
				e.fire(rule, resourceID, namespace, value, dedupeKey)
			} else if !violated && alreadyFiring {
				e.resolve(existingID, dedupeKey)
			}
		}
	}
}

// fire creates a new Alert record and publishes it to the trigger channel.
func (e *Engine) fire(rule Rule, resourceID, namespace string, value float64, dedupeKey string) {
	alert := &models.Alert{
		RuleName:   rule.Name,
		Severity:   rule.Severity,
		Status:     models.AlertFiring,
		ResourceID: resourceID,
		Namespace:  namespace,
		Value:      value,
		Threshold:  rule.Threshold,
		Message:    rule.Message(resourceID, value),
		FiredAt:    time.Now(),
		RCAStatus:  "pending",
	}

	if err := e.alertStore.SaveAlert(alert); err != nil {
		slog.Error("failed to save alert", "rule", rule.Name, "error", err)
		return
	}

	e.mu.Lock()
	e.active[dedupeKey] = alert.ID
	e.mu.Unlock()

	slog.Warn("alert fired",
		"rule", rule.Name,
		"severity", rule.Severity,
		"resource", resourceID,
		"namespace", namespace,
		"value", value,
		"threshold", rule.Threshold,
	)

	if e.trigger != nil {
		select {
		case e.trigger <- *alert:
		default:
			slog.Warn("rca trigger channel full — alert dropped from RCA queue", "alert_id", alert.ID)
		}
	}
}

// resolve updates an existing alert to resolved status.
func (e *Engine) resolve(alertID uint, dedupeKey string) {
	a, err := e.alertStore.GetAlertByID(alertID)
	if err != nil {
		slog.Warn("resolve: alert not found", "id", alertID, "error", err)
		return
	}

	now := time.Now()
	a.Status = models.AlertResolved
	a.ResolvedAt = &now

	if err := e.alertStore.UpdateAlert(a); err != nil {
		slog.Error("failed to resolve alert", "id", alertID, "error", err)
		return
	}

	e.mu.Lock()
	delete(e.active, dedupeKey)
	e.mu.Unlock()

	slog.Info("alert resolved", "id", alertID, "rule", a.RuleName, "resource", a.ResourceID)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// groupByResource groups metric samples by their resource identifier tag.
func groupByResource(samples []models.Metric) map[string][]models.Metric {
	m := make(map[string][]models.Metric)
	for _, s := range samples {
		key := ResourceID(s.Tags)
		m[key] = append(m[key], s)
	}
	return m
}

// isViolated returns true if more than half the samples exceed the threshold,
// preventing single-spike false positives. Also returns the max observed value.
func isViolated(rule Rule, samples []models.Metric) (bool, float64) {
	violations := 0
	maxVal := samples[0].Value
	for _, s := range samples {
		if rule.Evaluate(s.Value) {
			violations++
		}
		if s.Value > maxVal {
			maxVal = s.Value
		}
	}
	// Require at least 50% of samples to be over threshold (noise filter).
	return violations*2 >= len(samples), maxVal
}
