package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	sharedhttp "github.com/onkar717/visual-eyes/backend/http"
	appmetrics "github.com/onkar717/visual-eyes/backend/metrics"
	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/onkar717/visual-eyes/backend/storage"
	"github.com/onkar717/visual-eyes/backend/ws"
)

// Handler is the central HTTP handler for all VisualEyes API endpoints.
type Handler struct {
	systemStore       storage.MetricStore
	kubernetesStore   storage.MetricStore
	alertStore        storage.AlertStore
	logStore          storage.LogStore
	rcaStore          storage.RCAStore
	notificationStore storage.NotificationStore
	broadcaster       *ws.Broadcaster
	hostname          string
	corsOrigins       string
	startedAt         time.Time
	rateLimiter       interface{ Stop() }
}

func (h *Handler) SetAlertStore(s storage.AlertStore)             { h.alertStore = s }
func (h *Handler) SetLogStore(s storage.LogStore)                 { h.logStore = s }
func (h *Handler) SetRCAStore(s storage.RCAStore)                 { h.rcaStore = s }
func (h *Handler) SetNotificationStore(s storage.NotificationStore) { h.notificationStore = s }
func (h *Handler) SetBroadcaster(b *ws.Broadcaster)               { h.broadcaster = b }

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
// Kubernetes events — stub, fully implemented in Commit 4
// -------------------------------------------------------------------

// HandleKubeEvents accepts K8s event batches (POST) and serves them (GET).
// Full event store is added in a future iteration; for now we accept + log.
func (h *Handler) HandleKubeEvents(w http.ResponseWriter, r *http.Request) {
	if h.preflight(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		// Acknowledge; downstream RCA will consume these once wired.
		var payload []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			slog.Debug("received k8s events", "count", len(payload))
		}
		w.WriteHeader(http.StatusAccepted)
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}})
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
//   GET  /api/rca/{alertID}          — fetch RCA result
//   POST /api/rca/{alertID}/execute  — manually execute a specific command
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

	if r.Method == http.MethodGet {
		h.getRCAResult(w, r, path)
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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

	// Manual execution (may not be in auto-safe list — let it through with warning).
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

	// ── Active alerts ────────────────────────────────────────────────────────
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
				})
			}
		}
	}

	// ── System metrics ────────────────────────────────────────────────────────
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
					Resource: "host", Message: "Disk usage critical — eviction risk",
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

	// ── K8s metrics ───────────────────────────────────────────────────────────
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

	// ── Overall status ────────────────────────────────────────────────────────
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

// HandleIncidents returns recent notification delivery events.
// GET /api/incidents?limit=50&alert_id=<id>
func (h *Handler) HandleIncidents(w http.ResponseWriter, r *http.Request) {
	h.cors(w)
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
