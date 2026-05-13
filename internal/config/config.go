package config

import (
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig   `mapstructure:"server"`
	Proxy   ProxyConfig    `mapstructure:"proxy"`
	Rate    RateConfig     `mapstructure:"rate_limit"`
	Health  HealthConfig   `mapstructure:"health"`
	Admin   AdminConfig    `mapstructure:"admin"`
	Plugins []PluginConfig `mapstructure:"plugins"`
	Gzip    GzipConfig     `mapstructure:"gzip"`
}

type HealthConfig struct {
	Path            string `mapstructure:"path"`
	IntervalSeconds int    `mapstructure:"interval_seconds"`
	TimeoutSeconds  int    `mapstructure:"timeout_seconds"`
}

type ServerConfig struct {
	Host                string `mapstructure:"host"`
	Port                int    `mapstructure:"port"`
	HTTPSPort           int    `mapstructure:"https_port"`
	ReadTimeout         int    `mapstructure:"read_timeout"`
	WriteTimeout        int    `mapstructure:"write_timeout"`
	IdleTimeout         int    `mapstructure:"idle_timeout"`
	TLSCertFile         string `mapstructure:"tls_cert_file"`
	TLSKeyFile          string `mapstructure:"tls_key_file"`
	RedirectHTTPToHTTPS bool   `mapstructure:"redirect_http_to_https"`
}

type ProxyConfig struct {
	Algorithm string        `mapstructure:"algorithm"`
	Routes    []RouteConfig `mapstructure:"routes"`
}

type RouteConfig struct {
	Name           string               `mapstructure:"name"`
	Host           string               `mapstructure:"host"`
	PathPrefix     string               `mapstructure:"path_prefix"`
	Upstreams      []UpstreamConfig     `mapstructure:"upstreams"`
	TimeoutSeconds int                  `mapstructure:"timeout_seconds"`
	MaxBodyBytes   int64                `mapstructure:"max_body_bytes"`
	MaxRetries     int                  `mapstructure:"max_retries"`
	Plugins        []PluginConfig       `mapstructure:"plugins"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
}

type UpstreamConfig struct {
	URL    string `mapstructure:"url"`
	Weight int    `mapstructure:"weight"`
}

type CircuitBreakerConfig struct {
	FailureThreshold    int `mapstructure:"failure_threshold"`
	OpenDurationSeconds int `mapstructure:"open_duration_seconds"`
	HalfOpenRequests    int `mapstructure:"half_open_requests"`
}

type PluginConfig struct {
	Name    string                 `mapstructure:"name"`
	Enabled bool                   `mapstructure:"enabled"`
	Config  map[string]interface{} `mapstructure:"config"`
}

type RateConfig struct {
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

type AdminConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
}

type GzipConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	Level        int      `mapstructure:"level"`
	MinLength    int      `mapstructure:"min_length"`
	ContentTypes []string `mapstructure:"content_types"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	applyDefaults(&cfg)

	return &cfg, nil
}

// Reload re-reads the config file and returns a fresh Config.
func Reload() (*Config, error) {
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	applyDefaults(&cfg)

	return &cfg, nil
}

// LoadNginx parses a nginx.conf file and returns a *Config.
func LoadNginx(path string) (*Config, error) {
	nodes, err := ParseNginxFile(path)
	if err != nil {
		return nil, err
	}

	cfg, err := FromNginx(nodes)
	if err != nil {
		return nil, err
	}

	applyDefaults(cfg)

	return cfg, nil
}

// LoadAuto detects format from file extension: .conf/.nginx → nginx, else YAML.
func LoadAuto(path string) (*Config, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".conf", ".nginx":
		return LoadNginx(path)
	default:
		return Load()
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Admin.Host == "" {
		cfg.Admin.Host = "127.0.0.1"
	}

	if cfg.Admin.Port == 0 {
		cfg.Admin.Port = 8090
	}

	if cfg.Gzip.Level == 0 {
		cfg.Gzip.Level = 6
	}

	if cfg.Gzip.MinLength == 0 {
		cfg.Gzip.MinLength = 1024
	}

	if len(cfg.Gzip.ContentTypes) == 0 {
		cfg.Gzip.ContentTypes = []string{
			"text/plain",
			"text/html",
			"text/css",
			"application/json",
			"application/javascript",
		}
	}

	if cfg.Health.Path == "" {
		cfg.Health.Path = "/"
	}

	if cfg.Health.IntervalSeconds == 0 {
		cfg.Health.IntervalSeconds = 5
	}

	if cfg.Health.TimeoutSeconds == 0 {
		cfg.Health.TimeoutSeconds = 2
	}

	for i := range cfg.Proxy.Routes {
		r := &cfg.Proxy.Routes[i]

		if r.PathPrefix == "" {
			r.PathPrefix = "/"
		}

		if r.TimeoutSeconds == 0 {
			r.TimeoutSeconds = 30
		}

		if r.MaxRetries == 0 {
			r.MaxRetries = 2
		}

		if r.CircuitBreaker.FailureThreshold == 0 {
			r.CircuitBreaker.FailureThreshold = 3
		}

		if r.CircuitBreaker.OpenDurationSeconds == 0 {
			r.CircuitBreaker.OpenDurationSeconds = 30
		}

		if r.CircuitBreaker.HalfOpenRequests == 0 {
			r.CircuitBreaker.HalfOpenRequests = 1
		}

		for j := range r.Upstreams {
			if r.Upstreams[j].Weight <= 0 {
				r.Upstreams[j].Weight = 1
			}
		}
	}
}
