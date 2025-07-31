package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/onkar717/visual-eyes/backend/api"
	"github.com/onkar717/visual-eyes/backend/storage"
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
	http.HandleFunc("/api/metrics/snapshot", handler.GetMetricsSnapshot)
	http.HandleFunc("/api/kubernetes/metrics", handler.GetKubernetesMetrics)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%s", host, port)
	log.Printf("Starting VisualEyes server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
