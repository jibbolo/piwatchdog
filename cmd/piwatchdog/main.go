package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jibbolo/piwatchdog/internal/checker"
	"github.com/jibbolo/piwatchdog/internal/config"
	"github.com/jibbolo/piwatchdog/internal/notifier"
	"github.com/jibbolo/piwatchdog/internal/relay"
	"github.com/jibbolo/piwatchdog/internal/watchdog"
)

func main() {
	configPath := flag.String("config", "/etc/piwatchdog/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// slog not yet configured — use plain stderr output.
		slog.Error("failed to load config", "error", err, "path", *configPath)
		os.Exit(1)
	}

	logger := buildLogger(cfg)
	slog.SetDefault(logger)

	slog.Info("piwatchdog starting",
		"config", *configPath,
		"targets", cfg.Targets,
		"gpio_pin", cfg.Relay.GPIOPin,
	)

	r, err := relay.NewSysfsRelay(cfg.Relay.GPIOPin, cfg.Relay.ActiveLow)
	if err != nil {
		slog.Error("failed to initialise relay", "error", err)
		os.Exit(1)
	}
	// Ensure router has power on startup regardless of previous state.
	if r.State() != relay.RelayClosed {
		if err := r.Close(); err != nil {
			slog.Error("failed to close relay at startup", "error", err)
			os.Exit(1)
		}
	}

	ch := checker.New(cfg.Targets, 5*time.Second, nil)
	n := buildNotifier(cfg)
	w := watchdog.New(cfg, ch, r, n)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	w.Run(ctx)
	slog.Info("piwatchdog stopped")
}

func buildNotifier(cfg *config.Config) notifier.Notifier {
	if cfg.Notifications.Topic == "" {
		slog.Info("notifications disabled (no topic configured)")
		return notifier.NoopNotifier{}
	}
	slog.Info("notifications enabled",
		"ntfy_url", cfg.Notifications.NtfyURL,
		"topic", cfg.Notifications.Topic,
	)
	return notifier.NewNtfyNotifier(
		cfg.Notifications.NtfyURL,
		cfg.Notifications.Topic,
		cfg.Notifications.Token,
	)
}

func buildLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	if cfg.Log.Format == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
