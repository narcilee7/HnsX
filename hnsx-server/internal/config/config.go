// Package config provides HnsX configuration loading.
package config

// Config is the HnsX configuration.
type Config struct {
	ControlPlaneAddr string
	LogLevel         string
}

// Default returns a default configuration.
func Default() *Config {
	return &Config{
		ControlPlaneAddr: "http://127.0.0.1:50051",
		LogLevel:         "info",
	}
}
