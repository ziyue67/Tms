package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host      string
	Port      int
	JWTSecret string
	Database  Database
	Redis     Redis
}

type Database struct {
	Driver          string
	URL             string
	Host            string
	Port            int
	Name            string
	User            string
	Password        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type Redis struct {
	URL      string
	Host     string
	Port     int
	Password string
	Database int
	TLS      bool
	Timeout  time.Duration
}

func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	dbDriver := value(getenv, "DB_DRIVER", "com.mysql.cj.jdbc.Driver")
	dbPortDefault := 3306
	if strings.Contains(strings.ToLower(dbDriver+getenv("DB_URL")), "postgres") {
		dbPortDefault = 5432
	}

	cfg := Config{
		Host:      value(getenv, "SERVER_HOST", "0.0.0.0"),
		Port:      intValue(getenv, "SERVER_PORT", 6365),
		JWTSecret: strings.TrimSpace(getenv("JWT_SECRET")),
		Database: Database{
			Driver:          dbDriver,
			URL:             strings.TrimSpace(getenv("DB_URL")),
			Host:            value(getenv, "DB_HOST", "localhost"),
			Port:            intValue(getenv, "DB_PORT", dbPortDefault),
			Name:            value(getenv, "DB_NAME", "gost"),
			User:            strings.TrimSpace(getenv("DB_USER")),
			Password:        getenv("DB_PASSWORD"),
			MaxOpenConns:    intValue(getenv, "DB_MAX_OPEN_CONNS", 20),
			MaxIdleConns:    intValue(getenv, "DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: durationValue(getenv, "DB_CONN_MAX_LIFETIME", 500*time.Second),
		},
		Redis: Redis{
			URL:      strings.TrimSpace(getenv("REDIS_URL")),
			Host:     value(getenv, "REDIS_HOST", "localhost"),
			Port:     intValue(getenv, "REDIS_PORT", 6379),
			Password: getenv("REDIS_PASSWORD"),
			Database: intValue(getenv, "REDIS_DATABASE", 0),
			TLS:      boolValue(getenv, "REDIS_SSL", false),
			Timeout:  durationValue(getenv, "REDIS_TIMEOUT", 2*time.Second),
		},
	}

	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.Database.User == "" {
		return Config{}, errors.New("DB_USER is required")
	}
	if cfg.Database.Password == "" {
		return Config{}, errors.New("DB_PASSWORD is required")
	}
	if cfg.Database.Name == "" && cfg.Database.URL == "" {
		return Config{}, errors.New("DB_NAME is required when DB_URL is empty")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("SERVER_PORT must be between 1 and 65535: %d", cfg.Port)
	}
	return cfg, nil
}

func (c Config) ListenAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func value(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}

func intValue(getenv func(string) string, key string, fallback int) int {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolValue(getenv func(string) string, key string, fallback bool) bool {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationValue(getenv func(string) string, key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
