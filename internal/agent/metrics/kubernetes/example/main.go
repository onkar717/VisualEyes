package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/onkar717/visual-eyes/internal/agent/metrics/kubernetes/collector"
)

func main() {
	// Create a new collector
	k8sCollector, err := collector.New()
	if err != nil {
		fmt.Printf("Failed to create collector: %v\n", err)
		os.Exit(1)
	}

	// Collect metrics
	metrics, err := k8sCollector.Collect(context.Background())
	if err != nil {
		fmt.Printf("Failed to collect metrics: %v\n", err)
		os.Exit(1)
	}

	// Pretty print the metrics
	output, _ := json.MarshalIndent(metrics, "", "  ")
	fmt.Println(string(output))
}
