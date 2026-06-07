package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config is the root configuration for the VisualEyes server.
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Alerts        AlertsConfig        `mapstructure:"alerts"`
	RCA           RCAConfig           `mapstructure:"rca"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Agent         AgentConfig         `mapstructure:"agent"`
	Output        OutputConfig        `mapstructure:"output"`
}

// NotificationsConfig holds alert delivery integrations.
type NotificationsConfig struct {
	Slack SlackConfig `mapstructure:"slack"`
}

// SlackConfig configures the Slack incoming webhook notifier.
type SlackConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"webhook_url"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	CORSOrigins     []string      `mapstructure:"cors_origins"`
	RateLimit       RateLimitConfig `mapstructure:"rate_limit"`
}

type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

type DatabaseConfig struct {
	// PostgreSQL connection settings.
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	TimeZone string `mapstructure:"timezone"`
	// DSN overrides individual fields when set.
	DSN        string `mapstructure:"dsn"`
	MaxRecords int    `mapstructure:"max_records"`
}

// BuildDSN returns the PostgreSQL DSN, preferring an explicit DSN over individual fields.
func (d DatabaseConfig) BuildDSN() string {
	if d.DSN != "" {
		return d.DSN
	}
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		d.Host, d.User, d.Password, d.DBName, d.Port, d.SSLMode, d.TimeZone,
	)
}

type AlertsConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	EvalInterval    time.Duration `mapstructure:"eval_interval"`
	LookbackWindow  time.Duration `mapstructure:"lookback_window"`
	DeduplicateTTL  time.Duration `mapstructure:"deduplicate_ttl"`
	Rules           []AlertRule   `mapstructure:"rules"`
}

type AlertRule struct {
	Name       string  `mapstructure:"name"`
	MetricName string  `mapstructure:"metric_name"`
	Threshold  float64 `mapstructure:"threshold"`
	Operator   string  `mapstructure:"operator"` // "gt", "lt", "gte", "lte"
	Severity   string  `mapstructure:"severity"` // "critical", "warning", "info"
	TagFilter  map[string]string `mapstructure:"tag_filter"`
}

type RCAConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	APIKey      string `mapstructure:"api_key"`
	Model       string `mapstructure:"model"`
	MaxTokens   int    `mapstructure:"max_tokens"`
	LogLines    int    `mapstructure:"log_lines"`
	MetricSamples int  `mapstructure:"metric_samples"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // "debug", "info", "warn", "error"
	Format string `mapstructure:"format"` // "text", "json"
}

type AgentConfig struct {
	CollectionInterval int  `mapstructure:"collection_interval"`
	DisableHostMetrics bool `mapstructure:"disable_host_metrics"`
	DisableKubeMetrics bool `mapstructure:"disable_kube_metrics"`
}

type OutputConfig struct {
	Remote RemoteOutput `mapstructure:"remote"`
}

type RemoteOutput struct {
	Enabled  bool   `mapstructure:"enabled"`
	Endpoint string `mapstructure:"endpoint"`
}

// Load reads config from file + environment variables.
func Load() (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath("/etc/visual-eyes")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	v.AutomaticEnv()
	v.SetEnvPrefix("VISUAL_EYES")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Allow ANTHROPIC_API_KEY to set rca.api_key without prefix.
	if cfg.RCA.APIKey == "" {
		cfg.RCA.APIKey = v.GetString("ANTHROPIC_API_KEY")
	}

	// Allow SLACK_WEBHOOK_URL env var to set notifications.slack.webhook_url.
	if cfg.Notifications.Slack.WebhookURL == "" {
		if wh := v.GetString("SLACK_WEBHOOK_URL"); wh != "" {
			cfg.Notifications.Slack.WebhookURL = wh
			cfg.Notifications.Slack.Enabled = true
		}
	}

	return &cfg, nil
}

// LoadConfig loads configuration from a specific file path.
func LoadConfig(configFile string) (*Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetConfigFile(configFile)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "15s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "10s")
	v.SetDefault("server.cors_origins", []string{"http://localhost:3000", "http://localhost:5173"})
	v.SetDefault("server.rate_limit.enabled", true)
	v.SetDefault("server.rate_limit.requests_per_second", 100.0)
	v.SetDefault("server.rate_limit.burst", 200)

	// Database (PostgreSQL)
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "visual_eyes")
	v.SetDefault("database.password", "visual_eyes")
	v.SetDefault("database.dbname", "visual_eyes")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.timezone", "UTC")
	v.SetDefault("database.max_records", 10000)

	// Alerts
	v.SetDefault("alerts.enabled", true)
	v.SetDefault("alerts.eval_interval", "30s")
	v.SetDefault("alerts.lookback_window", "5m")
	v.SetDefault("alerts.deduplicate_ttl", "10m")
	v.SetDefault("alerts.rules", defaultAlertRules())

	// Notifications
	v.SetDefault("notifications.slack.enabled", false)
	v.SetDefault("notifications.slack.webhook_url", "")

	// RCA
	v.SetDefault("rca.enabled", false)
	v.SetDefault("rca.model", "claude-sonnet-4-6")
	v.SetDefault("rca.max_tokens", 4096)
	v.SetDefault("rca.log_lines", 100)
	v.SetDefault("rca.metric_samples", 20)

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")

	// Agent / Output (used by agent binaries)
	v.SetDefault("agent.collection_interval", 10)
	v.SetDefault("agent.disable_host_metrics", false)
	v.SetDefault("agent.disable_kube_metrics", false)
	v.SetDefault("output.remote.enabled", false)
	v.SetDefault("output.remote.endpoint", "http://localhost:8080/api/system-metrics")
}

func defaultAlertRules() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "cpu_spike_critical", "metric_name": "cpu.usage", "threshold": 90.0, "operator": "gt", "severity": "critical"},
		{"name": "cpu_spike_warning", "metric_name": "cpu.usage", "threshold": 80.0, "operator": "gt", "severity": "warning"},
		{"name": "memory_spike_critical", "metric_name": "memory.usage_percent", "threshold": 90.0, "operator": "gt", "severity": "critical"},
		{"name": "memory_spike_warning", "metric_name": "memory.usage_percent", "threshold": 85.0, "operator": "gt", "severity": "warning"},
		{"name": "disk_full_critical", "metric_name": "disk.usage_percent", "threshold": 95.0, "operator": "gt", "severity": "critical"},
		{"name": "disk_full_warning", "metric_name": "disk.usage_percent", "threshold": 90.0, "operator": "gt", "severity": "warning"},
		{"name": "k8s_node_cpu_high", "metric_name": "kubernetes.node.cpu.usage", "threshold": 0.85, "operator": "gt", "severity": "warning"},
		{"name": "k8s_pod_crash_loop", "metric_name": "kubernetes.pod.restart_count", "threshold": 5.0, "operator": "gt", "severity": "critical"},
	}
}
