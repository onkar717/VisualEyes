package rca

import (
	"fmt"
	"strings"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/storage"
)

// AlertContext bundles everything the LLM pipeline needs for high-quality RCA.
type AlertContext struct {
	Alert          models.Alert
	RecentMetrics  []models.Metric // last N samples of the triggering metric
	RelatedMetrics []models.Metric // samples of related metrics on same resource (problem pods only)
	PodLogs        []models.PodLog // current container log lines
	PrevLogs       []models.PodLog // previous container logs (pre-crash evidence for crashloop)
	SiblingAlerts  []models.Alert  // other firing alerts on the same resource
	// Pre-classified log patterns — populated by ClassifyLogs before LLM stages.
	LogClassification ClassifiedLogs
	// Detected metric anomalies (Z-score ≥ 2.5σ over recent samples).
	Anomalies []AnomalyResult
	// Observability stack availability — hints for the LLM agents.
	PrometheusURL     string
	PrometheusEnabled bool
	LokiURL           string
	LokiEnabled       bool
}

// ContextBuilder assembles AlertContext from multiple stores.
type ContextBuilder struct {
	metricStore    storage.QueryableStore
	logStore       storage.LogStore // may be nil
	alertStore     storage.AlertStore
	logLines       int
	metricSamples  int
	prometheusURL     string
	prometheusEnabled bool
	prometheusClient  *PrometheusClient // non-nil when prometheusEnabled and URL set
	lokiURL        string
	lokiEnabled    bool
	lokiClient     *LokiClient // non-nil when lokiEnabled and lokiURL set
}

// NewContextBuilder creates a ContextBuilder with the given stores and limits.
func NewContextBuilder(
	ms storage.QueryableStore,
	ls storage.LogStore,
	as storage.AlertStore,
	logLines, metricSamples int,
) *ContextBuilder {
	return &ContextBuilder{
		metricStore:   ms,
		logStore:      ls,
		alertStore:    as,
		logLines:      logLines,
		metricSamples: metricSamples,
	}
}

// SetPrometheus injects Prometheus connection info into context.
// When enabled and url is non-empty, a PrometheusClient is created for live metric queries.
func (b *ContextBuilder) SetPrometheus(url string, enabled bool) {
	b.prometheusURL = url
	b.prometheusEnabled = enabled
	if enabled && url != "" {
		b.prometheusClient = NewPrometheusClient(url)
	}
}

// SetLoki injects Loki connection info into context.
// When enabled and url is non-empty, a LokiClient is created for live log queries.
func (b *ContextBuilder) SetLoki(url string, enabled bool) {
	b.lokiURL = url
	b.lokiEnabled = enabled
	if enabled && url != "" {
		b.lokiClient = NewLokiClient(url)
	}
}

