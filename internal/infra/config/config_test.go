package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestConfig writes a minimal YAML config to a temp file and returns its path.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestLoad_DevelopmentSSLModeDisable(t *testing.T) {
	path := writeTestConfig(t, `
app:
  environment: development
database:
  sslmode: disable
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error in development with sslmode=disable, got: %v", err)
	}
	if cfg.Database.SSLMode != "disable" {
		t.Errorf("expected sslmode disable, got %q", cfg.Database.SSLMode)
	}
	if cfg.App.Environment != "development" {
		t.Errorf("expected environment development, got %q", cfg.App.Environment)
	}
}

func TestLoad_ProductionSSLModeDisable_Rejected(t *testing.T) {
	path := writeTestConfig(t, `
app:
  environment: production
database:
  sslmode: disable
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error when production + sslmode=disable, got nil")
	}
}

func TestLoad_ProductionSSLModeRequire_OK(t *testing.T) {
	path := writeTestConfig(t, `
app:
  environment: production
database:
  sslmode: require
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error with sslmode=require in production, got: %v", err)
	}
	if cfg.Database.SSLMode != "require" {
		t.Errorf("expected sslmode require, got %q", cfg.Database.SSLMode)
	}
}

func TestLoad_ProductionSSLModeVerifyFull_OK(t *testing.T) {
	path := writeTestConfig(t, `
app:
  environment: production
database:
  sslmode: verify-full
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error with sslmode=verify-full in production, got: %v", err)
	}
	if cfg.Database.SSLMode != "verify-full" {
		t.Errorf("expected sslmode verify-full, got %q", cfg.Database.SSLMode)
	}
}

func TestLoad_DefaultEnvironmentDevelopment(t *testing.T) {
	// No app.environment in config — should default to "development"
	path := writeTestConfig(t, `
database:
  sslmode: disable
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error with defaults, got: %v", err)
	}
	if cfg.App.Environment != "development" {
		t.Errorf("expected default environment development, got %q", cfg.App.Environment)
	}
}

func TestValidate_ProductionEmptySSLMode_Rejected(t *testing.T) {
	cfg := &Config{
		App:      AppConfig{Environment: "production"},
		Database: DatabaseConfig{SSLMode: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty sslmode in production")
	}
}
