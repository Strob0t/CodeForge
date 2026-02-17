// Package config provides hierarchical configuration loading for CodeForge.
// Precedence: defaults < YAML file < environment variables.
package config

import "time"

// Config holds all runtime configuration for the CodeForge core service.
type Config struct {
	Server   Server   `yaml:"server"`
	Postgres Postgres `yaml:"postgres"`
	NATS     NATS     `yaml:"nats"`
	LiteLLM  LiteLLM  `yaml:"litellm"`
	Logging  Logging  `yaml:"logging"`
	Breaker  Breaker  `yaml:"breaker"`
	Rate     Rate     `yaml:"rate"`
	Policy   Policy   `yaml:"policy"`
}

// Policy holds policy engine configuration.
type Policy struct {
	DefaultProfile string `yaml:"default_profile"`
	CustomDir      string `yaml:"custom_dir"`
}

// Server holds HTTP server configuration.
type Server struct {
	Port       string `yaml:"port"`
	CORSOrigin string `yaml:"cors_origin"`
}

// Postgres holds PostgreSQL connection configuration.
type Postgres struct {
	DSN             string        `yaml:"dsn"`
	MaxConns        int32         `yaml:"max_conns"`
	MinConns        int32         `yaml:"min_conns"`
	MaxConnLifetime time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `yaml:"max_conn_idle_time"`
	HealthCheck     time.Duration `yaml:"health_check"`
}

// NATS holds NATS JetStream configuration.
type NATS struct {
	URL string `yaml:"url"`
}

// LiteLLM holds LiteLLM proxy configuration.
type LiteLLM struct {
	URL       string `yaml:"url"`
	MasterKey string `yaml:"master_key"`
}

// Logging holds structured logging configuration.
type Logging struct {
	Level   string `yaml:"level"`
	Service string `yaml:"service"`
}

// Breaker holds circuit breaker configuration.
type Breaker struct {
	MaxFailures int           `yaml:"max_failures"`
	Timeout     time.Duration `yaml:"timeout"`
}

// Rate holds rate limiter configuration.
type Rate struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

// Defaults returns a Config with sensible default values for local development.
func Defaults() Config {
	return Config{
		Server: Server{
			Port:       "8080",
			CORSOrigin: "http://localhost:3000",
		},
		Postgres: Postgres{
			DSN:             "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=disable",
			MaxConns:        15,
			MinConns:        2,
			MaxConnLifetime: time.Hour,
			MaxConnIdleTime: 10 * time.Minute,
			HealthCheck:     time.Minute,
		},
		NATS: NATS{
			URL: "nats://localhost:4222",
		},
		LiteLLM: LiteLLM{
			URL: "http://localhost:4000",
		},
		Logging: Logging{
			Level:   "info",
			Service: "codeforge-core",
		},
		Breaker: Breaker{
			MaxFailures: 5,
			Timeout:     30 * time.Second,
		},
		Rate: Rate{
			RequestsPerSecond: 10,
			Burst:             100,
		},
		Policy: Policy{
			DefaultProfile: "headless-safe-sandbox",
		},
	}
}
