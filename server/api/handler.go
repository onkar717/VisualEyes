package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	sharedhttp "github.com/onkar717/visual-eyes/server/http"
	appmetrics "github.com/onkar717/visual-eyes/server/metrics"
	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/rca"
	"github.com/onkar717/visual-eyes/server/storage"
	"github.com/onkar717/visual-eyes/server/ws"
)

// Handler is the central HTTP handler for all VisualEyes API endpoints.
type Handler struct {
	systemStore       storage.MetricStore
	kubernetesStore   storage.MetricStore
	alertStore        storage.AlertStore
	logStore          storage.LogStore
	rcaStore            storage.RCAStore
	notificationStore   storage.NotificationStore
	incidentStore       storage.IncidentStore
	remediationLogStore storage.RemediationLogStore
	clusterStore        storage.ClusterStore
	snapshotStore       storage.ClusterSnapshotStore
	eventBuffer         *storage.EventBuffer // ring buffer for K8s Warning events
	broadcaster         *ws.Broadcaster
	rcaTrigger          chan<- models.Alert  // feed to RCA worker pool for on-demand scans
	hostname          string
	corsOrigins       map[string]bool
	startedAt         time.Time
	rateLimiter       interface{ Stop() }
}

func (h *Handler) SetAlertStore(s storage.AlertStore)               { h.alertStore = s }
func (h *Handler) SetLogStore(s storage.LogStore)                   { h.logStore = s }
func (h *Handler) SetEventBuffer(eb *storage.EventBuffer)           { h.eventBuffer = eb }
func (h *Handler) SetRCAStore(s storage.RCAStore)                   { h.rcaStore = s }
func (h *Handler) SetNotificationStore(s storage.NotificationStore) { h.notificationStore = s }
func (h *Handler) SetIncidentStore(s storage.IncidentStore)                 { h.incidentStore = s }
func (h *Handler) SetRemediationLogStore(s storage.RemediationLogStore) { h.remediationLogStore = s }
func (h *Handler) SetClusterStore(s storage.ClusterStore)               { h.clusterStore = s }
func (h *Handler) SetSnapshotStore(s storage.ClusterSnapshotStore)      { h.snapshotStore = s }
func (h *Handler) SetBroadcaster(b *ws.Broadcaster)                     { h.broadcaster = b }
func (h *Handler) SetRCATrigger(ch chan<- models.Alert)                  { h.rcaTrigger = ch }

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

	allowed := make(map[string]bool, len(corsOrigins))
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:3000", "http://localhost:5173"}
	}
	for _, o := range corsOrigins {
		allowed[o] = true
	}

	return &Handler{
		systemStore:     systemStore,
		kubernetesStore: kubernetesStore,
		hostname:        hostname,
		corsOrigins:     allowed,
		startedAt:       time.Now(),
	}, nil
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func roundValue(value float64) float64 {
	return float64(int64(value*100)) / 100
}

func (h *Handler) cors(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if h.corsOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, X-Request-ID")
	w.Header().Set("Vary", "Origin")
}

