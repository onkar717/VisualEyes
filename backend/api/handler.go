package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	sharedhttp "github.com/onkar717/visual-eyes/backend/http"
	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/onkar717/visual-eyes/backend/storage"
)

// Handler is the central HTTP handler for all VisualEyes API endpoints.
// Fields for log/alert/RCA stores are added as stubs here and wired up in
// later commits when those packages are introduced.
type Handler struct {
	systemStore     storage.MetricStore
	kubernetesStore storage.MetricStore
	hostname        string
	corsOrigins     string
	startedAt       time.Time
	rateLimiter     interface{ Stop() } // set by RegisterRoutes; stopped on shutdown
}

// StopRateLimiter cleans up the rate limiter's background cleanup goroutine.
func (h *Handler) StopRateLimiter() {
	if h.rateLimiter != nil {
		h.rateLimiter.Stop()
	}
}

// NewHandler builds a Handler from the provided stores and CORS origin list.
func NewHandler(systemStore, kubernetesStore storage.MetricStore, corsOrigins []string) (*Handler, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("os.Hostname: %w", err)
	}

	origins := strings.Join(corsOrigins, ",")
	if origins == "" {
		origins = "http://localhost:3000,http://localhost:5173"
	}

	return &Handler{
		systemStore:     systemStore,
		kubernetesStore: kubernetesStore,
		hostname:        hostname,
		corsOrigins:     origins,
		startedAt:       time.Now(),
	}, nil
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func roundValue(value float64) float64 {
	return float64(int64(value*100)) / 100
}

func (h *Handler) cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", h.corsOrigins)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func (h *Handler) preflight(w http.ResponseWriter, r *http.Request) bool {
	h.cors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", sharedhttp.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// -------------------------------------------------------------------
// Metric ingestion
// -------------------------------------------------------------------

func (h *Handler) handleMetricsPost(w http.ResponseWriter, r *http.Request, store storage.MetricStore, metricType string) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var metrics []models.Metric
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		slog.Warn("failed to decode metrics body", "type", metricType, "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now()
	for i := range metrics {
		if err := metrics[i].Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if metrics[i].Timestamp.IsZero() {
			metrics[i].Timestamp = now
		}
		if metrics[i].Tags == nil {
			metrics[i].Tags = make(map[string]string)
		}
		metrics[i].Tags["hostname"] = h.hostname
		metrics[i].Tags["source"] = metricType
	}

	if err := store.StoreMetrics(metrics); err != nil {
		slog.Error("failed to store metrics", "type", metricType, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store metrics")
		return
	}

	slog.Debug("stored metrics", "type", metricType, "count", len(metrics))
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) PostSystemMetrics(w http.ResponseWriter, r *http.Request) {
	h.handleMetricsPost(w, r, h.systemStore, "system")
}

func (h *Handler) PostKubernetesMetrics(w http.ResponseWriter, r *http.Request) {
	h.handleMetricsPost(w, r, h.kubernetesStore, "kubernetes")
}

// -------------------------------------------------------------------
// Metric reads
// -------------------------------------------------------------------

func (h *Handler) GetMetricsSnapshot(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metrics := h.systemStore.GetAllMetrics()

	grouped := map[string]map[string]any{
		"cpu": {}, "memory": {}, "disk": {}, "network": {}, "load": {},
	}

	for _, m := range metrics {
		m.Value = roundValue(m.Value)
		var cat, name string
		switch {
		case strings.HasPrefix(m.Name, "cpu."):
			cat, name = "cpu", strings.TrimPrefix(m.Name, "cpu.")
		case strings.HasPrefix(m.Name, "memory."):
			cat, name = "memory", strings.TrimPrefix(m.Name, "memory.")
		case strings.HasPrefix(m.Name, "disk."):
			cat, name = "disk", strings.TrimPrefix(m.Name, "disk.")
		case strings.HasPrefix(m.Name, "network."):
			cat, name = "network", strings.TrimPrefix(m.Name, "network.")
		case strings.HasPrefix(m.Name, "load."):
			cat, name = "load", strings.TrimPrefix(m.Name, "load.")
		default:
			continue
		}
		grouped[cat][name] = map[string]any{
			"value": m.Value, "unit": m.Unit, "tags": m.Tags, "timestamp": m.Timestamp,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"metrics":   grouped,
	})
}

func (h *Handler) GetMetricHistory(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// This endpoint is backed by QueryableStore (added in Commit 2).
	// For now return an empty time series if the store doesn't support history.
	qs, ok := h.systemStore.(storage.QueryableStore)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"samples": []any{}})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name query param required")
		return
	}
	since := time.Now().Add(-30 * time.Minute)
	samples, err := qs.QueryByName(name, since, 200)
	if err != nil {
		slog.Error("failed to query metric history", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"name": name, "samples": samples})
}

