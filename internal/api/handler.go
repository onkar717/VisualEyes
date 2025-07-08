package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/onkar717/visual-eyes/internal/models"
	"github.com/onkar717/visual-eyes/internal/storage"
)

// enableCORS adds CORS headers to the response
func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// Handler handles HTTP requests for metrics
type Handler struct {
	systemStore     storage.MetricStore
	kubernetesStore storage.MetricStore
	hostname        string
}

// NewHandler creates a new metrics handler
func NewHandler(systemStore, kubernetesStore storage.MetricStore) (*Handler, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	return &Handler{
		systemStore:     systemStore,
		kubernetesStore: kubernetesStore,
		hostname:        hostname,
	}, nil
}

// handleMetricsPost handles common POST request logic for metrics
func (h *Handler) handleMetricsPost(w http.ResponseWriter, r *http.Request, store storage.MetricStore, metricType string) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var metrics []models.Metric
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		log.Printf("Error decoding metrics: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("Received %d %s metrics", len(metrics), metricType)

	// Validate and enrich metrics
	for i := range metrics {
		if err := metrics[i].Validate(); err != nil {
			log.Printf("Error validating metric: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if metrics[i].Timestamp.IsZero() {
			metrics[i].Timestamp = time.Now()
		}

		if metrics[i].Tags == nil {
			metrics[i].Tags = make(map[string]string)
		}
		metrics[i].Tags["hostname"] = h.hostname
		metrics[i].Tags["type"] = metricType
	}

	if err := store.StoreMetrics(metrics); err != nil {
		log.Printf("Error storing %s metrics: %v", metricType, err)
		http.Error(w, "Failed to store metrics", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully stored %d %s metrics", len(metrics), metricType)
	w.WriteHeader(http.StatusCreated)
}

// PostSystemMetrics handles POST requests to store system metrics
func (h *Handler) PostSystemMetrics(w http.ResponseWriter, r *http.Request) {
	h.handleMetricsPost(w, r, h.systemStore, "system")
}

// PostKubernetesMetrics handles POST requests to store Kubernetes metrics
func (h *Handler) PostKubernetesMetrics(w http.ResponseWriter, r *http.Request) {
	h.handleMetricsPost(w, r, h.kubernetesStore, "kubernetes")
}

// GetMetrics handles GET requests to stream metrics
func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel for client disconnection
	notify := w.(http.CloseNotifier).CloseNotify()

	// Start streaming updates
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-notify:
			// Client disconnected
			return
		case <-ticker.C:
			metrics := h.systemStore.GetAllMetrics()
			if err := sendMetricsUpdate(w, metrics); err != nil {
				log.Printf("Error sending metrics update: %v", err)
				return
			}
			w.(http.Flusher).Flush()
		}
	}
}

// sendMetricsUpdate sends a single metrics update to the client
func sendMetricsUpdate(w http.ResponseWriter, metrics []models.Metric) error {
	// Initialize the response structure
	response := map[string]interface{}{
		"hostname":  "", // Will be set from metrics tags
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"metrics": map[string]interface{}{
			"cpu":     map[string]float64{},
			"memory":  map[string]float64{},
			"disk":    map[string]interface{}{},
			"network": map[string]interface{}{},
			"load":    map[string]float64{},
		},
	}

	metricsData := response["metrics"].(map[string]interface{})

	for _, m := range metrics {
		// Round float values to 2 decimal places
		m.Value = float64(int64(m.Value*100)) / 100

		// Set hostname from any metric's tags (they all should have it)
		if hostname, ok := m.Tags["hostname"]; ok && response["hostname"] == "" {
			response["hostname"] = hostname
		}

		switch {
		case strings.HasPrefix(m.Name, "cpu."):
			cpuMetrics := metricsData["cpu"].(map[string]float64)
			if strings.HasSuffix(m.Name, "usage") {
				cpuMetrics["usage_percent"] = m.Value
			} else if strings.HasSuffix(m.Name, "idle") {
				cpuMetrics["idle_percent"] = m.Value
			}

		case strings.HasPrefix(m.Name, "memory."):
			memoryMetrics := metricsData["memory"].(map[string]float64)
			if strings.HasSuffix(m.Name, "used") {
				memoryMetrics["used_bytes"] = m.Value
			} else if strings.HasSuffix(m.Name, "free") {
				memoryMetrics["free_bytes"] = m.Value
			}

		case strings.HasPrefix(m.Name, "disk."):
			diskMetrics := metricsData["disk"].(map[string]interface{})
			if strings.HasSuffix(m.Name, "used") {
				diskMetrics["used_bytes"] = m.Value
			} else if strings.HasSuffix(m.Name, "free") {
				diskMetrics["free_bytes"] = m.Value
			}
			if mountpoint, ok := m.Tags["mountpoint"]; ok {
				diskMetrics["mountpoint"] = mountpoint
			}

		case strings.HasPrefix(m.Name, "network."):
			networkMetrics := metricsData["network"].(map[string]interface{})
			if strings.HasSuffix(m.Name, "bytes_sent") {
				networkMetrics["bytes_sent"] = m.Value
			} else if strings.HasSuffix(m.Name, "bytes_recv") {
				networkMetrics["bytes_recv"] = m.Value
			}
			networkMetrics["interface"] = "total"

		case strings.HasPrefix(m.Name, "load."):
			loadMetrics := metricsData["load"].(map[string]float64)
			if strings.HasSuffix(m.Name, "1min") {
				loadMetrics["load_1min"] = m.Value
			} else if strings.HasSuffix(m.Name, "5min") {
				loadMetrics["load_5min"] = m.Value
			}
		}
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling metrics: %v", err)
	}

	fmt.Fprintf(w, "data: %s\n\n", data)
	return nil
}

