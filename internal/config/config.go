package config

import "github.com/spf13/viper"

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Proxy  ProxyConfig  `mapstructure:"proxy"`
	Rate   RateConfig   `mapstructure:"rate_limit"`
	Health HealthConfig `mapstructure:"health"`
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
	Name       string           `mapstructure:"name"`
	Host       string           `mapstructure:"host"`
	PathPrefix string           `mapstructure:"path_prefix"`
	Upstreams  []UpstreamConfig `mapstructure:"upstreams"`
}

type UpstreamConfig struct {
	URL    string `mapstructure:"url"`
	Weight int    `mapstructure:"weight"`
}

type RateConfig struct {
	RequestsPerSecond int `mapstructure:"requests_per_second"`
	Burst             int `mapstructure:"burst"`
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

func applyDefaults(cfg *Config) {
	for i := range cfg.Proxy.Routes {
		if cfg.Proxy.Routes[i].PathPrefix == "" {
			cfg.Proxy.Routes[i].PathPrefix = "/"
		}

		for j := range cfg.Proxy.Routes[i].Upstreams {
			if cfg.Proxy.Routes[i].Upstreams[j].Weight <= 0 {
				cfg.Proxy.Routes[i].Upstreams[j].Weight = 1
			}
		}
	}
}