func (h *Handler) GetKubernetesMetrics(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metrics := h.kubernetesStore.GetAllMetrics()

	cpuUsage := 0.0
	cpuTotal := 1.0
	memUsage := 0.0
	memTotal := 0.0
	podCPU := 0.0
	podMem := 0.0
	podSet := map[string]bool{}
	nodeSet := map[string]bool{}

	for _, m := range metrics {
		switch m.Name {
		case "kubernetes.node.cpu.usage":
			cpuUsage = m.Value
			if n, ok := m.Tags["node"]; ok {
				nodeSet[n] = true
			}
		case "kubernetes.node.memory.usage":
			memUsage = m.Value
		case "kubernetes.node.memory.available":
			memTotal = memUsage + m.Value
		case "kubernetes.pod.cpu.usage":
			podCPU += m.Value
			if p, ok := m.Tags["pod"]; ok {
				podSet[p] = true
			}
		case "kubernetes.pod.memory.working_set":
			podMem += m.Value
		}
	}
	if cpuTotal == 1 && cpuUsage > 0 {
		cpuTotal = cpuUsage * 2 // rough estimate if no total reported
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"metrics": map[string]any{
			"nodes": map[string]int{"total": len(nodeSet), "ready": len(nodeSet)},
			"pods":  map[string]int{"total": len(podSet), "running": len(podSet)},
			"resources": map[string]any{
				"cpu":    map[string]float64{"usage": cpuUsage, "total": cpuTotal},
				"memory": map[string]float64{"usage": memUsage, "total": memTotal},
			},
			"podResources": map[string]any{
				"cpu":    map[string]float64{"usage": podCPU, "total": cpuTotal},
				"memory": map[string]float64{"usage": podMem, "total": memTotal},
			},
		},
	})
}

// -------------------------------------------------------------------
// Pod logs — stub, fully implemented in Commit 4
// -------------------------------------------------------------------

func (h *Handler) HandlePodLogs(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	// Log store injected via SetLogStore() in Commit 4.
	writeJSON(w, http.StatusOK, map[string]any{"logs": []any{}, "message": "log pipeline coming in commit 4"})
}

// -------------------------------------------------------------------
// Kubernetes events — stub, fully implemented in Commit 4
// -------------------------------------------------------------------

// HandleKubeEvents accepts K8s event batches (POST) and serves them (GET).
func (h *Handler) HandleKubeEvents(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		w.WriteHeader(http.StatusAccepted)
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// WebSocketStream upgrades to a WebSocket connection for real-time metric streaming.
// Full implementation lands in Commit 6.
func (h *Handler) WebSocketStream(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "websocket stream not yet initialised — coming in commit 6", http.StatusNotImplemented)
}

// -------------------------------------------------------------------
// Alerts — stub, fully implemented in Commit 3
// -------------------------------------------------------------------

func (h *Handler) HandleAlerts(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": []any{}, "message": "alert engine coming in commit 3"})
}

func (h *Handler) HandleAlertByID(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	writeError(w, http.StatusNotFound, "not implemented yet")
}

// -------------------------------------------------------------------
// RCA — stub, fully implemented in Commit 5
// -------------------------------------------------------------------

func (h *Handler) HandleRCA(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	writeError(w, http.StatusNotFound, "not implemented yet")
}

// -------------------------------------------------------------------
// Observability endpoints
// -------------------------------------------------------------------

// Ping is a liveness probe — always returns 200.
func (h *Handler) Ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "pong")
}

// Healthz returns component health as JSON. HTTP 503 if any component is unhealthy.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	h.cors(w)
	systemOK := h.systemStore != nil
	k8sOK := h.kubernetesStore != nil
	uptime := time.Since(h.startedAt).Round(time.Second).String()

	status := "healthy"
	code := http.StatusOK
	if !systemOK || !k8sOK {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]any{
		"status": status,
		"uptime": uptime,
		"components": map[string]bool{
			"system_store": systemOK,
			"k8s_store":    k8sOK,
		},
	})
}

// PrometheusMetrics exposes basic counters in the Prometheus text format.
// A full Prometheus registry is added in Commit 6.
func (h *Handler) PrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	uptime := time.Since(h.startedAt).Seconds()
	fmt.Fprintf(w, "# HELP visual_eyes_uptime_seconds Seconds since server start\n")
	fmt.Fprintf(w, "# TYPE visual_eyes_uptime_seconds gauge\n")
	fmt.Fprintf(w, "visual_eyes_uptime_seconds %.2f\n", uptime)
}
