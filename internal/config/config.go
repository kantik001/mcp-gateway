package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig describes a single MCP server process.
type ServerConfig struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Command     string            `yaml:"command" json:"command"`
	Args        []string          `yaml:"args" json:"args"`
	Env         map[string]string `yaml:"env" json:"env,omitempty"`
	Enabled     bool              `yaml:"enabled" json:"enabled"`
}

// ServersFile is the YAML document under config/servers.yaml.
type ServersFile struct {
	Servers []ServerConfig `yaml:"servers"`
}

// Config is the runtime configuration for the gateway.
type Config struct {
	Port                string
	DatabaseURL         string
	LogLevel            string
	ConfigPath          string
	HealthCheckInterval time.Duration
	ToolCallTimeout     time.Duration
	APIKey              string
	Servers             []ServerConfig
}

// Load reads YAML servers config and applies environment overrides.
func Load() (*Config, error) {
	cfg := &Config{
		Port:                envOr("PORT", "8080"),
		DatabaseURL:         envOr("DATABASE_URL", ""),
		LogLevel:            envOr("LOG_LEVEL", "info"),
		ConfigPath:          envOr("CONFIG_PATH", "config/servers.yaml"),
		HealthCheckInterval: 30 * time.Second,
		ToolCallTimeout:     30 * time.Second,
		APIKey:              os.Getenv("API_KEY"),
	}

	if v := os.Getenv("HEALTH_CHECK_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid HEALTH_CHECK_INTERVAL: %w", err)
		}
		cfg.HealthCheckInterval = d
	}

	if v := os.Getenv("TOOL_CALL_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TOOL_CALL_TIMEOUT: %w", err)
		}
		cfg.ToolCallTimeout = d
	}

	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", cfg.ConfigPath, err)
	}

	var file ServersFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", cfg.ConfigPath, err)
	}
	cfg.Servers = file.Servers

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvInt is a helper for optional integer env vars.
func EnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
