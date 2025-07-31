package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds all configuration for the agent
type Config struct {
	Agent  AgentConfig  `mapstructure:"agent"`
	Output OutputConfig `mapstructure:"output"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	CollectionInterval int  `mapstructure:"collection_interval"`
	DisableHostMetrics bool `mapstructure:"disable_host_metrics"`
	DisableKubeMetrics bool `mapstructure:"disable_kube_metrics"`
}

// OutputConfig holds output configuration
type OutputConfig struct {
	Remote RemoteOutput `mapstructure:"remote"`
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
	v.SetDefault("agent.collection_interval", 10)
	v.SetDefault("agent.disable_host_metrics", false)
	v.SetDefault("agent.disable_kube_metrics", false)

	// Output defaults
	v.SetDefault("output.remote.enabled", false)
	v.SetDefault("output.remote.endpoint", "http://localhost:8080/api/system-metrics")
}
