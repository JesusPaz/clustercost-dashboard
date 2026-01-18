package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig describes a single ClusterCost agent instance.
type AgentConfig struct {
	Name    string `yaml:"name"`
	BaseURL string `yaml:"baseUrl"`
	Type    string `yaml:"type"`
	Token   string `yaml:"token"`
	Region  string `yaml:"clusterRegion"`
}

// Config contains runtime settings for the dashboard backend.
type Config struct {
	ListenAddr                   string        `yaml:"listenAddr"`
	GrpcAddr                     string        `yaml:"grpcAddr"`
	DefaultAgentToken            string        `yaml:"defaultAgentToken"`
	PollInterval                 time.Duration `yaml:"pollInterval"`
	DisableHTTPPolling           bool          `yaml:"disableHttpPolling"`
	Agents                       []AgentConfig `yaml:"agents"`
	RecommendedAgentVersion      string        `yaml:"recommendedAgentVersion"`
	VictoriaMetricsURL           string        `yaml:"victoriaMetricsUrl"`
	VictoriaMetricsIngestPath    string        `yaml:"victoriaMetricsIngestPath"`
	VictoriaMetricsToken         string        `yaml:"victoriaMetricsToken"`
	VictoriaMetricsUsername      string        `yaml:"victoriaMetricsUsername"`
	VictoriaMetricsPassword      string        `yaml:"victoriaMetricsPassword"`
	VictoriaMetricsTimeout       time.Duration `yaml:"victoriaMetricsTimeout"`
	VictoriaMetricsBatchBytes    int           `yaml:"victoriaMetricsBatchBytes"`
	VictoriaMetricsFlushInterval time.Duration `yaml:"victoriaMetricsFlushInterval"`
	VictoriaMetricsWorkers       int           `yaml:"victoriaMetricsWorkers"`
	VictoriaMetricsQueueSize     int           `yaml:"victoriaMetricsQueueSize"`
	VictoriaMetricsGzip          bool          `yaml:"victoriaMetricsGzip"`
	VictoriaMetricsLookback      time.Duration `yaml:"victoriaMetricsLookback"`
	StoragePath                  string        `yaml:"storagePath"`
	JWTSecret                    string        `yaml:"jwtSecret"`
	LogLevel                     string        `yaml:"logLevel"`
}

// Default returns the default configuration used when no other information is provided.
func Default() Config {
	return Config{
		ListenAddr:                   ":9090",
		GrpcAddr:                     ":9091",
		PollInterval:                 30 * time.Second,
		Agents:                       []AgentConfig{},
		VictoriaMetricsIngestPath:    "/api/v1/import/prometheus",
		VictoriaMetricsTimeout:       5 * time.Second,
		VictoriaMetricsBatchBytes:    2 << 20,
		VictoriaMetricsFlushInterval: 2 * time.Second,
		VictoriaMetricsWorkers:       0,
		VictoriaMetricsQueueSize:     10000,
		VictoriaMetricsGzip:          true,
		VictoriaMetricsLookback:      24 * time.Hour,
		StoragePath:                  "data/clustercost.db",
		JWTSecret:                    "clustercost-secret",
	}
}

