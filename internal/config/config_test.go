package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

const validYAML = `
targets:
  - 8.8.8.8
  - 1.1.1.1
relay:
  gpio_pin: 17
  off_duration: 5s
  active_low: true
`

func TestLoad_ValidMinimal(t *testing.T) {
	cfg, err := Load(writeTemp(t, validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(cfg.Targets))
	}
	if cfg.Relay.GPIOPin != 17 {
		t.Errorf("expected gpio_pin=17, got %d", cfg.Relay.GPIOPin)
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	cfg, err := Load(writeTemp(t, validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CheckIntervalD != 60*time.Second {
		t.Errorf("expected check_interval default 60s, got %v", cfg.CheckIntervalD)
	}
	if cfg.EvidenceWindowD != 5*time.Minute {
		t.Errorf("expected evidence_window default 5m, got %v", cfg.EvidenceWindowD)
	}
	if cfg.RecoveryWindowD != 3*time.Minute {
		t.Errorf("expected recovery_window default 3m, got %v", cfg.RecoveryWindowD)
	}
	if cfg.DeepSleepIntervalD != 8*time.Hour {
		t.Errorf("expected deep_sleep_interval default 8h, got %v", cfg.DeepSleepIntervalD)
	}
	if cfg.Retry.MaxCount != 5 {
		t.Errorf("expected retry.max_count default 5, got %d", cfg.Retry.MaxCount)
	}
	if cfg.Retry.Multiplier != 2.0 {
		t.Errorf("expected retry.multiplier default 2.0, got %f", cfg.Retry.Multiplier)
	}
	if cfg.Notifications.NtfyURL != "https://ntfy.sh" {
		t.Errorf("expected ntfy_url default, got %q", cfg.Notifications.NtfyURL)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected log.level default info, got %q", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("expected log.format default json, got %q", cfg.Log.Format)
	}
}

func TestLoad_OverridesDefaults(t *testing.T) {
	y := `
targets: [8.8.8.8]
relay:
  gpio_pin: 23
check_interval: 30s
evidence_window: 2m
deep_sleep_interval: 4h
retry:
  max_count: 3
  multiplier: 3.0
`
	cfg, err := Load(writeTemp(t, y))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CheckIntervalD != 30*time.Second {
		t.Errorf("expected 30s, got %v", cfg.CheckIntervalD)
	}
	if cfg.EvidenceWindowD != 2*time.Minute {
		t.Errorf("expected 2m, got %v", cfg.EvidenceWindowD)
	}
	if cfg.DeepSleepIntervalD != 4*time.Hour {
		t.Errorf("expected 4h, got %v", cfg.DeepSleepIntervalD)
	}
	if cfg.Retry.MaxCount != 3 {
		t.Errorf("expected max_count=3, got %d", cfg.Retry.MaxCount)
	}
	if cfg.Retry.Multiplier != 3.0 {
		t.Errorf("expected multiplier=3.0, got %f", cfg.Retry.Multiplier)
	}
}

func TestLoad_AllDurationsParsed(t *testing.T) {
	y := `
targets: [1.1.1.1]
relay:
  gpio_pin: 17
  off_duration: 10s
check_interval: 45s
evidence_window: 3m
recovery_window: 2m
deep_sleep_interval: 6h
retry:
  base_backoff: 8m
`
	cfg, err := Load(writeTemp(t, y))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Relay.OffDurationD != 10*time.Second {
		t.Errorf("off_duration: expected 10s, got %v", cfg.Relay.OffDurationD)
	}
	if cfg.Retry.BaseBackoffD != 8*time.Minute {
		t.Errorf("base_backoff: expected 8m, got %v", cfg.Retry.BaseBackoffD)
	}
}

// --- Validation errors ---

func TestLoad_MissingTargets(t *testing.T) {
	y := `relay:\n  gpio_pin: 17\n`
	_, err := Load(writeTemp(t, y))
	if err == nil {
		t.Fatal("expected error for missing targets")
	}
	if !strings.Contains(err.Error(), "targets") {
		t.Errorf("error should mention 'targets', got: %v", err)
	}
}

func TestLoad_MissingGPIOPin(t *testing.T) {
	y := "targets:\n  - 8.8.8.8\n"
	_, err := Load(writeTemp(t, y))
	if err == nil {
		t.Fatal("expected error for missing gpio_pin")
	}
	if !strings.Contains(err.Error(), "gpio_pin") {
		t.Errorf("error should mention 'gpio_pin', got: %v", err)
	}
}

func TestLoad_InvalidMultiplier(t *testing.T) {
	y := "targets:\n  - 8.8.8.8\nrelay:\n  gpio_pin: 17\nretry:\n  multiplier: 0\n"
	_, err := Load(writeTemp(t, y))
	if err == nil {
		t.Fatal("expected error for zero multiplier")
	}
	if !strings.Contains(err.Error(), "multiplier") {
		t.Errorf("error should mention 'multiplier', got: %v", err)
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	y := "targets:\n  - 8.8.8.8\nrelay:\n  gpio_pin: 17\ncheck_interval: notaduration\n"
	_, err := Load(writeTemp(t, y))
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
	if !strings.Contains(err.Error(), "check_interval") {
		t.Errorf("error should mention 'check_interval', got: %v", err)
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	_, err := Load(writeTemp(t, ":::invalid yaml:::"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
