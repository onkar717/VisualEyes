// Package metrics exposes a dedicated Prometheus registry and all
// application-level metrics so every package can import and update them
// without importing the full handler or storage stack.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry is the application-wide Prometheus registry.
// We use a dedicated registry (not the default) to avoid polluting the
// global namespace and to make testing deterministic.
var Registry = prometheus.NewRegistry()

// Metric variables registered with Registry.
var (
	UptimeSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "visual_eyes",
		Name:      "uptime_seconds",
		Help:      "Seconds since the server started.",
	})

	// ActiveAlerts is a live gauge split by severity ("critical", "warning", "info").
	ActiveAlerts = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "visual_eyes",
		Name:      "active_alerts",
		Help:      "Number of currently firing alerts, labelled by severity.",
	}, []string{"severity"})

	// MetricsIngested counts data-points accepted by source ("system", "kubernetes").
	MetricsIngested = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "visual_eyes",
		Name:      "metrics_ingested_total",
		Help:      "Total metric data-points ingested, labelled by source.",
	}, []string{"source"})

	// RCARuns counts completed RCA pipeline runs by final status.
	RCARuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "visual_eyes",
		Name:      "rca_runs_total",
		Help:      "Total RCA pipeline runs, labelled by status (done|failed).",
	}, []string{"status"})

	// WSClients is a live gauge of connected WebSocket clients.
	WSClients = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "visual_eyes",
		Name:      "ws_connected_clients",
		Help:      "Number of WebSocket clients currently connected.",
	})

	// HTTPRequests counts all handled requests by method, condensed path, and
	// HTTP status code.
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "visual_eyes",
		Name:      "http_requests_total",
		Help:      "Total HTTP requests handled, labelled by method, path, and code.",
	}, []string{"method", "path", "code"})

	// LastMetricValue stores the most recently ingested value per named metric
	// so Prometheus scrapers can alert on individual metric values without
	// querying the storage backend.
	LastMetricValue = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "visual_eyes",
		Name:      "last_metric_value",
		Help:      "Most recently ingested value for each named metric.",
	}, []string{"name", "source"})
)

func init() {
	Registry.MustRegister(
		// Standard Go runtime and process metrics.
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),

		// Application metrics.
		UptimeSeconds,
		ActiveAlerts,
		MetricsIngested,
		RCARuns,
		WSClients,
		HTTPRequests,
		LastMetricValue,
	)
}
