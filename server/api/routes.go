package api

import (
	"net/http"

	"github.com/onkar717/visual-eyes/server/api/middleware"
	"github.com/onkar717/visual-eyes/server/config"
)

// RegisterRoutes wires all HTTP routes onto mux, wrapping with middleware chain.
func RegisterRoutes(mux *http.ServeMux, h *Handler, cfg *config.Config) http.Handler {
	// Observability
	mux.HandleFunc("/ping", h.Ping)
	mux.HandleFunc("/healthz", h.Healthz)
	mux.HandleFunc("/metrics", h.PrometheusMetrics)

	// Metric ingestion (agents → server)
	mux.HandleFunc("/api/system-metrics", h.PostSystemMetrics)
	mux.HandleFunc("/api/kubernetes-metrics", h.PostKubernetesMetrics)

	// Metric query (UI → server)
	mux.HandleFunc("/api/metrics/snapshot", h.GetMetricsSnapshot)
	mux.HandleFunc("/api/metrics/history", h.GetMetricHistory)
	mux.HandleFunc("/api/kubernetes/metrics", h.GetKubernetesMetrics)

	// Logs & K8s events
	mux.HandleFunc("/api/pod-logs", h.HandlePodLogs)
	mux.HandleFunc("/api/events", h.HandleKubeEvents)

	// Alerts
	mux.HandleFunc("/api/alerts", h.HandleAlerts)
	mux.HandleFunc("/api/alerts/", h.HandleAlertByID) // /api/alerts/{id}

	// RCA
	mux.HandleFunc("/api/rca/", h.HandleRCA) // /api/rca/{id} and /api/rca/{id}/execute

	// Cluster health scan
	mux.HandleFunc("/api/scan", h.HandleScan)

	// Notification delivery history
	mux.HandleFunc("/api/incidents", h.HandleIncidents)

	// Incident lifecycle (SEV1-4, MTTR, status, single-incident GET)
	mux.HandleFunc("/api/incidents/full", h.HandleIncidentsFull)
	mux.HandleFunc("/api/incidents/full/", h.HandleIncidentByIDOrStatus) // GET|PATCH /api/incidents/full/{id}

	// Aggregate stats
	mux.HandleFunc("/api/stats", h.HandleStats)

	// Multi-cluster registry
	mux.HandleFunc("/api/clusters", h.HandleClusters)
	mux.HandleFunc("/api/clusters/heartbeat", h.HandleClusterHeartbeat)
	mux.HandleFunc("/api/clusters/", h.HandleClusterDetail)

	// Cluster health trend snapshots
	mux.HandleFunc("/api/snapshots", h.HandleSnapshots)

	// Remediation audit log
	mux.HandleFunc("/api/remediation-log", h.HandleRemediationLog)

	// WebSocket real-time stream
	mux.HandleFunc("/ws", h.WebSocketStream)

	// Build middleware chain (outermost first):
	// recovery → CORS → rate-limit → request-logger → mux
	var chain http.Handler = mux
	chain = middleware.RequestLogger(chain)

	if cfg.Server.RateLimit.Enabled {
		rl := middleware.NewRateLimiter(
			cfg.Server.RateLimit.RequestsPerSecond,
			cfg.Server.RateLimit.Burst,
		)
		h.rateLimiter = rl
		chain = middleware.Limit(rl)(chain)
	}

	chain = middleware.CORS(cfg.Server.CORSOrigins)(chain)
	chain = middleware.Recovery(chain)
	return chain
}
