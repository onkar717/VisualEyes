package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the agent
type Config struct {
	Agent     AgentConfig     `mapstructure:"agent"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	Output    OutputConfig    `mapstructure:"output"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	Name               string          `mapstructure:"name"`
	Interval           time.Duration   `mapstructure:"interval"`
	Verbose            bool            `mapstructure:"verbose"`
	CollectionInterval int             `mapstructure:"collection_interval"`
	DisableHostMetrics bool            `mapstructure:"disable_host_metrics"`
	DisableKubeMetrics bool            `mapstructure:"disable_kube_metrics"`
	Collectors         CollectorConfig `mapstructure:"collectors"`
}

// CollectorConfig holds enable/disable flags for collectors
type CollectorConfig struct {
	CPU     bool `mapstructure:"cpu"`
	Memory  bool `mapstructure:"memory"`
	Disk    bool `mapstructure:"disk"`
	Network bool `mapstructure:"network"`
	Load    bool `mapstructure:"load"`
}

// TelemetryConfig holds all telemetry configuration
type TelemetryConfig struct {
	Metrics MetricsConfig `mapstructure:"metrics"`
}

// MetricsConfig holds metrics-specific configuration
type MetricsConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Collectors []string      `mapstructure:"collectors"`
	Interval   time.Duration `mapstructure:"interval"`
}

// OutputConfig holds output configuration
type OutputConfig struct {
	Console ConsoleOutput `mapstructure:"console"`
	File    FileOutput    `mapstructure:"file"`
	Remote  RemoteOutput  `mapstructure:"remote"`
}

// ConsoleOutput configures console output
type ConsoleOutput struct {
	Enabled bool   `mapstructure:"enabled"`
	Format  string `mapstructure:"format"`
}

// FileOutput configures file output
type FileOutput struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// RemoteOutput configures remote output
type RemoteOutput struct {
	Enabled  bool   `mapstructure:"enabled"`
	Endpoint string `mapstructure:"endpoint"`
}

// Load reads the configuration file and environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set default configurations
	setDefaults(v)

	// Read config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Read environment variables
	v.AutomaticEnv()
	v.SetEnvPrefix("VISUAL_EYES")

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}

// LoadConfig loads configuration from a specific file
func LoadConfig(configFile string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configFile)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func setDefaults(v *viper.Viper) {
	// Agent defaults
	v.SetDefault("agent.name", "visual-eyes-agent")
	v.SetDefault("agent.interval", "15s")
	v.SetDefault("agent.verbose", false)
	v.SetDefault("agent.collection_interval", 10)
	v.SetDefault("agent.disable_host_metrics", false)
	v.SetDefault("agent.disable_kube_metrics", false)
	v.SetDefault("agent.collectors.cpu", true)
	v.SetDefault("agent.collectors.memory", true)
	v.SetDefault("agent.collectors.disk", true)
	v.SetDefault("agent.collectors.network", true)
	v.SetDefault("agent.collectors.load", true)

	// Metrics defaults
	v.SetDefault("telemetry.metrics.enabled", true)
	v.SetDefault("telemetry.metrics.collectors", []string{"cpu", "memory", "disk", "network", "load"})
	v.SetDefault("telemetry.metrics.interval", "15s")

	// Output defaults
	v.SetDefault("output.console.enabled", true)
	v.SetDefault("output.console.format", "json")
	v.SetDefault("output.file.enabled", false)
	v.SetDefault("output.file.path", "/var/log/visual-eyes.log")
	v.SetDefault("output.remote.enabled", false)
	v.SetDefault("output.remote.endpoint", "http://localhost:8080")
}