// Build assembles a complete AlertContext for the given alert.
func (b *ContextBuilder) Build(alert models.Alert) AlertContext {
	since := time.Now().Add(-30 * time.Minute)
	ctx := AlertContext{
		Alert:             alert,
		PrometheusURL:     b.prometheusURL,
		PrometheusEnabled: b.prometheusEnabled,
		LokiURL:           b.lokiURL,
		LokiEnabled:       b.lokiEnabled,
	}

	// Primary metric samples — use MetricName (e.g. "cpu.usage"), not RuleName.
	metricName := alert.MetricName
	if metricName == "" {
		metricName = alert.RuleName // fallback for alerts stored before MetricName was added
	}
	if samples, err := b.metricStore.QueryByName(metricName, since, b.metricSamples); err == nil {
		ctx.RecentMetrics = samples
		// Run Z-score anomaly detection on the primary metric time series.
		ctx.Anomalies = DetectAnomalies(samples)
	}

	// Related system metrics for same resource — filter to problem pods only.
	relatedNames := relatedMetrics(alert.RuleName)
	for _, name := range relatedNames {
		if samples, err := b.metricStore.QueryByName(name, since, 10); err == nil {
			ctx.RelatedMetrics = append(ctx.RelatedMetrics, samples...)
		}
	}
	// Remove healthy idle pods from related metrics to reduce LLM token usage.
	ctx.RelatedMetrics = FilterProblemPods(ctx.RelatedMetrics)

	// Supplement with Prometheus PromQL metrics when available.
	// For pod-related alerts, fetch CPU and memory via standard cAdvisor queries.
	if b.prometheusClient != nil && looksLikePod(alert.ResourceID) {
		if promSamples, err := b.prometheusClient.QueryRange(
			coreCPUQuery(alert.ResourceID, alert.Namespace),
			30*time.Minute, 30*time.Second,
		); err == nil {
			ctx.RelatedMetrics = append(ctx.RelatedMetrics, promSamples...)
		}
		if promSamples, err := b.prometheusClient.QueryRange(
			coreMemQuery(alert.ResourceID, alert.Namespace),
			30*time.Minute, 30*time.Second,
		); err == nil {
			ctx.RelatedMetrics = append(ctx.RelatedMetrics, promSamples...)
		}
	}

	// Pod logs — prefer Loki when enabled; fall back to stored push logs.
	if looksLikePod(alert.ResourceID) {
		if b.lokiClient != nil {
			// Query Loki for live logs (last 30 min, up to logLines).
			if lokiLines, err := b.lokiClient.QueryLogs(alert.ResourceID, alert.Namespace,
				30*time.Minute, b.logLines); err == nil && len(lokiLines) > 0 {
				ctx.PodLogs = lokiLines
			}
		}
		// Always supplement with stored logs (includes prev-stream from k8s-agent).
		if b.logStore != nil {
			if logLines, err := b.logStore.GetLogs(alert.ResourceID, alert.Namespace, b.logLines); err == nil {
				for _, l := range logLines {
					if l.Stream == "prev" || l.Stream == "previous" {
						ctx.PrevLogs = append(ctx.PrevLogs, l)
					} else if b.lokiClient == nil {
						// Only use stored stdout/stderr if Loki is not providing them.
						ctx.PodLogs = append(ctx.PodLogs, l)
					}
				}
			}
		}
	}

	// Pre-classify logs deterministically before handing to LLM.
	ctx.LogClassification = ClassifyLogs(ctx.PodLogs, ctx.PrevLogs)

	// Sibling alerts on the same resource (cap at 10 to bound prompt size).
	if all, err := b.alertStore.GetActiveAlerts(); err == nil {
		for _, a := range all {
			if a.ID != alert.ID && a.ResourceID == alert.ResourceID {
				ctx.SiblingAlerts = append(ctx.SiblingAlerts, a)
				if len(ctx.SiblingAlerts) >= 10 {
					break
				}
			}
		}
	}

	// Cap related metrics to prevent oversized prompts on high-cardinality clusters.
	const maxRelatedMetrics = 50
	if len(ctx.RelatedMetrics) > maxRelatedMetrics {
		ctx.RelatedMetrics = ctx.RelatedMetrics[:maxRelatedMetrics]
	}

	return ctx
}

