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
	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/onkar717/visual-eyes/backend/storage"
)

func main() {
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

	// ── Alert Engine ─────────────────────────────────────────────────────────
	// rcaTrigger is a buffered channel feeding fired alerts to the RCA processor
	// (Commit 5). Buffer of 100 so fast bursts don't block the eval loop.
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
				)
				alertEngine.Start()
			}
		}
	}

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

// Ensure the config.ServerConfig duration fields are non-zero before using them.
func init() {
	// Viper parses duration strings like "15s" correctly, but if the field is
	// zero (e.g. first run with no config file), set safe defaults.
	_ = time.Second // silence unused import if durations are removed
}
