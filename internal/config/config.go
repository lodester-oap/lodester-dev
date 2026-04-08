// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"os"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	DatabaseURL string
	ServerPort  string
	LogLevel    string
}

// Load reads configuration from environment variables.
// DATABASE_URL is required; all others have sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		ServerPort:  getEnvOr("SERVER_PORT", "8080"),
		LogLevel:    getEnvOr("LOG_LEVEL", "info"),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
}

func getEnvOr(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