// Format serialises the context into a structured prompt section for Claude.
func (c AlertContext) Format() string {
	var b strings.Builder

	// Observability stack availability — let agents know what they can reference.
	b.WriteString("=== OBSERVABILITY STACK ===\n")
	if c.PrometheusEnabled && c.PrometheusURL != "" {
		b.WriteString(fmt.Sprintf("Prometheus: AVAILABLE at %s (use PromQL for CPU/memory/error-rate queries)\n", c.PrometheusURL))
	} else {
		b.WriteString("Prometheus: NOT configured — use kubelet/agent metrics only\n")
	}
	if c.LokiEnabled && c.LokiURL != "" {
		b.WriteString(fmt.Sprintf("Loki: AVAILABLE at %s (use LogQL for log queries)\n", c.LokiURL))
	} else {
		b.WriteString("Loki: NOT configured — logs provided inline below\n")
	}
	b.WriteString("\n")

	// Statistical anomalies detected on primary metric.
	if anomSummary := AnomalySummary(c.Anomalies); anomSummary != "" {
		b.WriteString(anomSummary)
	}

	b.WriteString("=== ALERT ===\n")
	b.WriteString(fmt.Sprintf("Rule: %s | Severity: %s | Status: %s\n", c.Alert.RuleName, c.Alert.Severity, c.Alert.Status))
	b.WriteString(fmt.Sprintf("Resource: %s (namespace: %s)\n", c.Alert.ResourceID, c.Alert.Namespace))
	b.WriteString(fmt.Sprintf("Value: %.4f | Threshold: %.4f\n", c.Alert.Value, c.Alert.Threshold))
	b.WriteString(fmt.Sprintf("Fired at: %s\n", c.Alert.FiredAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Message: %s\n\n", c.Alert.Message))

	if len(c.RecentMetrics) > 0 {
		b.WriteString("=== RECENT METRIC SAMPLES (last 30m) ===\n")
		for _, m := range c.RecentMetrics {
			b.WriteString(fmt.Sprintf("  %s  %s=%.4f %s\n",
				m.Timestamp.Format("15:04:05"), m.Name, m.Value, m.Unit))
		}
		b.WriteString("\n")
	}

	if len(c.RelatedMetrics) > 0 {
		b.WriteString("=== RELATED METRICS ===\n")
		for _, m := range c.RelatedMetrics {
			b.WriteString(fmt.Sprintf("  %s  %s=%.4f %s\n",
				m.Timestamp.Format("15:04:05"), m.Name, m.Value, m.Unit))
		}
		b.WriteString("\n")
	}

	if len(c.SiblingAlerts) > 0 {
		b.WriteString("=== OTHER ACTIVE ALERTS ON SAME RESOURCE ===\n")
		for _, a := range c.SiblingAlerts {
			b.WriteString(fmt.Sprintf("  %s (%s) value=%.4f\n", a.RuleName, a.Severity, a.Value))
		}
		b.WriteString("\n")
	}

	if len(c.PodLogs) > 0 {
		b.WriteString(fmt.Sprintf("=== POD LOGS (last %d lines) ===\n", len(c.PodLogs)))
		for _, l := range c.PodLogs {
			b.WriteString(fmt.Sprintf("  [%s] %s\n", l.Timestamp.Format("15:04:05"), l.Line))
		}
		b.WriteString("\n")
	}

	if len(c.PrevLogs) > 0 {
		b.WriteString(fmt.Sprintf("=== PREVIOUS CONTAINER LOGS (pre-crash — %d lines) ===\n", len(c.PrevLogs)))
		for _, l := range c.PrevLogs {
			b.WriteString(fmt.Sprintf("  [PREV][%s] %s\n", l.Timestamp.Format("15:04:05"), l.Line))
		}
		b.WriteString("\n")
	}

	// Pre-classified log patterns — deterministic signal, placed after raw logs.
	if c.LogClassification.Summary != "" {
		b.WriteString(c.LogClassification.Summary)
	}

	return b.String()
}

// relatedMetrics returns metric names that are useful context for a given rule.
func relatedMetrics(ruleName string) []string {
	mapping := map[string][]string{
		"cpu_spike_critical":  {"memory.usage_percent", "load.load1", "load.load5"},
		"cpu_spike_warning":   {"memory.usage_percent", "load.load1"},
		"memory_spike_critical": {"cpu.usage", "disk.usage_percent"},
		"memory_spike_warning":  {"cpu.usage"},
		"disk_full_critical":    {"memory.usage_percent"},
		"k8s_pod_crash_loop":    {"kubernetes.pod.cpu.usage", "kubernetes.pod.memory.usage"},
		"k8s_node_cpu_high":     {"kubernetes.node.memory.usage"},
	}
	if related, ok := mapping[ruleName]; ok {
		return related
	}
	return nil
}

func looksLikePod(resourceID string) bool {
	// Pod names typically contain at least two "-" segments (deployment-replicaset-pod).
	return strings.Count(resourceID, "-") >= 2
}
