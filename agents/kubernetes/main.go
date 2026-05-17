package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/onkar717/visual-eyes/agents/kubernetes/events"
	"github.com/onkar717/visual-eyes/agents/kubernetes/logs"
	"github.com/onkar717/visual-eyes/agents/kubernetes/metrics"
	"github.com/onkar717/visual-eyes/backend/config"
	sharedhttp "github.com/onkar717/visual-eyes/backend/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Warn("could not load config file — using defaults", "error", err)
		cfg = defaultConfig()
	}

	// Environment overrides for container deployments.
	if ep := os.Getenv("VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT"); ep != "" {
		cfg.Output.Remote.Endpoint = ep
		cfg.Output.Remote.Enabled = true
	}

	metricsEndpoint := cfg.Output.Remote.Endpoint
	logsEndpoint := strings.Replace(metricsEndpoint, "/api/kubernetes-metrics", "/api/pod-logs", 1)
	eventsEndpoint := strings.Replace(metricsEndpoint, "/api/kubernetes-metrics", "/api/events", 1)
	nodeName := os.Getenv("NODE_NAME")

	slog.Info("VisualEyes Kubernetes Agent starting",
		"metrics_endpoint", metricsEndpoint,
		"logs_endpoint", logsEndpoint,
		"node", nodeName,
	)

	// ── Kubernetes client ─────────────────────────────────────────────────────
	collector, err := metrics.New()
	if err != nil {
		slog.Error("failed to create kubernetes metrics collector", "error", err)
		os.Exit(1)
	}

	httpClient := sharedhttp.NewDefaultClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Metrics goroutine ─────────────────────────────────────────────────────
	go func() {
		ticker := time.NewTicker(sharedhttp.DefaultCollectionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m, err := collector.Collect(ctx)
				if err != nil {
					slog.Warn("metrics collect failed", "error", err)
					continue
				}
				if len(m) == 0 {
					continue
				}
				if err := sharedhttp.SendMetrics(httpClient, metricsEndpoint, m); err != nil {
					slog.Warn("metrics send failed", "error", err)
				} else {
					slog.Debug("sent kubernetes metrics", "count", len(m))
				}
			}
		}
	}()

	// ── Log collection goroutine ──────────────────────────────────────────────
	logDir := os.Getenv("VISUAL_EYES_LOG_DIR")
	if logDir == "" {
		logDir = "/var/log/containers"
	}
	logCollector := logs.NewCollector(logDir, nodeName)
	logShipper := logs.NewShipper(logsEndpoint)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lines, err := logCollector.Collect()
				if err != nil {
					slog.Warn("log collect failed", "error", err)
					continue
				}
				if len(lines) == 0 {
					continue
				}
				if err := logShipper.Ship(lines); err != nil {
					slog.Warn("log ship failed", "error", err)
				}
			}
		}
	}()

	// ── K8s Events goroutine ──────────────────────────────────────────────────
	// Events collector uses the same in-cluster client as the metrics collector.
	k8sClient := collector.Client()
	if k8sClient != nil {
		evCollector := events.NewCollector(k8sClient, eventsEndpoint)
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := evCollector.Collect(ctx); err != nil {
						slog.Warn("events collect failed", "error", err)
					}
				}
			}
		}()
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	slog.Info("kubernetes agent running — waiting for signal")
	<-sigChan
	slog.Info("shutdown signal received — stopping agent")
	cancel()
}

func defaultConfig() *config.Config {
	ep := os.Getenv("VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT")
	if ep == "" {
		ep = "http://localhost:8080/api/kubernetes-metrics"
	}
	return &config.Config{
		Output: config.OutputConfig{
			Remote: config.RemoteOutput{Enabled: true, Endpoint: ep},
		},
	}
}
