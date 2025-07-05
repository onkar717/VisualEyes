package main

import (
	"log"
	"net/http"

	"github.com/onkar717/visual-eyes/internal/api"
	"github.com/onkar717/visual-eyes/internal/storage"
)

func main() {
	// Initialize in-memory storage
	systemStore := storage.NewMemoryStore()
	k8sStore := storage.NewMemoryStore()

	// Initialize handler with both stores
	handler, err := api.NewHandler(systemStore, k8sStore)
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	// Set up routes
	http.HandleFunc("/api/system-metrics", handler.PostSystemMetrics)
	http.HandleFunc("/api/kubernetes-metrics", handler.PostKubernetesMetrics)
	http.HandleFunc("/api/metrics/stream", handler.GetMetrics)
	http.HandleFunc("/api/metrics/snapshot", handler.GetMetricsSnapshot)
	http.HandleFunc("/api/kubernetes/metrics", handler.GetKubernetesMetrics)

	// Start server
	log.Println("Starting VisualEyes server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
