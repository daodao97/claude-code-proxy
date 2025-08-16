package config

import (
	"gopkg.in/yaml.v2"
	"os"
)

type Config struct {
	Server struct {
		Port     string `yaml:"port"`
		Host     string `yaml:"host"`
		Timeouts struct {
			Read     int `yaml:"read"`
			Write    int `yaml:"write"`
			Idle     int `yaml:"idle"`
			Shutdown int `yaml:"shutdown"`
		} `yaml:"timeouts"`
	} `yaml:"server"`

	Web struct {
		Port    string `yaml:"port"`
		Enabled bool   `yaml:"enabled"`
		MaxLogs int    `yaml:"max_logs"`
	} `yaml:"web"`

	Proxy struct {
		Targets    []ProxyTarget `yaml:"targets"`
		Timeout    int           `yaml:"timeout"`
		MaxRetries int           `yaml:"max_retries"`
		RetryDelay int           `yaml:"retry_delay"` // milliseconds
	} `yaml:"proxy"`

	Logging struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"logging"`

	WebSocket struct {
		BufferSize    int `yaml:"buffer_size"`
		BroadcastSize int `yaml:"broadcast_size"`
	} `yaml:"websocket"`
}

type ProxyTarget struct {
	Path      string            `yaml:"path"`
	TargetURL string            `yaml:"target_url"`
	Methods   []string          `yaml:"methods"`
	Headers   map[string]string `yaml:"headers"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	setDefaults(&config)
	return &config, nil
}

func setDefaults(config *Config) {
	if config.Server.Port == "" {
		config.Server.Port = "9727"
	}
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.Timeouts.Read == 0 {
		config.Server.Timeouts.Read = 30
	}
	if config.Server.Timeouts.Write == 0 {
		config.Server.Timeouts.Write = 30
	}
	if config.Server.Timeouts.Idle == 0 {
		config.Server.Timeouts.Idle = 60
	}
	if config.Server.Timeouts.Shutdown == 0 {
		config.Server.Timeouts.Shutdown = 30
	}
	if config.Web.Port == "" {
		config.Web.Port = "9528"
	}
	if !config.Web.Enabled {
		config.Web.Enabled = true
	}
	if config.Web.MaxLogs == 0 {
		config.Web.MaxLogs = 1000
	}
	if config.Proxy.Timeout == 0 {
		config.Proxy.Timeout = 30
	}
	if config.Proxy.MaxRetries == 0 {
		config.Proxy.MaxRetries = 3
	}
	if config.Proxy.RetryDelay == 0 {
		config.Proxy.RetryDelay = 1000
	}
	if config.WebSocket.BufferSize == 0 {
		config.WebSocket.BufferSize = 1024
	}
	if config.WebSocket.BroadcastSize == 0 {
		config.WebSocket.BroadcastSize = 1000
	}
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
}
