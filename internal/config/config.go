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
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
	IdleTimeout  int    `mapstructure:"idle_timeout"`
}

type ProxyConfig struct {
	Upstreams []string `mapstructure:"upstreams"`
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

	return &cfg, nil
}
