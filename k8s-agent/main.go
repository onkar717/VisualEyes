package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	k8sexec "github.com/onkar717/visual-eyes/k8s-agent/exec"
	"github.com/onkar717/visual-eyes/k8s-agent/events"
	"github.com/onkar717/visual-eyes/k8s-agent/logs"
	"github.com/onkar717/visual-eyes/k8s-agent/metrics"
	"github.com/onkar717/visual-eyes/server/config"
	sharedhttp "github.com/onkar717/visual-eyes/server/http"
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

	// Namespace filter: env var VISUAL_EYES_AGENT_NAMESPACES overrides config.
	// Comma-separated, e.g. "default,kube-system". Empty = all namespaces.
	allowedNamespaces := cfg.Agent.Namespaces
	if nsEnv := os.Getenv("VISUAL_EYES_AGENT_NAMESPACES"); nsEnv != "" {
		allowedNamespaces = strings.Split(nsEnv, ",")
	}

	slog.Info("VisualEyes Kubernetes Agent starting",
		"metrics_endpoint", metricsEndpoint,
		"logs_endpoint", logsEndpoint,
		"node", nodeName,
		"allowed_namespaces", allowedNamespaces,
	)

	// Kubernetes client
	collector, err := metrics.New(allowedNamespaces)
	if err != nil {
		slog.Error("failed to create kubernetes metrics collector", "error", err)
		os.Exit(1)
	}

	httpClient := sharedhttp.NewDefaultClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Metrics goroutine
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

	// Log collection goroutine
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

	// K8s Events goroutine
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

	// Exec endpoint — allows the server to run commands inside pods in-cluster.
	// Listens on VISUAL_EYES_EXEC_PORT (default 8090).
	execPort := os.Getenv("VISUAL_EYES_EXEC_PORT")
	if execPort == "" {
		execPort = "8090"
	}
	exectr, execErr := k8sexec.NewExecutor()
	if execErr != nil {
		slog.Warn("exec endpoint disabled — not running in-cluster", "error", execErr)
	} else {
		mux := http.NewServeMux()
		mux.HandleFunc("/exec", makeExecHandler(exectr))
		go func() {
			slog.Info("exec endpoint listening", "port", execPort)
			if err := http.ListenAndServe(":"+execPort, mux); err != nil && err != http.ErrServerClosed {
				slog.Warn("exec server stopped", "error", err)
			}
		}()
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	slog.Info("kubernetes agent running — waiting for signal")
	<-sigChan
	slog.Info("shutdown signal received — stopping agent")
	cancel()
}

type execRequest struct {
	Namespace string   `json:"namespace"`
	Pod       string   `json:"pod"`
	Container string   `json:"container"`
	Command   []string `json:"command"`
}

type execResponse struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Error  string `json:"error,omitempty"`
}

func makeExecHandler(e *k8sexec.Executor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req execRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Pod == "" || len(req.Command) == 0 {
			http.Error(w, "pod and command required", http.StatusBadRequest)
			return
		}
		if req.Namespace == "" {
			req.Namespace = "default"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := e.Exec(ctx, k8sexec.ExecOptions{
			Namespace: req.Namespace,
			Pod:       req.Pod,
			Container: req.Container,
			Command:   req.Command,
		})

		resp := execResponse{}
		if result != nil {
			resp.Stdout = result.Stdout
			resp.Stderr = result.Stderr
		}
		if err != nil {
			resp.Error = err.Error()
		}

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(resp)
	}
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