func (h *Handler) preflight(w http.ResponseWriter, r *http.Request) bool {
	h.cors(w, r)
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

	// Update Prometheus counters and last-value gauges.
	appmetrics.MetricsIngested.WithLabelValues(metricType).Add(float64(len(metrics)))
	for _, m := range metrics {
		appmetrics.LastMetricValue.WithLabelValues(m.Name, metricType).Set(m.Value)
	}

	// Broadcast a fresh snapshot to any connected WebSocket clients.
	if h.broadcaster != nil && h.broadcaster.Len() > 0 {
		go h.broadcastSnapshot()
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
// Pod logs
// -------------------------------------------------------------------

// HandlePodLogs dispatches POST (agent ingestion) and GET (UI query) for /api/pod-logs.
func (h *Handler) HandlePodLogs(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.postPodLogs(w, r)
	case http.MethodGet:
		h.getPodLogs(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) postPodLogs(w http.ResponseWriter, r *http.Request) {
	if h.logStore == nil {
		w.WriteHeader(http.StatusAccepted) // accept but discard until store is ready
		return
	}
	var logs []models.PodLog
	if err := json.NewDecoder(r.Body).Decode(&logs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.logStore.StoreLogs(logs); err != nil {
		slog.Error("failed to store pod logs", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store logs")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) getPodLogs(w http.ResponseWriter, r *http.Request) {
	if h.logStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"logs": []any{}})
		return
	}
	pod := r.URL.Query().Get("pod")
	ns := r.URL.Query().Get("namespace") // empty = all namespaces
	limit := 500
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)

	logs, err := h.logStore.GetLogs(pod, ns, limit)
	if err != nil {
		slog.Error("failed to get pod logs", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get logs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "count": len(logs)})
}

// -------------------------------------------------------------------
// Kubernetes events   stub, fully implemented in Commit 4
// -------------------------------------------------------------------

// HandleKubeEvents accepts K8s event batches (POST) and serves them (GET).
// Full event store is added in a future iteration; for now we accept + log.
func (h *Handler) HandleKubeEvents(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var payload []storage.K8sEvent
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if h.eventBuffer != nil && len(payload) > 0 {
			h.eventBuffer.Store(payload)
		}
		slog.Debug("received k8s events", "count", len(payload))
		w.WriteHeader(http.StatusAccepted)
	case http.MethodGet:
		var events []storage.K8sEvent
		if h.eventBuffer != nil {
			events = h.eventBuffer.GetRecent("", 100)
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// WebSocketStream upgrades to a WebSocket connection and streams metric
// snapshots in real-time. Each message is a JSON object identical to the
// /api/metrics/snapshot response so the UI can share a single decoder.
func (h *Handler) WebSocketStream(w http.ResponseWriter, r *http.Request) {
	if h.broadcaster == nil {
		http.Error(w, "broadcaster not initialised", http.StatusServiceUnavailable)
		return
	}
	appmetrics.WSClients.Inc()
	defer appmetrics.WSClients.Dec()
	h.broadcaster.ServeClient(w, r)
}

// broadcastSnapshot reads the current metric snapshot and fans it out to all
// connected WebSocket clients. It is called in a goroutine after every
// successful metric ingestion.
func (h *Handler) broadcastSnapshot() {
	all := h.systemStore.GetAllMetrics()
	grouped := map[string]map[string]any{
		"cpu": {}, "memory": {}, "disk": {}, "network": {}, "load": {},
	}
	for _, m := range all {
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
	payload, err := json.Marshal(map[string]any{
		"type":      "metrics_snapshot",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"metrics":   grouped,
	})
	if err != nil {
		return
	}
	h.broadcaster.Send(payload)
}

// -------------------------------------------------------------------
// Alerts
// -------------------------------------------------------------------

// HandleAlerts serves GET /api/alerts?status=firing|all&limit=N.
func (h *Handler) HandleAlerts(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.alertStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"alerts": []any{}})
		return
	}

	statusFilter := r.URL.Query().Get("status")
	var (
		alertList []models.Alert
		err       error
	)
	if statusFilter == "firing" || statusFilter == "" {
		alertList, err = h.alertStore.GetActiveAlerts()
	} else {
		limit := 100
		fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
		alertList, err = h.alertStore.GetAlertHistory(limit)
	}
	if err != nil {
		slog.Error("failed to fetch alerts", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch alerts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": alertList, "count": len(alertList)})
}

// HandleAlertByID serves GET /api/alerts/{id}.
func (h *Handler) HandleAlertByID(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.alertStore == nil {
		writeError(w, http.StatusServiceUnavailable, "alert store not initialised")
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/alerts/")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert id")
		return
	}

	alert, err := h.alertStore.GetAlertByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "alert not found")
		return
	}
	writeJSON(w, http.StatusOK, alert)
}

// -------------------------------------------------------------------
// RCA
// -------------------------------------------------------------------

// HandleRCA dispatches:
//   GET  /api/rca/{alertID}            fetch RCA result
//   POST /api/rca/{alertID}/execute    manually execute a specific command
func (h *Handler) HandleRCA(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if h.rcaStore == nil {
		writeError(w, http.StatusServiceUnavailable, "rca store not initialised")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/rca/")

	if strings.HasSuffix(path, "/execute") && r.Method == http.MethodPost {
		h.executeRCACommand(w, r, strings.TrimSuffix(path, "/execute"))
		return
	}

	if strings.HasSuffix(path, "/progress") && r.Method == http.MethodGet {
		h.streamRCAProgress(w, r, strings.TrimSuffix(path, "/progress"))
		return
	}

	if r.Method == http.MethodGet {
		h.getRCAResult(w, r, path)
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// streamRCAProgress streams Server-Sent Events with live stage progress for an RCA run.
func (h *Handler) streamRCAProgress(w http.ResponseWriter, r *http.Request, alertIDStr string) {
	var alertID uint
	if _, err := fmt.Sscanf(alertIDStr, "%d", &alertID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert id")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	h.cors(w, r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Replay history so late subscribers see completed stages immediately.
	history := rca.StageHistory(alertID)
	for _, ev := range history {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	flusher.Flush()

	// If RCA is already fully done/failed (all 6 stages replayed), close immediately.
	if rca.IsDone(alertID) {
		return
	}

	// Also close immediately if alert status shows RCA will never run.
	if h.alertStore != nil {
		if a, err := h.alertStore.GetAlertByID(alertID); err == nil {
			if a.RCAStatus == "done" || a.RCAStatus == "failed" {
				return
			}
		}
	}

	// Subscribe to live events.
	ch, cancel := rca.SubscribeStage(alertID)
	defer cancel()

	// idleTimer fires if no stages arrive for 10s (RCA disabled or not started).
	idleTimer := time.NewTimer(10 * time.Second)
	defer idleTimer.Stop()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			idleTimer.Reset(10 * time.Second)
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
			if rca.IsDone(alertID) {
				return
			}
		case <-idleTimer.C:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (h *Handler) getRCAResult(w http.ResponseWriter, r *http.Request, alertIDStr string) {
	var alertID uint
	if _, err := fmt.Sscanf(alertIDStr, "%d", &alertID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert id")
		return
	}
	result, err := h.rcaStore.GetRCAResult(alertID)
	if err != nil {
		writeError(w, http.StatusNotFound, "rca result not found")
		return
	}

	// Parse commands JSON for the response.
	var commands []models.FixCommand
	if result.Commands != "" {
		if err := json.Unmarshal([]byte(result.Commands), &commands); err != nil {
			slog.Warn("getRCAResult: failed to parse commands JSON", "alert_id", alertID, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          result.ID,
		"alertID":     result.AlertID,
		"explanation": result.Explanation,
		"rootCause":   result.RootCause,
		"commands":    commands,
		"status":      result.Status,
		"model":       result.Model,
		"inputTokens": result.InputTokens,
		"createdAt":   result.CreatedAt,
	})
}

func (h *Handler) executeRCACommand(w http.ResponseWriter, r *http.Request, alertIDStr string) {
	var alertID uint
	if _, err := fmt.Sscanf(alertIDStr, "%d", &alertID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert id")
		return
	}

	var req struct {
		CommandIndex int `json:"commandIndex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.rcaStore.GetRCAResult(alertID)
	if err != nil {
		writeError(w, http.StatusNotFound, "rca result not found")
		return
	}

	var commands []models.FixCommand
	if err := json.Unmarshal([]byte(result.Commands), &commands); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse commands")
		return
	}

	if req.CommandIndex < 0 || req.CommandIndex >= len(commands) {
		writeError(w, http.StatusBadRequest, "command index out of range")
		return
	}

	cmd := &commands[req.CommandIndex]
	if cmd.Status == models.RemediationExecuted {
		writeJSON(w, http.StatusOK, map[string]any{"status": "already_executed", "output": cmd.Output})
		return
	}

	// Manual execution (may not be in auto-safe list   let it through with warning).
	slog.Info("manual rca command execution requested", "command", cmd.Command, "alert_id", alertID)
	output, execErr := runCommand(cmd.Command)
	if execErr != nil {
		cmd.Status = models.RemediationFailed
		cmd.ExecError = execErr.Error()
	} else {
		cmd.Status = models.RemediationExecuted
		cmd.Output = output
	}

	// Persist updated command status.
	updated, err := json.Marshal(commands)
	if err != nil {
		slog.Error("executeRCACommand: failed to marshal commands", "alert_id", alertID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to persist command status")
		return
	}
	result.Commands = string(updated)
	result.UpdatedAt = time.Now()
	if err := h.rcaStore.UpdateRCAResult(result); err != nil {
		slog.Error("executeRCACommand: failed to update rca result", "alert_id", alertID, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": string(cmd.Status),
		"output": cmd.Output,
		"error":  cmd.ExecError,
	})
}

// runCommand executes a shell command and returns combined output.
// This is the manual-approval path; the auto-safe path uses rca.Executor.
func runCommand(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, parts[0], parts[1:]...)
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// -------------------------------------------------------------------
// Observability endpoints
// -------------------------------------------------------------------

// Ping is a liveness probe   always returns 200.
func (h *Handler) Ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "pong")
}

// Healthz returns component health + cluster health score as JSON.
// HTTP 503 if any core component is unhealthy.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
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
		"status":       status,
		"uptime":       uptime,
		"health_score": h.computeHealthScore(),
		"components": map[string]bool{
			"system_store":    systemOK,
			"k8s_store":       k8sOK,
			"alert_store":     h.alertStore != nil,
			"incident_store":  h.incidentStore != nil,
		},
	})
}

// computeHealthScore returns a 0-100 score representing cluster health.
// 100 = perfect, 0 = critical. Deductions based on active alerts and metrics.
func (h *Handler) computeHealthScore() float64 {
	score := 100.0

	// Active alerts penalty.
	if h.alertStore != nil {
		if active, err := h.alertStore.GetActiveAlerts(); err == nil {
			for _, a := range active {
				switch a.Severity {
				case "critical":
					score -= 15
				case "warning":
					score -= 7
				default:
					score -= 2
				}
			}
		}
	}

	// Metric thresholds penalty.
	if h.systemStore != nil {
		for _, m := range h.systemStore.GetAllMetrics() {
			switch m.Name {
			case "cpu.usage":
				if m.Value >= 90 {
					score -= 15
				} else if m.Value >= 75 {
					score -= 8
				}
			case "memory.usage_percent":
				if m.Value >= 90 {
					score -= 15
				} else if m.Value >= 80 {
					score -= 8
				}
			case "disk.usage_percent":
				if m.Value >= 95 {
					score -= 20
				} else if m.Value >= 85 {
					score -= 10
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// PrometheusMetrics exposes all registered application metrics in Prometheus
// text exposition format (scrape endpoint for Prometheus / Grafana / Victoria).
func (h *Handler) PrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	// Refresh gauges that must be computed on-demand.
	appmetrics.UptimeSeconds.Set(time.Since(h.startedAt).Seconds())

	if h.alertStore != nil {
		h.refreshAlertGauges()
	}

	promhttp.HandlerFor(appmetrics.Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}).ServeHTTP(w, r)
}

// refreshAlertGauges re-queries active alerts and updates the Prometheus gauges.
func (h *Handler) refreshAlertGauges() {
	active, err := h.alertStore.GetActiveAlerts()
	if err != nil {
		return
	}
	counts := map[string]float64{"critical": 0, "warning": 0, "info": 0}
	for _, a := range active {
		if _, ok := counts[string(a.Severity)]; ok {
			counts[string(a.Severity)]++
		}
	}
	for sev, n := range counts {
		appmetrics.ActiveAlerts.WithLabelValues(sev).Set(n)
	}
}

// -------------------------------------------------------------------
// Cluster health scan
// -------------------------------------------------------------------

// ScanIssue is a single finding from a cluster scan.
type ScanIssue struct {
	Severity string `json:"severity"` // critical | warning | info
	Category string `json:"category"` // cpu | memory | disk | alerts | k8s
	Resource string `json:"resource"`
	Message  string `json:"message"`
	Value    string `json:"value,omitempty"`
	AlertID  uint   `json:"alertID,omitempty"` // set when finding is derived from an alert
}

// ScanResult is the full /api/scan response.
type ScanResult struct {
	Timestamp   string      `json:"timestamp"`
	Overall     string      `json:"overall"` // healthy | degraded | critical
	IssueCount  int         `json:"issueCount"`
	Issues      []ScanIssue `json:"issues"`
	Summary     ScanSummary `json:"summary"`
}

// ScanSummary provides high-level metrics for the scan output.
type ScanSummary struct {
	ActiveAlerts    int     `json:"activeAlerts"`
	CriticalAlerts  int     `json:"criticalAlerts"`
	WarningAlerts   int     `json:"warningAlerts"`
	CPUPercent      float64 `json:"cpuPercent"`
	MemoryPercent   float64 `json:"memoryPercent"`
	DiskPercent     float64 `json:"diskPercent"`
}

// HandleScan performs a point-in-time health assessment using stored metric
// and alert data, and returns a structured list of findings.
func (h *Handler) HandleScan(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result := ScanResult{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Issues:    []ScanIssue{},
	}

	// Active alerts
	if h.alertStore != nil {
		active, err := h.alertStore.GetActiveAlerts()
		if err == nil {
			result.Summary.ActiveAlerts = len(active)
			for _, a := range active {
				sev := string(a.Severity)
				switch a.Severity {
				case "critical":
					result.Summary.CriticalAlerts++
				case "warning":
					result.Summary.WarningAlerts++
				}
				result.Issues = append(result.Issues, ScanIssue{
					Severity: sev,
					Category: "alerts",
					Resource: a.ResourceID + "/" + a.Namespace,
					Message:  a.Message,
					Value:    fmt.Sprintf("%.2f (threshold %.2f)", a.Value, a.Threshold),
					AlertID:  a.ID,
				})
			}
		}
	}

	// System metrics
	metrics := h.systemStore.GetAllMetrics()
	for _, m := range metrics {
		switch m.Name {
		case "cpu.usage":
			result.Summary.CPUPercent = m.Value
			if m.Value >= 90 {
				result.Issues = append(result.Issues, ScanIssue{
					Severity: "critical", Category: "cpu",
					Resource: "host", Message: "CPU usage critical",
					Value: fmt.Sprintf("%.1f%%", m.Value),
				})
			} else if m.Value >= 75 {
				result.Issues = append(result.Issues, ScanIssue{
					Severity: "warning", Category: "cpu",
					Resource: "host", Message: "CPU usage elevated",
					Value: fmt.Sprintf("%.1f%%", m.Value),
				})
			}
		case "memory.usage_percent":
			result.Summary.MemoryPercent = m.Value
			if m.Value >= 90 {
				result.Issues = append(result.Issues, ScanIssue{
					Severity: "critical", Category: "memory",
					Resource: "host", Message: "Memory usage critical",
					Value: fmt.Sprintf("%.1f%%", m.Value),
				})
			} else if m.Value >= 80 {
				result.Issues = append(result.Issues, ScanIssue{
					Severity: "warning", Category: "memory",
					Resource: "host", Message: "Memory usage elevated",
					Value: fmt.Sprintf("%.1f%%", m.Value),
				})
			}
		case "disk.usage_percent":
			result.Summary.DiskPercent = m.Value
			if m.Value >= 95 {
				result.Issues = append(result.Issues, ScanIssue{
					Severity: "critical", Category: "disk",
					Resource: "host", Message: "Disk usage critical   eviction risk",
					Value: fmt.Sprintf("%.1f%%", m.Value),
				})
			} else if m.Value >= 85 {
				result.Issues = append(result.Issues, ScanIssue{
					Severity: "warning", Category: "disk",
					Resource: "host", Message: "Disk usage high",
					Value: fmt.Sprintf("%.1f%%", m.Value),
				})
			}
		}
	}

	// K8s metrics
	k8sMetrics := h.kubernetesStore.GetAllMetrics()
	for _, m := range k8sMetrics {
		if m.Name == "kubernetes.pod.cpu.usage" && m.Value > 0.9 {
			pod := m.Tags["pod"]
			if pod == "" {
				pod = "unknown"
			}
			result.Issues = append(result.Issues, ScanIssue{
				Severity: "warning", Category: "k8s",
				Resource: pod, Message: "Pod CPU usage near limit",
				Value: fmt.Sprintf("%.3f cores", m.Value),
			})
		}
	}

	// Overall status
	result.IssueCount = len(result.Issues)
	result.Overall = "healthy"
	for _, issue := range result.Issues {
		if issue.Severity == "critical" {
			result.Overall = "critical"
			break
		}
		if issue.Severity == "warning" {
			result.Overall = "degraded"
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleScanAll triggers on-demand AI RCA for every firing alert that has not
// yet completed analysis. POST /api/rca/scan-all
//
// Query param: dry_run=true lists which alerts WOULD be triggered without
// actually queueing them. Returns HTTP 200 with dry_run=true in body.
func (h *Handler) HandleScanAll(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if h.alertStore == nil || h.rcaTrigger == nil {
		writeError(w, http.StatusServiceUnavailable, "rca engine not enabled or not wired")
		return
	}

	dryRun := r.URL.Query().Get("dry_run") == "true"

	active, err := h.alertStore.GetActiveAlerts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "fetch active alerts: "+err.Error())
		return
	}

	type candidateItem struct {
		ID       uint   `json:"id"`
		Message  string `json:"message"`
		Severity string `json:"severity"`
		Resource string `json:"resource"`
	}

	var candidates []candidateItem
	skipped := 0

	for _, a := range active {
		if a.RCAStatus == "running" || a.RCAStatus == "done" {
			skipped++
			continue
		}
		candidates = append(candidates, candidateItem{
			ID:       a.ID,
			Message:  a.Message,
			Severity: string(a.Severity),
			Resource: a.ResourceID,
		})
	}

	if candidates == nil {
		candidates = []candidateItem{}
	}

	if dryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"dry_run":            true,
			"would_trigger":      len(candidates),
			"already_processing": skipped,
			"alerts":             candidates,
		})
		return
	}

	// Actually queue eligible alerts into the RCA worker pool.
	var queued []candidateItem
	for _, a := range active {
		if a.RCAStatus == "running" || a.RCAStatus == "done" {
			continue
		}
		select {
		case h.rcaTrigger <- a:
			queued = append(queued, candidateItem{
				ID:       a.ID,
				Message:  a.Message,
				Severity: string(a.Severity),
				Resource: a.ResourceID,
			})
		default:
			// Worker queue full count as already processing.
			skipped++
		}
	}

	if queued == nil {
		queued = []candidateItem{}
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"triggered":          len(queued),
		"already_processing": skipped,
		"alerts":             queued,
	})
}

// HandleIncidents returns recent notification delivery events.
// GET /api/incidents?limit=50&alert_id=<id>
func (h *Handler) HandleIncidents(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.notificationStore == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	q := r.URL.Query()

	// Optional filter by alert ID.
	if alertIDStr := q.Get("alert_id"); alertIDStr != "" {
		var alertID uint
		if _, err := fmt.Sscanf(alertIDStr, "%d", &alertID); err != nil {
			http.Error(w, "invalid alert_id", http.StatusBadRequest)
			return
		}
		events, err := h.notificationStore.GetNotificationEvents(alertID)
		if err != nil {
			slog.Error("get notification events", "alert_id", alertID, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, events)
		return
	}

	limit := 50
	if l := q.Get("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil || limit <= 0 {
			limit = 50
		}
	}
	if limit > 500 {
		limit = 500
	}

	events, err := h.notificationStore.GetRecentNotificationEvents(limit)
	if err != nil {
		slog.Error("get recent notification events", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// HandleIncidentsFull returns structured incidents with SEV/status/MTTR.
// GET /api/incidents/full?limit=50&severity=SEV1&status=OPEN
func (h *Handler) HandleIncidentsFull(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.incidentStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"incidents": []struct{}{}, "mttr_avg_seconds": 0, "count": 0})
		return
	}

	q := r.URL.Query()
	limit := 50
	if l := q.Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if limit > 500 {
		limit = 500
	}

	hours := 0
	if h := q.Get("hours"); h != "" {
		fmt.Sscanf(h, "%d", &hours)
	}

	incidents, err := h.incidentStore.GetRecentIncidents(q.Get("severity"), q.Get("status"), limit, hours)
	if err != nil {
		slog.Error("get incidents", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	avg, count, _ := h.incidentStore.MTTRStats()
	mttrBySev, _ := h.incidentStore.MTTRStatsBySeverity()
	writeJSON(w, http.StatusOK, map[string]any{
		"incidents":        incidents,
		"count":            len(incidents),
		"mttr_avg_seconds": avg,
		"mttr_count":       count,
		"mttr_by_severity": mttrBySev,
	})
}

// HandleClusters lists all registered clusters.
// GET /api/clusters
func (h *Handler) HandleClusters(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if h.clusterStore == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}
	clusters, err := h.clusterStore.ListClusters()
	if err != nil {
		slog.Error("list clusters", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if clusters == nil {
		clusters = []models.ClusterHealth{}
	}
	writeJSON(w, http.StatusOK, clusters)
}

// HandleClusterDetail returns one cluster and optionally its incidents.
// GET /api/clusters/{name}
// GET /api/clusters/{name}/incidents
func (h *Handler) HandleClusterDetail(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/clusters/")
	parts := strings.SplitN(path, "/", 2)
	name := parts[0]

	if len(parts) == 2 && parts[1] == "incidents" && h.incidentStore != nil {
		incidents, err := h.incidentStore.GetRecentIncidents("", "", 100, 0)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, incidents)
		return
	}

	if h.clusterStore == nil {
		http.Error(w, "cluster store not available", http.StatusServiceUnavailable)
		return
	}
	c, err := h.clusterStore.GetCluster(name)
	if err != nil {
		http.Error(w, "cluster not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// HandleClusterHeartbeat accepts a health snapshot from a cluster agent.
// POST /api/clusters/heartbeat
func (h *Handler) HandleClusterHeartbeat(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.clusterStore == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var c models.ClusterHealth
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if c.Name == "" {
		http.Error(w, "cluster name required", http.StatusBadRequest)
		return
	}
	c.LastSeen = time.Now()

	// Attach open incident count if incidentStore is available.
	if h.incidentStore != nil {
		open, _ := h.incidentStore.GetRecentIncidents("", "OPEN", 1000, 0)
		c.OpenIncidents = len(open)
	}

	if err := h.clusterStore.UpsertCluster(&c); err != nil {
		slog.Error("upsert cluster", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Persist immutable time-series snapshot for trending.
	if h.snapshotStore != nil {
		snap := &models.ClusterSnapshot{
			ClusterName:   c.Name,
			RecordedAt:    c.LastSeen,
			TotalNodes:    c.TotalNodes,
			ReadyNodes:    c.ReadyNodes,
			TotalPods:     c.TotalPods,
			RunningPods:   c.RunningPods,
			PendingPods:   c.PendingPods,
			FailedPods:    c.FailedPods,
			CrashloopPods: c.CrashloopPods,
			OpenIncidents: c.OpenIncidents,
			CPUUsagePct:   c.CPUUsagePct,
			MemUsagePct:   c.MemUsagePct,
		}
		// Recompute health score using all factors including CPU/Mem pressure.
		snap.ComputeHealthScore()
		c.HealthScore = snap.HealthScore
		if err := h.snapshotStore.SaveSnapshot(snap); err != nil {
			slog.Warn("save cluster snapshot failed", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "cluster": c.Name})
}

// HandleSnapshots returns point-in-time health trend data for a cluster.
// GET /api/snapshots?cluster=NAME&hours=24&limit=288
func (h *Handler) HandleSnapshots(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if h.snapshotStore == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	cluster := r.URL.Query().Get("cluster")
	if cluster == "" {
		http.Error(w, "cluster param required", http.StatusBadRequest)
		return
	}
	hours := 24
	limit := 288
	fmt.Sscanf(r.URL.Query().Get("hours"), "%d", &hours)
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)

	snaps, err := h.snapshotStore.GetSnapshots(cluster, hours, limit)
	if err != nil {
		slog.Error("get snapshots", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if snaps == nil {
		snaps = []models.ClusterSnapshot{}
	}
	writeJSON(w, http.StatusOK, snaps)
}

// HandleRemediationLog handles GET and POST for remediation step audit log.
// GET  /api/remediation-log?incident_id=N    fetch log for one incident
// GET  /api/remediation-log?limit=N          fetch recent entries (default 50)
// POST /api/remediation-log                  record a step execution
func (h *Handler) HandleRemediationLog(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if h.remediationLogStore == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		if idStr := q.Get("incident_id"); idStr != "" {
			var id uint
			fmt.Sscanf(idStr, "%d", &id)
			logs, err := h.remediationLogStore.GetRemediationLogs(id)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if logs == nil {
				logs = []models.RemediationLogEntry{}
			}
			writeJSON(w, http.StatusOK, logs)
			return
		}
		limit := 50
		if l := q.Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}
		logs, err := h.remediationLogStore.GetRecentRemediationLogs(limit)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if logs == nil {
			logs = []models.RemediationLogEntry{}
		}
		writeJSON(w, http.StatusOK, logs)

	case http.MethodPost:
		var entry models.RemediationLogEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if entry.ExecutedAt.IsZero() {
			entry.ExecutedAt = time.Now()
		}
		if err := h.remediationLogStore.SaveRemediationLog(&entry); err != nil {
			slog.Error("save remediation log", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, entry)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleStats returns aggregate incident statistics.
// GET /api/stats
func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if h.incidentStore == nil {
		writeJSON(w, http.StatusOK, storage.IncidentStats{
			BySeverity: map[string]int{},
			ByStatus:   map[string]int{},
		})
		return
	}
	stats, err := h.incidentStore.GetStats()
	if err != nil {
		slog.Error("get stats", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// HandleIncidentStatus updates an incident's status (OPEN→INVESTIGATING→MITIGATED→RESOLVED).
// PATCH /api/incidents/full/<id>
func (h *Handler) HandleIncidentStatus(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	if h.preflight(w, r) {
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.incidentStore == nil {
		http.Error(w, "incident store not available", http.StatusServiceUnavailable)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/incidents/full/")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Error(w, "invalid incident id", http.StatusBadRequest)
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	inc, err := h.incidentStore.GetIncidentByID(id)
	if err != nil {
		http.Error(w, "incident not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	switch body.Status {
	case "MITIGATED":
		inc.Status = "MITIGATED"
		inc.MitigatedAt = &now
		inc.ComputeMTTR()
	case "RESOLVED":
		inc.Status = "RESOLVED"
		inc.ResolvedAt = &now
		if inc.MitigatedAt == nil {
			inc.MitigatedAt = &now
		}
		inc.ComputeMTTR()
	case "INVESTIGATING":
		inc.Status = "INVESTIGATING"
	case "OPEN":
		inc.Status = "OPEN"
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	if err := h.incidentStore.UpdateIncident(inc); err != nil {
		slog.Error("update incident status", "id", id, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, inc)
}

// HandleIncidentByIDOrStatus dispatches GET → HandleGetIncident, PATCH → HandleIncidentStatus.
func (h *Handler) HandleIncidentByIDOrStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.HandleGetIncident(w, r)
	} else {
		h.HandleIncidentStatus(w, r)
	}
}

// HandleGetIncident returns a single incident by ID.
// GET /api/incidents/full/{id}
func (h *Handler) HandleGetIncident(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	if h.incidentStore == nil {
		writeError(w, http.StatusServiceUnavailable, "incident store not initialised")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/incidents/full/")
	idStr = strings.TrimSuffix(idStr, "/")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid incident id")
		return
	}
	inc, err := h.incidentStore.GetIncidentByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	writeJSON(w, http.StatusOK, inc)
}

// HandleAISREInfo proxies GET /api/ai-sre/info → Python service /config.
// Returns active LLM model, provider, and feature flags. 404 when ai-sre not configured.
func (h *Handler) HandleAISREInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	aiSREURL := os.Getenv("AI_SRE_URL")
	if aiSREURL == "" {
		writeError(w, http.StatusNotFound, "ai-sre service not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aiSREURL+"/config", nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "build request: "+err.Error())
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "ai-sre unavailable")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// HandleInternalStageEvent receives stage-completion callbacks from the Python
// AI-SRE service and publishes them to the Go SSE hub so veye CLI gets live progress.
// This endpoint is internal not exposed on the public API.
// POST /internal/rca/stage-event
func (h *Handler) HandleInternalStageEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var ev struct {
		AlertID uint   `json:"alert_id"`
		Stage   int    `json:"stage"`
		Label   string `json:"label"`
		Status  string `json:"status"`
		Detail  string `json:"detail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if ev.AlertID == 0 || ev.Stage < 1 || ev.Stage > 6 {
		writeError(w, http.StatusBadRequest, "invalid alert_id or stage")
		return
	}

	switch ev.Status {
	case "start":
		rca.PublishStageStart(ev.AlertID, ev.Stage, ev.Label)
	case "done":
		rca.PublishStageDone(ev.AlertID, ev.Stage, ev.Label, ev.Detail)
	case "failed":
		rca.PublishStageFailed(ev.AlertID, ev.Stage, ev.Label)
	default:
		writeError(w, http.StatusBadRequest, "unknown status")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