// Load reads configuration from environment variables and an optional YAML file.
func Load() (Config, error) {
	cfg := Default()

	if listen := os.Getenv("LISTEN_ADDR"); listen != "" {
		cfg.ListenAddr = listen
	}

	if grpcAddr := os.Getenv("GRPC_ADDR"); grpcAddr != "" {
		cfg.GrpcAddr = grpcAddr
	}

	if defaultToken := os.Getenv("DEFAULT_AGENT_TOKEN"); defaultToken != "" {
		cfg.DefaultAgentToken = defaultToken
	}

	if interval := os.Getenv("POLL_INTERVAL"); interval != "" {
		d, err := time.ParseDuration(interval)
		if err != nil {
			return Config{}, fmt.Errorf("invalid POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = d
		if d == 0 {
			cfg.DisableHTTPPolling = true
		}
	}

	if disable := os.Getenv("DISABLE_HTTP_POLLING"); disable != "" {
		parsed, err := strconv.ParseBool(disable)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DISABLE_HTTP_POLLING: %w", err)
		}
		cfg.DisableHTTPPolling = parsed
	}

	if file := os.Getenv("CONFIG_FILE"); file != "" {
		loaded, err := fromFile(file)
		if err != nil {
			return Config{}, err
		}
		merge(&cfg, loaded)
	}

	if urls := os.Getenv("AGENT_URLS"); urls != "" {
		cfg.Agents = []AgentConfig{}
		for idx, raw := range strings.Split(urls, ",") {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
			name := fmt.Sprintf("agent-%d", idx+1)
			cfg.Agents = append(cfg.Agents, AgentConfig{
				Name:    name,
				BaseURL: trimmed,
				Type:    "k8s",
			})
		}
	}

	if expected := os.Getenv("RECOMMENDED_AGENT_VERSION"); expected != "" {
		cfg.RecommendedAgentVersion = expected
	}

	if vmURL := os.Getenv("VICTORIA_METRICS_URL"); vmURL != "" {
		cfg.VictoriaMetricsURL = vmURL
	}

	if ingestPath := os.Getenv("VICTORIA_METRICS_INGEST_PATH"); ingestPath != "" {
		cfg.VictoriaMetricsIngestPath = ingestPath
	}

	if token := os.Getenv("VICTORIA_METRICS_TOKEN"); token != "" {
		cfg.VictoriaMetricsToken = token
	}

	if user := os.Getenv("VICTORIA_METRICS_USERNAME"); user != "" {
		cfg.VictoriaMetricsUsername = user
	}

	if pass := os.Getenv("VICTORIA_METRICS_PASSWORD"); pass != "" {
		cfg.VictoriaMetricsPassword = pass
	}

	if timeout := os.Getenv("VICTORIA_METRICS_TIMEOUT"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return Config{}, fmt.Errorf("invalid VICTORIA_METRICS_TIMEOUT: %w", err)
		}
		cfg.VictoriaMetricsTimeout = d
	}

	if raw := os.Getenv("VICTORIA_METRICS_BATCH_BYTES"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid VICTORIA_METRICS_BATCH_BYTES: %w", err)
		}
		cfg.VictoriaMetricsBatchBytes = parsed
	}

	if raw := os.Getenv("VICTORIA_METRICS_FLUSH_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid VICTORIA_METRICS_FLUSH_INTERVAL: %w", err)
		}
		cfg.VictoriaMetricsFlushInterval = d
	}

	if raw := os.Getenv("VICTORIA_METRICS_WORKERS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid VICTORIA_METRICS_WORKERS: %w", err)
		}
		cfg.VictoriaMetricsWorkers = parsed
	}

	if raw := os.Getenv("VICTORIA_METRICS_QUEUE_SIZE"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid VICTORIA_METRICS_QUEUE_SIZE: %w", err)
		}
		cfg.VictoriaMetricsQueueSize = parsed
	}

	if raw := os.Getenv("VICTORIA_METRICS_GZIP"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid VICTORIA_METRICS_GZIP: %w", err)
		}
		cfg.VictoriaMetricsGzip = parsed
	}

	if raw := os.Getenv("VICTORIA_METRICS_LOOKBACK"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid VICTORIA_METRICS_LOOKBACK: %w", err)
		}
		cfg.VictoriaMetricsLookback = d
	}

	if storage := os.Getenv("STORAGE_PATH"); storage != "" {
		cfg.StoragePath = storage
	}

	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		cfg.JWTSecret = secret
	}

	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		cfg.LogLevel = strings.ToLower(logLevel)
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	return cfg, nil
}

func fromFile(path string) (Config, error) {
	b, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- loading config from user-supplied path is intended
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":9090"
	}
	if cfg.PollInterval == 0 && !cfg.DisableHTTPPolling {
		cfg.PollInterval = 30 * time.Second
	}

	return cfg, nil
}

func merge(dst *Config, src Config) {
	if src.ListenAddr != "" {
		dst.ListenAddr = src.ListenAddr
	}
	if src.GrpcAddr != "" {
		dst.GrpcAddr = src.GrpcAddr
	}
	if src.DefaultAgentToken != "" {
		dst.DefaultAgentToken = src.DefaultAgentToken
	}
	if src.PollInterval != 0 {
		dst.PollInterval = src.PollInterval
	}
	if src.DisableHTTPPolling {
		dst.DisableHTTPPolling = true
	}

	if len(src.Agents) > 0 {
		dst.Agents = src.Agents
	}
	if src.RecommendedAgentVersion != "" {
		dst.RecommendedAgentVersion = src.RecommendedAgentVersion
	}

	if src.VictoriaMetricsURL != "" {
		dst.VictoriaMetricsURL = src.VictoriaMetricsURL
	}
	if src.VictoriaMetricsIngestPath != "" {
		dst.VictoriaMetricsIngestPath = src.VictoriaMetricsIngestPath
	}
	if src.VictoriaMetricsToken != "" {
		dst.VictoriaMetricsToken = src.VictoriaMetricsToken
	}
	if src.VictoriaMetricsUsername != "" {
		dst.VictoriaMetricsUsername = src.VictoriaMetricsUsername
	}
	if src.VictoriaMetricsPassword != "" {
		dst.VictoriaMetricsPassword = src.VictoriaMetricsPassword
	}
	if src.VictoriaMetricsTimeout != 0 {
		dst.VictoriaMetricsTimeout = src.VictoriaMetricsTimeout
	}
	if src.VictoriaMetricsBatchBytes != 0 {
		dst.VictoriaMetricsBatchBytes = src.VictoriaMetricsBatchBytes
	}
	if src.VictoriaMetricsFlushInterval != 0 {
		dst.VictoriaMetricsFlushInterval = src.VictoriaMetricsFlushInterval
	}
	if src.VictoriaMetricsWorkers != 0 {
		dst.VictoriaMetricsWorkers = src.VictoriaMetricsWorkers
	}
	if src.VictoriaMetricsQueueSize != 0 {
		dst.VictoriaMetricsQueueSize = src.VictoriaMetricsQueueSize
	}
	if src.VictoriaMetricsGzip {
		dst.VictoriaMetricsGzip = true
	}
	// ... (in merge function)
	if src.VictoriaMetricsLookback != 0 {
		dst.VictoriaMetricsLookback = src.VictoriaMetricsLookback
	}
	if src.StoragePath != "" {
		dst.StoragePath = src.StoragePath
	}
	if src.JWTSecret != "" {
		dst.JWTSecret = src.JWTSecret
	}
}
