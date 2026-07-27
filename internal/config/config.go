package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig describes a single MCP server (stdio process or WASM module).
type ServerConfig struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	// Runtime selects the backend: ""/"stdio" (default) or "wasm".
	Runtime     string            `yaml:"runtime" json:"runtime,omitempty"`
	Command     string            `yaml:"command" json:"command,omitempty"`
	Args        []string          `yaml:"args" json:"args,omitempty"`
	Env         map[string]string `yaml:"env" json:"env,omitempty"`
	// WASM is the path to a .wasm guest when Runtime is "wasm".
	WASM        string            `yaml:"wasm" json:"wasm,omitempty"`
	Enabled     bool              `yaml:"enabled" json:"enabled"`
}

// RuntimeStdio is the default MCP stdio subprocess runtime.
const RuntimeStdio = "stdio"

// RuntimeWASM runs tools inside wazero (sandboxed guest).
const RuntimeWASM = "wasm"

// EffectiveRuntime returns the normalized runtime name.
func (s ServerConfig) EffectiveRuntime() string {
	switch s.Runtime {
	case "", RuntimeStdio:
		return RuntimeStdio
	case RuntimeWASM:
		return RuntimeWASM
	default:
		return s.Runtime
	}
}

// ServersFile is the YAML document under config/servers.yaml.
type ServersFile struct {
	Servers []ServerConfig `yaml:"servers"`
}

// Config is the runtime configuration for the gateway.
type Config struct {
	Port                string
	GRPCPort            string
	DatabaseURL         string
	LogLevel            string
	ConfigPath          string
	HealthCheckInterval time.Duration
	ToolCallTimeout     time.Duration
	APIKey              string
	OTELEndpoint        string
	DefaultTenant       string
	Servers             []ServerConfig
}

// Load reads YAML servers config and applies environment overrides.
func Load() (*Config, error) {
	cfg := &Config{
		Port:                envOr("PORT", "8080"),
		GRPCPort:            envOr("GRPC_PORT", "8081"),
		DatabaseURL:         envOr("DATABASE_URL", ""),
		LogLevel:            envOr("LOG_LEVEL", "info"),
		ConfigPath:          envOr("CONFIG_PATH", "config/servers.yaml"),
		HealthCheckInterval: 30 * time.Second,
		ToolCallTimeout:     30 * time.Second,
		APIKey:              os.Getenv("API_KEY"),
		OTELEndpoint:        envOr("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		DefaultTenant:       envOr("DEFAULT_TENANT", "default"),
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
