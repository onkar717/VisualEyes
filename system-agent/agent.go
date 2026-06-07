package main

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/system-agent/metrics"
	"github.com/onkar717/visual-eyes/server/config"
	sharedhttp "github.com/onkar717/visual-eyes/server/http"
	"github.com/onkar717/visual-eyes/server/models"
)

// Agent represents the main VisualEyes agent
type Agent struct {
	config         *config.Config
	metricsChan    chan []models.Metric
	stopChan       chan struct{}
	wg             sync.WaitGroup
	serverEndpoint string
	contextInfo    *ContextInfo
	httpClient     *http.Client
}

// NewAgent creates a new agent instance
func NewAgent(cfg *config.Config) *Agent {
	endpoint := os.Getenv("VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT")
	log.Printf("Environment endpoint: %s", endpoint)

	if endpoint == "" && cfg != nil && cfg.Output.Remote.Enabled {
		endpoint = cfg.Output.Remote.Endpoint
		log.Printf("Using config endpoint: %s", endpoint)
	}
	if endpoint == "" {
		endpoint = "http://localhost:8080/api/system-metrics" // default
		log.Printf("Using default endpoint: %s", endpoint)
	}
	log.Printf("Final server endpoint: %s", endpoint)

	// Detect runtime context
	ctx := Detect()
	log.Printf("Agent context: Kubernetes=%v, Container=%v", ctx.IsKubernetes, ctx.IsRunningInsideContainer)

	// Create HTTP client using shared utility
	httpClient := sharedhttp.NewDefaultClient()

	return &Agent{
		config:         cfg,
		metricsChan:    make(chan []models.Metric, 100),
		stopChan:       make(chan struct{}),
		serverEndpoint: endpoint,
		contextInfo:    ctx,
		httpClient:     httpClient,
	}
}

// Start begins the agent's collection processes
func (a *Agent) Start() error {
	log.Printf("Starting VisualEyes Agent with collection interval: %ds", a.config.Agent.CollectionInterval)

	// Start single collector routine
	a.wg.Add(1)
	go a.collectMetrics()

	// Start sender
	a.wg.Add(1)
	go a.sendMetrics()

	return nil
}

func (a *Agent) Stop() {
	close(a.stopChan)
	a.wg.Wait()
}

func (a *Agent) collectMetrics() {
	defer a.wg.Done()
	ticker := time.NewTicker(time.Duration(a.config.Agent.CollectionInterval) * time.Second)
	defer ticker.Stop()

	// Log initial collection mode
	a.logConfig()

	for {
		select {
		case <-a.stopChan:
			return
		case <-ticker.C:
			log.Println("Starting metric collection...")
			var allMetrics []models.Metric

			// Host metrics collection
			if !a.config.Agent.DisableHostMetrics {
				// CPU metrics
				if cpuMetrics, err := metrics.CollectCPU(); err == nil {
					allMetrics = append(allMetrics, cpuMetrics...)
				} else {
					log.Printf("Error collecting CPU metrics: %v", err)
				}

				// Memory metrics
				if memMetrics, err := metrics.CollectMemory(); err == nil {
					allMetrics = append(allMetrics, memMetrics...)
				} else {
					log.Printf("Error collecting memory metrics: %v", err)
				}

				// Disk metrics
				if diskMetrics, err := metrics.CollectDisk(); err == nil {
					allMetrics = append(allMetrics, diskMetrics...)
				} else {
					log.Printf("Error collecting disk metrics: %v", err)
				}

				// Network metrics
				if netMetrics, err := metrics.CollectNetwork(); err == nil {
					allMetrics = append(allMetrics, netMetrics...)
				} else {
					log.Printf("Error collecting network metrics: %v", err)
				}

				// Load metrics
				if loadMetrics, err := metrics.CollectLoad(); err == nil {
					allMetrics = append(allMetrics, loadMetrics...)
				} else {
					log.Printf("Error collecting load metrics: %v", err)
				}
			}

			// Send metrics if any were collected
			if len(allMetrics) > 0 {
				log.Printf("Sending %d metrics to server...", len(allMetrics))
				a.metricsChan <- allMetrics
			} else {
				log.Println("No metrics collected in this interval")
			}
		}
	}
}

// logConfig logs the current metric collection configuration
func (a *Agent) logConfig() {
	log.Printf("Agent Collection Mode:")
	log.Printf("- Running in Kubernetes: %v", a.contextInfo.IsKubernetes)
	log.Printf("- Running in Container: %v", a.contextInfo.IsRunningInsideContainer)
	log.Printf("- Host Metrics Disabled: %v", a.config.Agent.DisableHostMetrics)
	log.Printf("- Kubernetes Metrics Disabled: %v", a.config.Agent.DisableKubeMetrics)

	if !a.config.Agent.DisableKubeMetrics && a.contextInfo.IsKubernetes {
		log.Printf("✓ Will collect Kubernetes metrics")
	}
	if !a.config.Agent.DisableHostMetrics && (!a.contextInfo.IsKubernetes || !a.contextInfo.IsRunningInsideContainer) {
		log.Printf("✓ Will collect host metrics")
	}
}

func (a *Agent) sendMetrics() {
	defer a.wg.Done()

	for {
		select {
		case <-a.stopChan:
			return
		case metrics := <-a.metricsChan:
			log.Printf("Attempting to send %d metrics to %s", len(metrics), a.serverEndpoint)
			if err := sharedhttp.SendMetrics(a.httpClient, a.serverEndpoint, metrics); err != nil {
				log.Printf("Error sending metrics: %v", err)
			} else {
				log.Printf("Successfully sent %d metrics", len(metrics))
			}
		}
	}
}
