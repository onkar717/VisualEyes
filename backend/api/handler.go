package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	sharedhttp "github.com/onkar717/visual-eyes/backend/http"
	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/onkar717/visual-eyes/backend/storage"
)

// roundValue rounds a float64 value to 2 decimal places
func roundValue(value float64) float64 {
	return float64(int64(value*100)) / 100
}

// enableCORS adds CORS headers to the response
func enableCORS(w http.ResponseWriter) {
	corsOrigin := os.Getenv("CORS_ALLOWED_ORIGINS")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:3000,http://localhost:5173" // Default for dev
	}
	w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
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
		m.Value = roundValue(m.Value)

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

	w.Header().Set("Content-Type", sharedhttp.ContentTypeJSON)
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
			getResourceMetrics(response, "cpu")["usage"] = metric.Value
		case "kubernetes.node.memory.usage":
			getResourceMetrics(response, "memory")["usage"] = metric.Value
		case "kubernetes.node.memory.available":
			memoryMetrics := getResourceMetrics(response, "memory")
			total := metric.Value + memoryMetrics["usage"]
			memoryMetrics["total"] = total
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
			podCPUMetrics := getPodResourceMetrics(response, "cpu")
			podCPUMetrics["usage"] = podCPUMetrics["usage"] + metric.Value
		}
		if metric.Name == "kubernetes.pod.memory.working_set" {
			podMemoryMetrics := getPodResourceMetrics(response, "memory")
			podMemoryMetrics["usage"] = podMemoryMetrics["usage"] + metric.Value
		}
	}

	// Update pod counts
	totalPods := len(podSet)
	if totalPods > 0 {
		response["metrics"].(map[string]interface{})["pods"].(map[string]int)["total"] = totalPods
		response["metrics"].(map[string]interface{})["pods"].(map[string]int)["running"] = runningPods
	}

	// Set pod resource limits based on node resources
	getPodResourceMetrics(response, "cpu")["total"] = getResourceMetrics(response, "cpu")["total"]
	getPodResourceMetrics(response, "memory")["total"] = getResourceMetrics(response, "memory")["total"]

	w.Header().Set("Content-Type", sharedhttp.ContentTypeJSON)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// getResourceMetrics safely extracts resource metrics map from response
func getResourceMetrics(response map[string]interface{}, resourceType string) map[string]float64 {
	return response["metrics"].(map[string]interface{})["resources"].(map[string]interface{})[resourceType].(map[string]float64)
}

// getPodResourceMetrics safely extracts pod resource metrics map from response
func getPodResourceMetrics(response map[string]interface{}, resourceType string) map[string]float64 {
	return response["metrics"].(map[string]interface{})["podResources"].(map[string]interface{})[resourceType].(map[string]float64)
}
