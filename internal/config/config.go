package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all operational parameters loaded from the YAML config file.
type Config struct {
	CheckInterval     string         `yaml:"check_interval"`
	EvidenceWindow    string         `yaml:"evidence_window"`
	Targets           []string       `yaml:"targets"`
	Relay             RelayConfig    `yaml:"relay"`
	RecoveryWindow    string         `yaml:"recovery_window"`
	Retry             RetryConfig    `yaml:"retry"`
	DeepSleepInterval string         `yaml:"deep_sleep_interval"`
	Notifications     NotifyConfig   `yaml:"notifications"`
	Log               LogConfig      `yaml:"log"`

	// Parsed durations — populated by Validate.
	CheckIntervalD     time.Duration
	EvidenceWindowD    time.Duration
	RecoveryWindowD    time.Duration
	DeepSleepIntervalD time.Duration
}

type RelayConfig struct {
	GPIOPin     int    `yaml:"gpio_pin"`
	OffDuration string `yaml:"off_duration"`
	ActiveLow   bool   `yaml:"active_low"`

	OffDurationD time.Duration
}

type RetryConfig struct {
	MaxCount    int     `yaml:"max_count"`
	BaseBackoff string  `yaml:"base_backoff"`
	Multiplier  float64 `yaml:"multiplier"`

	BaseBackoffD time.Duration
}

type NotifyConfig struct {
	NtfyURL string `yaml:"ntfy_url"`
	Topic   string `yaml:"topic"`
	Token   string `yaml:"token"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Load reads the YAML file at path, applies defaults, and validates.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	cfg := defaults()
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		CheckInterval:     "60s",
		EvidenceWindow:    "5m",
		RecoveryWindow:    "3m",
		DeepSleepInterval: "8h",
		Relay: RelayConfig{
			OffDuration: "5s",
			ActiveLow:   true,
		},
		Retry: RetryConfig{
			MaxCount:    5,
			BaseBackoff: "5m",
			Multiplier:  2.0,
		},
		Notifications: NotifyConfig{
			NtfyURL: "https://ntfy.sh",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Validate checks required fields and parses all duration strings.
func (c *Config) Validate() error {
	if len(c.Targets) == 0 {
		return fmt.Errorf("targets: at least one target is required")
	}
	if c.Relay.GPIOPin <= 0 {
		return fmt.Errorf("relay.gpio_pin: must be a positive integer")
	}
	if c.Retry.Multiplier <= 0 {
		return fmt.Errorf("retry.multiplier: must be > 0")
	}

	var err error
	if c.CheckIntervalD, err = time.ParseDuration(c.CheckInterval); err != nil {
		return fmt.Errorf("check_interval: %w", err)
	}
	if c.EvidenceWindowD, err = time.ParseDuration(c.EvidenceWindow); err != nil {
		return fmt.Errorf("evidence_window: %w", err)
	}
	if c.RecoveryWindowD, err = time.ParseDuration(c.RecoveryWindow); err != nil {
		return fmt.Errorf("recovery_window: %w", err)
	}
	if c.DeepSleepIntervalD, err = time.ParseDuration(c.DeepSleepInterval); err != nil {
		return fmt.Errorf("deep_sleep_interval: %w", err)
	}
	if c.Relay.OffDurationD, err = time.ParseDuration(c.Relay.OffDuration); err != nil {
		return fmt.Errorf("relay.off_duration: %w", err)
	}
	if c.Retry.BaseBackoffD, err = time.ParseDuration(c.Retry.BaseBackoff); err != nil {
		return fmt.Errorf("retry.base_backoff: %w", err)
	}
	return nil
}
