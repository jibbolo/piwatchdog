package watchdog

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/jibbolo/piwatchdog/internal/checker"
	"github.com/jibbolo/piwatchdog/internal/config"
	"github.com/jibbolo/piwatchdog/internal/notifier"
	"github.com/jibbolo/piwatchdog/internal/relay"
)

// State names used in structured log output.
const (
	stateMonitoring      = "MONITORING"
	stateOutageDetected  = "OUTAGE_DETECTED"
	stateResetting       = "RESETTING"
	stateRecovering      = "RECOVERING"
	stateBackoff         = "BACKOFF"
	stateDeepSleep       = "DEEP_SLEEP"
)

// stateFunc is a function that executes one state and returns the next.
// Returning nil signals a clean shutdown (context cancelled).
type stateFunc func(ctx context.Context) stateFunc

// Watchdog orchestrates the internet watchdog state machine.
type Watchdog struct {
	cfg         *config.Config
	checker     *checker.Checker
	relay       relay.RelayController
	notifier    notifier.Notifier
	retryCount  int
	outageAt    time.Time // when all targets first went dark (for evidence window)
	outageBegin time.Time // same value — preserved for notification duration

	// OnStateChange is an optional hook called on every state transition.
	// It is intended for testing; production code leaves it nil.
	OnStateChange func(from, to string)
}

// New creates a Watchdog wiring together all subsystems.
func New(cfg *config.Config, ch *checker.Checker, r relay.RelayController, n notifier.Notifier) *Watchdog {
	return &Watchdog{
		cfg:      cfg,
		checker:  ch,
		relay:    r,
		notifier: n,
	}
}

// Run drives the state machine until ctx is cancelled.
func (w *Watchdog) Run(ctx context.Context) {
	state := w.stateMonitoring
	for state != nil {
		state = state(ctx)
	}
}

// stateMonitoring waits one check interval, then tests connectivity.
func (w *Watchdog) stateMonitoring(ctx context.Context) stateFunc {
	if !sleepCtx(ctx, w.cfg.CheckIntervalD) {
		return nil
	}
	if !w.checker.AnyReachable() {
		w.outageAt = time.Now()
		w.outageBegin = w.outageAt
		w.transition(ctx, stateMonitoring, stateOutageDetected)
		return w.stateOutageDetected
	}
	return w.stateMonitoring
}

// stateOutageDetected re-checks at each interval and waits for the evidence window.
func (w *Watchdog) stateOutageDetected(ctx context.Context) stateFunc {
	if !sleepCtx(ctx, w.cfg.CheckIntervalD) {
		return nil
	}
	if w.checker.AnyReachable() {
		w.transition(ctx, stateOutageDetected, stateMonitoring)
		return w.stateMonitoring
	}
	if time.Since(w.outageAt) >= w.cfg.EvidenceWindowD {
		w.retryCount++
		w.transition(ctx, stateOutageDetected, stateResetting)
		return w.stateResetting
	}
	return w.stateOutageDetected
}

// stateResetting toggles the relay to power-cycle the router.
func (w *Watchdog) stateResetting(ctx context.Context) stateFunc {
	if err := w.relay.Open(); err != nil {
		slog.Error("relay open failed", "error", err, "retry", w.retryCount)
	} else {
		slog.Info("relay opened", "gpio_pin", w.cfg.Relay.GPIOPin, "off_duration", w.cfg.Relay.OffDuration)
	}

	if !sleepCtx(ctx, w.cfg.Relay.OffDurationD) {
		// Context cancelled mid-reset — restore power before exiting.
		_ = w.relay.Close()
		return nil
	}

	if err := w.relay.Close(); err != nil {
		slog.Error("relay close failed", "error", err, "retry", w.retryCount)
	} else {
		slog.Info("relay closed", "gpio_pin", w.cfg.Relay.GPIOPin)
	}

	w.transition(ctx, stateResetting, stateRecovering)
	return w.stateRecovering
}

// stateRecovering waits the recovery window then checks if internet is back.
func (w *Watchdog) stateRecovering(ctx context.Context) stateFunc {
	if !sleepCtx(ctx, w.cfg.RecoveryWindowD) {
		return nil
	}
	if w.checker.AnyReachable() {
		w.notifyRecovery(false)
		w.transition(ctx, stateRecovering, stateMonitoring)
		w.retryCount = 0
		return w.stateMonitoring
	}
	if w.retryCount >= w.cfg.Retry.MaxCount {
		w.transition(ctx, stateRecovering, stateDeepSleep)
		return w.stateDeepSleep
	}
	w.transition(ctx, stateRecovering, stateBackoff)
	return w.stateBackoff
}

// stateBackoff waits an exponentially increasing interval before retrying.
func (w *Watchdog) stateBackoff(ctx context.Context) stateFunc {
	backoff := w.backoffDuration()
	slog.Info("backoff", "state", stateBackoff, "duration", backoff.String(), "retry", w.retryCount)
	if !sleepCtx(ctx, backoff) {
		return nil
	}
	w.retryCount++
	w.transition(ctx, stateBackoff, stateResetting)
	return w.stateResetting
}

// stateDeepSleep waits the long deep-sleep interval, then resumes monitoring.
func (w *Watchdog) stateDeepSleep(ctx context.Context) stateFunc {
	slog.Info("entering deep sleep",
		"state", stateDeepSleep,
		"duration", w.cfg.DeepSleepInterval,
		"retry", w.retryCount,
	)
	if !sleepCtx(ctx, w.cfg.DeepSleepIntervalD) {
		return nil
	}
	slog.Info("waking from deep sleep", "state", stateDeepSleep)
	w.retryCount = 0
	if w.checker.AnyReachable() {
		w.notifyRecovery(true)
	}
	w.transition(ctx, stateDeepSleep, stateMonitoring)
	return w.stateMonitoring
}

// transition logs a state change and calls the optional test hook.
func (w *Watchdog) transition(_ context.Context, from, to string) {
	slog.Info("state transition",
		"from", from,
		"to", to,
		"retry", w.retryCount,
		"max_retry", w.cfg.Retry.MaxCount,
	)
	if w.OnStateChange != nil {
		w.OnStateChange(from, to)
	}
}

// notifyRecovery fires the ntfy.sh notification. Failures are logged and ignored.
func (w *Watchdog) notifyRecovery(afterDeepSleep bool) {
	w.notifier.NotifyRecovery(notifier.Event{
		ResetCount:     w.retryCount,
		OutageDuration: time.Since(w.outageBegin),
		AfterDeepSleep: afterDeepSleep,
	})
}

// backoffDuration computes base × multiplier^(retryCount-1).
func (w *Watchdog) backoffDuration() time.Duration {
	exp := math.Pow(w.cfg.Retry.Multiplier, float64(w.retryCount-1))
	return time.Duration(float64(w.cfg.Retry.BaseBackoffD) * exp)
}

// sleepCtx waits for d or until ctx is cancelled.
// Returns true if the sleep completed normally, false if ctx was cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}
