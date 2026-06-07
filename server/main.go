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

	"github.com/onkar717/visual-eyes/server/alerts"
	"github.com/onkar717/visual-eyes/server/api"
	"github.com/onkar717/visual-eyes/server/config"
	"github.com/onkar717/visual-eyes/server/internal/logger"
	appmetrics "github.com/onkar717/visual-eyes/server/metrics"
	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/notifications"
	"github.com/onkar717/visual-eyes/server/rca"
	"github.com/onkar717/visual-eyes/server/storage"
	"github.com/onkar717/visual-eyes/server/ws"
)

func main() {
	startedAt := time.Now()

	// Config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Logging
	logger.Init(cfg.Logging.Level, cfg.Logging.Format)
	slog.Info("VisualEyes server starting",
		"version", version(),
		"log_level", cfg.Logging.Level,
	)

	// Storage (PostgreSQL)
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

	// Notifications (multi-channel fan-out)
	var activeNotifiers []notifications.Notifier
	var channelNames []string

	if cfg.Notifications.Slack.Enabled && cfg.Notifications.Slack.WebhookURL != "" {
		activeNotifiers = append(activeNotifiers, notifications.NewSlackNotifier(cfg.Notifications.Slack.WebhookURL))
		channelNames = append(channelNames, "slack")
		slog.Info("slack notifications enabled")
	}
	if cfg.Notifications.PagerDuty.Enabled && cfg.Notifications.PagerDuty.RoutingKey != "" {
		activeNotifiers = append(activeNotifiers, notifications.NewPagerDutyNotifier(cfg.Notifications.PagerDuty.RoutingKey))
		channelNames = append(channelNames, "pagerduty")
		slog.Info("pagerduty notifications enabled")
	}
	if cfg.Notifications.Webhook.Enabled && cfg.Notifications.Webhook.URL != "" {
		activeNotifiers = append(activeNotifiers, notifications.NewWebhookNotifier(cfg.Notifications.Webhook.URL, cfg.Notifications.Webhook.Secret))
		channelNames = append(channelNames, "webhook")
		slog.Info("webhook notifications enabled", "url", cfg.Notifications.Webhook.URL)
	}

	var baseNotifier notifications.Notifier
	switch len(activeNotifiers) {
	case 0:
		baseNotifier = notifications.Noop{}
		channelNames = []string{"noop"}
	case 1:
		baseNotifier = activeNotifiers[0]
	default:
		baseNotifier = notifications.NewMultiNotifier(activeNotifiers...)
	}

	// Wrap with LoggingNotifier so every delivery attempt is persisted.
	var notifier notifications.Notifier = baseNotifier
	if ns, ok := store.(storage.NotificationStore); ok {
		channel := "noop"
		if len(channelNames) > 0 {
			channel = channelNames[0]
			if len(channelNames) > 1 {
				channel = "multi"
			}
		}
		notifier = notifications.NewLoggingNotifier(baseNotifier, channel, ns)
	}

	// Alert Engine
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

	// WebSocket Broadcaster
	// broadcaster fans real-time metric snapshots to all connected WS clients.
	broadcaster := ws.NewBroadcaster()

	// Handler + Router
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

	// RCA Engine (Claude)
	appCtx, appCancel := context.WithCancel(context.Background())

	// Observability background loop
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

	// RCA LLM provider selection
	llmProvider := rca.BuildLLMProvider(cfg)
	rcaEnabled := cfg.RCA.Enabled && llmProvider != nil

	if rcaEnabled {
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
			executor := rca.NewExecutor(30 * time.Second)
			processor := rca.NewProcessor(ctxBuilder, llmProvider, executor, rs, as, cfg.RCA.AgentTimeoutSeconds)

			handler.SetRCAStore(rs)
			if is, ok := store.(storage.IncidentStore); ok {
				processor.SetIncidentStore(is)
				handler.SetIncidentStore(is)
			}

			go processor.RunWorker(appCtx, rcaTrigger, 2)
			slog.Info("rca engine started", "provider", cfg.RCA.Provider, "model", cfg.RCA.Model)
		}
	} else {
		slog.Info("rca engine disabled — set rca.enabled=true and a provider API key to enable")
		// Wire store anyway so /api/rca/* returns empty results instead of 503.
		if rs, ok := store.(storage.RCAStore); ok {
			handler.SetRCAStore(rs)
		}
		if is, ok := store.(storage.IncidentStore); ok {
			handler.SetIncidentStore(is)
		}
		go func() {
			for range rcaTrigger {
			}
		}()
	}

	mux := http.NewServeMux()
	router := api.RegisterRoutes(mux, handler, cfg)

	// HTTP Server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Graceful shutdown
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

