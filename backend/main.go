package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/onkar717/visual-eyes/backend/alerts"
	"github.com/onkar717/visual-eyes/backend/api"
	"github.com/onkar717/visual-eyes/backend/config"
	"github.com/onkar717/visual-eyes/backend/internal/logger"
	appmetrics "github.com/onkar717/visual-eyes/backend/metrics"
	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/onkar717/visual-eyes/backend/notifications"
	"github.com/onkar717/visual-eyes/backend/rca"
	"github.com/onkar717/visual-eyes/backend/storage"
	"github.com/onkar717/visual-eyes/backend/ws"
)

func main() {
	startedAt := time.Now()

	// ── Config ────────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// ── Logging ───────────────────────────────────────────────────────────────
	logger.Init(cfg.Logging.Level, cfg.Logging.Format)
	slog.Info("VisualEyes server starting",
		"version", version(),
		"log_level", cfg.Logging.Level,
	)

	// ── Storage (PostgreSQL) ─────────────────────────────────────────────────
	// A single PostgresStore handles all feature tables. Falls back to
	// in-memory if Postgres is unreachable (dev/no-DB mode).
	var store storage.MetricStore
	pgStore, dbErr := storage.NewPostgresStore(cfg.Database.BuildDSN(), cfg.Database.MaxRecords)
	if dbErr != nil {
		slog.Error("failed to connect to postgres — falling back to in-memory store",
			"error", dbErr,
			"dsn_hint", cfg.Database.Host+":"+fmt.Sprint(cfg.Database.Port),
		)
		store = storage.NewMemoryStore()
	} else {
		store = pgStore
	}
	systemStore := store
	k8sStore := store

	// ── Notifications ─────────────────────────────────────────────────────────
	var baseNotifier notifications.Notifier = notifications.Noop{}
	channel := "noop"
	if cfg.Notifications.Slack.Enabled && cfg.Notifications.Slack.WebhookURL != "" {
		baseNotifier = notifications.NewSlackNotifier(cfg.Notifications.Slack.WebhookURL)
		channel = "slack"
		slog.Info("slack notifications enabled")
	}
	// Wrap with LoggingNotifier so every delivery attempt is persisted.
	var notifier notifications.Notifier = baseNotifier
	if ns, ok := store.(storage.NotificationStore); ok {
		notifier = notifications.NewLoggingNotifier(baseNotifier, channel, ns)
	}

	// ── Alert Engine ─────────────────────────────────────────────────────────
	// rcaTrigger is a buffered channel feeding fired alerts to the RCA processor.
	// Buffer of 100 so fast bursts don't block the eval loop.
	rcaTrigger := make(chan models.Alert, 100)

	var alertEngine *alerts.Engine
	if cfg.Alerts.Enabled {
		if qs, ok := store.(storage.QueryableStore); ok {
			if as, ok := store.(storage.AlertStore); ok {
				rules := alerts.FromConfig(cfg.Alerts.Rules)
				alertEngine = alerts.NewEngine(
					qs, as, rules,
					cfg.Alerts.EvalInterval,
					cfg.Alerts.LookbackWindow,
					rcaTrigger,
					notifier,
				)
				alertEngine.Start()
			}
		}
	}

	// ── WebSocket Broadcaster ────────────────────────────────────────────────
	// broadcaster fans real-time metric snapshots to all connected WS clients.
	broadcaster := ws.NewBroadcaster()

	// ── Handler + Router ──────────────────────────────────────────────────────
	handler, err := api.NewHandler(systemStore, k8sStore, cfg.Server.CORSOrigins)
	if err != nil {
		slog.Error("failed to create handler", "error", err)
		os.Exit(1)
	}
	if as, ok := store.(storage.AlertStore); ok {
		handler.SetAlertStore(as)
	}
	if ls, ok := store.(storage.LogStore); ok {
		handler.SetLogStore(ls)
	}
	if ns, ok := store.(storage.NotificationStore); ok {
		handler.SetNotificationStore(ns)
	}
	handler.SetBroadcaster(broadcaster)

	// ── RCA Engine (Claude) ───────────────────────────────────────────────────
	appCtx, appCancel := context.WithCancel(context.Background())

	// ── Observability background loop ────────────────────────────────────────
	// Ticks every 5 s to refresh the Prometheus uptime gauge and push a metric
	// snapshot to any connected WebSocket clients (so the UI stays live even
	// if no new data arrived since the last ingestion).
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-appCtx.Done():
				return
			case <-ticker.C:
				appmetrics.UptimeSeconds.Set(time.Since(startedAt).Seconds())
				appmetrics.WSClients.Set(float64(broadcaster.Len()))
			}
		}
	}()

	if cfg.RCA.Enabled && cfg.RCA.APIKey != "" {
		qs, qsOK := store.(storage.QueryableStore)
		ls, lsOK := store.(storage.LogStore)
		as, asOK := store.(storage.AlertStore)
		rs, rsOK := store.(storage.RCAStore)

		if qsOK && asOK && rsOK {
			var logStore storage.LogStore
			if lsOK {
				logStore = ls
			}
			ctxBuilder := rca.NewContextBuilder(qs, logStore, as,
				cfg.RCA.LogLines, cfg.RCA.MetricSamples)
			claudeClient := rca.NewClaudeClient(cfg.RCA.APIKey, cfg.RCA.Model, cfg.RCA.MaxTokens)
			executor := rca.NewExecutor(30 * time.Second)
			processor := rca.NewProcessor(ctxBuilder, claudeClient, executor, rs, as)

			handler.SetRCAStore(rs)

			go processor.RunWorker(appCtx, rcaTrigger, 2)
			slog.Info("rca engine started", "model", cfg.RCA.Model)
		}
	} else {
		slog.Info("rca engine disabled — set rca.enabled=true and ANTHROPIC_API_KEY to enable")
		// Still drain the trigger channel so the alert engine doesn't block.
		go func() {
			for range rcaTrigger {
			}
		}()
	}

	mux := http.NewServeMux()
	router := api.RegisterRoutes(mux, handler, cfg)

	// ── HTTP Server ───────────────────────────────────────────────────────────
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sig := <-shutdownCh
	slog.Info("shutdown signal received", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	appCancel() // stop RCA workers
	handler.StopRateLimiter()
	if alertEngine != nil {
		alertEngine.Stop()
	}
	close(rcaTrigger)

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped cleanly")
}

// version returns the binary version from the VERSION env var or "dev".
func version() string {
	if v := os.Getenv("VISUAL_EYES_VERSION"); v != "" {
		return v
	}
	return "dev"
}

