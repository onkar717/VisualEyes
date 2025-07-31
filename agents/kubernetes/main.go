package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/onkar717/visual-eyes/agents/kubernetes/metrics"
	"github.com/onkar717/visual-eyes/backend/config"
	sharedhttp "github.com/onkar717/visual-eyes/backend/http"
)

func main() {
	log.Println("Starting VisualEyes Kubernetes Agent...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: Could not load config: %v. Using defaults.", err)
		// Use environment variable or default
		defaultEndpoint := os.Getenv("VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT")
		if defaultEndpoint == "" {
			defaultEndpoint = "http://localhost:8080/api/kubernetes-metrics"
		}
		cfg = &config.Config{
			Output: config.OutputConfig{
				Remote: config.RemoteOutput{
					Enabled:  true,
					Endpoint: defaultEndpoint,
				},
			},
		}
	}

	// Override with environment variable if provided
	endpoint := os.Getenv("VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT")
	if endpoint != "" {
		cfg.Output.Remote.Endpoint = endpoint
		log.Printf("Using endpoint from environment: %s", endpoint)
	} else {
		log.Printf("Using endpoint from config: %s", cfg.Output.Remote.Endpoint)
	}

	// Create a new Kubernetes metrics collector
	collector, err := metrics.New()
	if err != nil {
		log.Fatalf("Failed to create Kubernetes collector: %v", err)
	}

	// Create HTTP client using shared utility
	httpClient := sharedhttp.NewDefaultClient()

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start collecting metrics in a goroutine
	go func() {
		ticker := time.NewTicker(sharedhttp.DefaultCollectionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics, err := collector.Collect(ctx)
				if err != nil {
					log.Printf("Failed to collect Kubernetes metrics: %v", err)
					continue
				}
				log.Printf("Collected %d Kubernetes metrics", len(metrics))

				// Send metrics to backend server using shared utility
				if len(metrics) > 0 {
					if err := sharedhttp.SendMetrics(httpClient, cfg.Output.Remote.Endpoint, metrics); err != nil {
						log.Printf("Failed to send metrics to backend: %v", err)
					} else {
						log.Printf("Successfully sent %d metrics to backend", len(metrics))
					}
				}
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("VisualEyes Kubernetes Agent started. Press Ctrl+C to stop.")
	<-sigChan

	log.Println("Shutting down VisualEyes Kubernetes Agent...")
	cancel()
}
