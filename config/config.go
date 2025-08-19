package config

import (
	"gopkg.in/yaml.v2"
	"os"
	"strings"
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
		HTTPProxy  string        `yaml:"http_proxy"`  // Global HTTP proxy
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
	Path             string            `yaml:"path"`
	TargetURL        string            `yaml:"target_url"`        // Supports comma-separated URLs
	TargetURLs       []string          // Parsed URLs from TargetURL (internal use)
	HealthCheckPath  string            `yaml:"health_check_path"` // Health check endpoint
	HealthCheckDelay int               `yaml:"health_check_delay"` // Health check interval in seconds
	Methods          []string          `yaml:"methods"`
	Headers          map[string]string `yaml:"headers"`
	HTTPProxy        string            `yaml:"http_proxy"` // Target-specific HTTP proxy
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
	processTargetURLs(&config)
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

// processTargetURLs processes comma-separated target_url field into target_urls array
func processTargetURLs(config *Config) {
	for i := range config.Proxy.Targets {
		target := &config.Proxy.Targets[i]
		
		// Parse target_url field (supports comma-separated URLs)
		if target.TargetURL != "" {
			if strings.Contains(target.TargetURL, ",") {
				// Multiple URLs separated by commas
				urls := strings.Split(target.TargetURL, ",")
				for _, url := range urls {
					trimmed := strings.TrimSpace(url)
					if trimmed != "" {
						target.TargetURLs = append(target.TargetURLs, trimmed)
					}
				}
				// Keep the first URL as the primary target_url
				if len(target.TargetURLs) > 0 {
					target.TargetURL = target.TargetURLs[0]
				}
			} else {
				// Single URL case
				target.TargetURLs = []string{target.TargetURL}
			}
		}
		
		// Set default health check path and delay
		if target.HealthCheckPath == "" {
			// For API endpoints, try to detect a reasonable health check path
			// If the target looks like an API endpoint, use empty path (let health checker try different paths)
			if strings.Contains(target.TargetURL, "api") {
				target.HealthCheckPath = "" // Let health checker auto-detect
			} else {
				target.HealthCheckPath = "/"
			}
		}
		if target.HealthCheckDelay == 0 {
			target.HealthCheckDelay = 30 // 30 seconds default
		}
	}
}
