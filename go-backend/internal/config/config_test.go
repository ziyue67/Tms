package config

import "testing"

func TestLoadLegacyEnvironment(t *testing.T) {
	env := map[string]string{
		"JWT_SECRET":  "secret",
		"DB_HOST":     "mysql",
		"DB_NAME":     "gost",
		"DB_USER":     "gost",
		"DB_PASSWORD": "password",
	}
	cfg, err := load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ListenAddress() != "0.0.0.0:6365" {
		t.Fatalf("unexpected listen address: %s", cfg.ListenAddress())
	}
	if cfg.Database.Port != 3306 || cfg.Redis.Port != 6379 {
		t.Fatalf("legacy defaults were not preserved: db=%d redis=%d", cfg.Database.Port, cfg.Redis.Port)
	}
}

func TestLoadRejectsMissingSecrets(t *testing.T) {
	_, err := load(func(string) string { return "" })
	if err == nil || err.Error() != "JWT_SECRET is required" {
		t.Fatalf("expected JWT secret error, got %v", err)
	}
}