// GetMetricsSnapshot handles GET requests to get a single snapshot of metrics
func (h *Handler) GetMetricsSnapshot(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := h.systemStore.GetAllMetrics()
	log.Printf("Retrieved %d metrics from store", len(metrics))

	// Group metrics by type
	groupedMetrics := map[string]map[string]interface{}{
		"cpu":     make(map[string]interface{}),
		"memory":  make(map[string]interface{}),
		"disk":    make(map[string]interface{}),
		"network": make(map[string]interface{}),
		"load":    make(map[string]interface{}),
	}

	for _, m := range metrics {
		// Round float values to 2 decimal places
		m.Value = float64(int64(m.Value*100)) / 100

		var category string
		var name string

		switch {
		case strings.HasPrefix(m.Name, "cpu."):
			category = "cpu"
			name = strings.TrimPrefix(m.Name, "cpu.")
		case strings.HasPrefix(m.Name, "memory."):
			category = "memory"
			name = strings.TrimPrefix(m.Name, "memory.")
		case strings.HasPrefix(m.Name, "disk."):
			category = "disk"
			name = strings.TrimPrefix(m.Name, "disk.")
		case strings.HasPrefix(m.Name, "network."):
			category = "network"
			name = strings.TrimPrefix(m.Name, "network.")
		case strings.HasPrefix(m.Name, "load."):
			category = "load"
			name = strings.TrimPrefix(m.Name, "load.")
		default:
			log.Printf("Unknown metric category for: %s", m.Name)
			continue
		}

		groupedMetrics[category][name] = map[string]interface{}{
			"value":     m.Value,
			"unit":      m.Unit,
			"tags":      m.Tags,
			"timestamp": m.Timestamp,
		}
	}

	response := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"metrics":   groupedMetrics,
	}

	w.Header().Set("Content-Type", "application/json")
	// Use json.MarshalIndent for pretty printing
	data, err := json.MarshalIndent(response, "", "    ")
	if err != nil {
		http.Error(w, "Error formatting response", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

// GetKubernetesMetrics handles GET requests for Kubernetes metrics
func (h *Handler) GetKubernetesMetrics(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := h.kubernetesStore.GetAllMetrics()
	log.Printf("Retrieved %d Kubernetes metrics", len(metrics))

	// Process metrics into the expected format
	response := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"metrics": map[string]interface{}{
			"nodes": map[string]int{
				"total": 1, // Since we have metrics, we know we have at least one node
				"ready": 1,
			},
			"pods": map[string]int{
				"total":   0,
				"running": 0,
			},
			"resources": map[string]interface{}{
				"cpu": map[string]float64{
					"usage": 0,
					"total": 1, // Default to 1 core if not specified
				},
				"memory": map[string]float64{
					"usage": 0,
					"total": 0,
				},
			},
			"podResources": map[string]interface{}{
				"cpu": map[string]float64{
					"usage": 0,
					"total": 1, // Default to 1 core if not specified
				},
				"memory": map[string]float64{
					"usage": 0,
					"total": 0,
				},
			},
		},
	}

	// Count unique pods
	podSet := make(map[string]bool)
	runningPods := 0

	// Process metrics
	for _, metric := range metrics {
		switch metric.Name {
		case "kubernetes.node.cpu.usage":
			response["metrics"].(map[string]interface{})["resources"].(map[string]interface{})["cpu"].(map[string]float64)["usage"] = metric.Value
		case "kubernetes.node.memory.usage":
			response["metrics"].(map[string]interface{})["resources"].(map[string]interface{})["memory"].(map[string]float64)["usage"] = metric.Value
		case "kubernetes.node.memory.available":
			total := metric.Value + response["metrics"].(map[string]interface{})["resources"].(map[string]interface{})["memory"].(map[string]float64)["usage"]
			response["metrics"].(map[string]interface{})["resources"].(map[string]interface{})["memory"].(map[string]float64)["total"] = total
		}

		// Track pods
		if strings.HasPrefix(metric.Name, "kubernetes.pod.") {
			if podName, ok := metric.Tags["pod"]; ok {
				podSet[podName] = true
				// Assume all pods we see metrics for are running
				runningPods++
			}
		}

		// Aggregate pod resource usage
		if metric.Name == "kubernetes.pod.cpu.usage" {
			current := response["metrics"].(map[string]interface{})["podResources"].(map[string]interface{})["cpu"].(map[string]float64)["usage"]
			response["metrics"].(map[string]interface{})["podResources"].(map[string]interface{})["cpu"].(map[string]float64)["usage"] = current + metric.Value
		}
		if metric.Name == "kubernetes.pod.memory.working_set" {
			current := response["metrics"].(map[string]interface{})["podResources"].(map[string]interface{})["memory"].(map[string]float64)["usage"]
			response["metrics"].(map[string]interface{})["podResources"].(map[string]interface{})["memory"].(map[string]float64)["usage"] = current + metric.Value
		}
	}

	// Update pod counts
	totalPods := len(podSet)
	if totalPods > 0 {
		response["metrics"].(map[string]interface{})["pods"].(map[string]int)["total"] = totalPods
		response["metrics"].(map[string]interface{})["pods"].(map[string]int)["running"] = runningPods
	}

	// Set pod resource limits based on node resources
	response["metrics"].(map[string]interface{})["podResources"].(map[string]interface{})["cpu"].(map[string]float64)["total"] =
		response["metrics"].(map[string]interface{})["resources"].(map[string]interface{})["cpu"].(map[string]float64)["total"]
	response["metrics"].(map[string]interface{})["podResources"].(map[string]interface{})["memory"].(map[string]float64)["total"] =
		response["metrics"].(map[string]interface{})["resources"].(map[string]interface{})["memory"].(map[string]float64)["total"]

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
