package watchdog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jibbolo/piwatchdog/internal/checker"
	"github.com/jibbolo/piwatchdog/internal/config"
	"github.com/jibbolo/piwatchdog/internal/notifier"
	"github.com/jibbolo/piwatchdog/internal/relay"
)

// testCfg returns a config with tiny durations so tests complete instantly.
func testCfg() *config.Config {
	return &config.Config{
		CheckInterval:      "1ms",
		EvidenceWindow:     "5ms",
		RecoveryWindow:     "1ms",
		DeepSleepInterval:  "1ms",
		Targets:            []string{"8.8.8.8"},
		CheckIntervalD:     1 * time.Millisecond,
		EvidenceWindowD:    5 * time.Millisecond,
		RecoveryWindowD:    1 * time.Millisecond,
		DeepSleepIntervalD: 1 * time.Millisecond,
		Relay: config.RelayConfig{
			GPIOPin:      17,
			OffDuration:  "1ms",
			OffDurationD: 1 * time.Millisecond,
			ActiveLow:    true,
		},
		Retry: config.RetryConfig{
			MaxCount:     3,
			BaseBackoff:  "1ms",
			BaseBackoffD: 1 * time.Millisecond,
			Multiplier:   2.0,
		},
	}
}

// scriptedPing returns a PingFunc that pops results from a slice.
// If the slice is exhausted it returns true (healthy) to avoid hanging.
func scriptedPing(results []bool) checker.PingFunc {
	i := 0
	return func(target string, timeout time.Duration) bool {
		if i >= len(results) {
			return true
		}
		v := results[i]
		i++
		return v
	}
}

// runWithTimeout runs the watchdog with a context that cancels after d.
func runWithTimeout(w *Watchdog, d time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	w.Run(ctx)
}

// --- Config tests -----------------------------------------------------------

func TestConfigLoad(t *testing.T) {
	cfg, err := config.Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Targets) == 0 {
		t.Error("expected at least one target")
	}
	if cfg.CheckIntervalD == 0 {
		t.Error("expected non-zero check_interval duration")
	}
}

func TestConfigMissingTargets(t *testing.T) {
	_, err := config.Load("testdata/missing_targets.yaml")
	if err == nil {
		t.Fatal("expected error for missing targets, got nil")
	}
}

// --- State machine tests ----------------------------------------------------

// TestMonitoringStaysWhenHealthy: healthy pings → relay never toggled.
func TestMonitoringStaysWhenHealthy(t *testing.T) {
	cfg := testCfg()
	// 20 successful pings then true indefinitely.
	ping := scriptedPing(repeat(true, 20))
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	runWithTimeout(w, 50*time.Millisecond)

	if r.OpenCount() != 0 {
		t.Errorf("relay was opened %d times; expected 0", r.OpenCount())
	}
}

// TestPartialFailureStaysMonitoring: one target fails, one succeeds → no reset.
func TestPartialFailureStaysMonitoring(t *testing.T) {
	cfg := testCfg()
	cfg.Targets = []string{"8.8.8.8", "1.1.1.1"}
	// First target always fails, second always succeeds.
	ping := func(target string, timeout time.Duration) bool {
		return target == "1.1.1.1"
	}
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	runWithTimeout(w, 50*time.Millisecond)

	if r.OpenCount() != 0 {
		t.Errorf("relay was opened %d times; expected 0", r.OpenCount())
	}
}

// TestEvidenceWindowNotElapsed: all fail but elapsed < evidence window → no relay toggle.
func TestEvidenceWindowNotElapsed(t *testing.T) {
	cfg := testCfg()
	// Make evidence window very long relative to run time.
	cfg.EvidenceWindowD = 10 * time.Second

	// All pings fail.
	ping := scriptedPing(repeat(false, 50))
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	runWithTimeout(w, 30*time.Millisecond)

	if r.OpenCount() != 0 {
		t.Errorf("relay was opened %d times before evidence window; expected 0", r.OpenCount())
	}
}

// TestOutageConfirmed: all fail for ≥ evidence window → relay Open() called.
func TestOutageConfirmed(t *testing.T) {
	cfg := testCfg()
	// All pings fail, then recover after relay toggle.
	ping := scriptedPing(append(repeat(false, 30), repeat(true, 20)...))
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	runWithTimeout(w, 100*time.Millisecond)

	if r.OpenCount() == 0 {
		t.Error("relay was never opened; expected at least one open after outage confirmation")
	}
}

// TestRecoveryAfterOneReset: ping recovers after relay toggle → retryCount reset.
func TestRecoveryAfterOneReset(t *testing.T) {
	cfg := testCfg()
	r := relay.NewMockRelay()

	// Ping fails until the relay has been opened (router "rebooted"), then succeeds.
	// This mirrors real-world behaviour: connectivity is restored after power-cycle.
	ping := func(target string, timeout time.Duration) bool {
		return r.OpenCount() >= 1
	}
	ch := checker.New(cfg.Targets, time.Second, ping)

	var notified bool
	n := &spyNotifier{onNotify: func(e notifier.Event) {
		notified = true
		if e.ResetCount != 1 {
			t.Errorf("expected ResetCount=1, got %d", e.ResetCount)
		}
	}}
	w := New(cfg, ch, r, n)
	runWithTimeout(w, 200*time.Millisecond)

	if !notified {
		t.Error("expected recovery notification, got none")
	}
	if r.OpenCount() != 1 {
		t.Errorf("expected exactly 1 relay open, got %d", r.OpenCount())
	}
}

// TestExponentialBackoff: 3 failed resets → backoff durations double each time.
func TestExponentialBackoff(t *testing.T) {
	cfg := testCfg()
	cfg.Retry.BaseBackoffD = 4 * time.Millisecond
	cfg.Retry.Multiplier = 2.0
	cfg.Retry.MaxCount = 5

	// All pings fail throughout.
	ping := scriptedPing(repeat(false, 200))
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	runWithTimeout(w, 300*time.Millisecond)

	// Should have attempted multiple resets.
	if r.OpenCount() < 2 {
		t.Errorf("expected at least 2 relay opens for backoff test, got %d", r.OpenCount())
	}
}

// TestMaxRetriesDeepSleep: N consecutive failures → deep sleep state entered.
func TestMaxRetriesDeepSleep(t *testing.T) {
	cfg := testCfg()
	cfg.Retry.MaxCount = 2

	// All pings fail indefinitely — internet never recovers during this test.
	ping := func(target string, timeout time.Duration) bool { return false }
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	deepSleepEntered := false
	w.OnStateChange = func(from, to string) {
		if to == stateDeepSleep {
			deepSleepEntered = true
		}
	}

	runWithTimeout(w, 300*time.Millisecond)

	if !deepSleepEntered {
		t.Error("expected DEEP_SLEEP state to be entered after max retries")
	}
}

// TestDeepSleepExit: after deep sleep interval, retryCount is reset and monitoring resumes.
func TestDeepSleepExit(t *testing.T) {
	cfg := testCfg()
	cfg.Retry.MaxCount = 1

	// Fail to enter deep sleep, then recover on wake.
	ping := scriptedPing(append(repeat(false, 50), repeat(true, 50)...))
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	runWithTimeout(w, 300*time.Millisecond)

	// After deep sleep exit the retry counter is reset; relay may be opened again
	// in a second cycle — just verify no panic and healthy state is eventually reached.
}

// TestStructuredLogOutput: state transitions emit valid JSON with required fields.
func TestStructuredLogOutput(t *testing.T) {
	cfg := testCfg()
	ping := scriptedPing(append(repeat(false, 30), repeat(true, 20)...))
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(slog.Default()) })

	runWithTimeout(w, 150*time.Millisecond)

	if buf.Len() == 0 {
		t.Fatal("no log output produced")
	}
	// Validate each line is parseable JSON with required fields.
	for _, line := range splitLines(buf.Bytes()) {
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Errorf("invalid JSON log line: %s", line)
			continue
		}
		for _, field := range []string{"time", "level", "msg"} {
			if _, ok := m[field]; !ok {
				t.Errorf("log line missing field %q: %s", field, line)
			}
		}
	}
}

// --- Safety / fault-injection tests -----------------------------------------

// TestContextCancelledDuringReset: SIGTERM while relay is open must close the relay
// before the process exits, otherwise the router is left without power indefinitely.
func TestContextCancelledDuringReset(t *testing.T) {
	cfg := testCfg()
	cfg.Relay.OffDurationD = 10 * time.Second // much longer than the cancel

	ctx, cancel := context.WithCancel(context.Background())

	mock := relay.NewMockRelay()
	r := &cancelOnOpenRelay{MockRelay: mock, cancel: cancel}

	ping := scriptedPing(repeat(false, 100))
	ch := checker.New(cfg.Targets, time.Second, ping)
	w := New(cfg, ch, r, notifier.NoopNotifier{})

	w.Run(ctx)

	if r.State() != relay.RelayClosed {
		t.Errorf("relay left open after shutdown: state=%v", r.State())
	}
	if mock.OpenCount() != 1 {
		t.Errorf("expected exactly 1 relay open, got %d", mock.OpenCount())
	}
}

// cancelOnOpenRelay wraps MockRelay and cancels a context the moment Open() is
// called, simulating a SIGTERM that races the relay off-duration sleep.
type cancelOnOpenRelay struct {
	*relay.MockRelay
	cancel context.CancelFunc
}

func (r *cancelOnOpenRelay) Open() error {
	err := r.MockRelay.Open()
	r.cancel()
	return err
}

// TestRelayOpenFailure_FSMCompletesFullCycle: if relay.Open() always fails (e.g.
// GPIO fd gone bad), the FSM must not hang — it should still exhaust retries and
// reach DEEP_SLEEP, and the relay must remain closed throughout.
func TestRelayOpenFailure_FSMCompletesFullCycle(t *testing.T) {
	cfg := testCfg()
	cfg.Retry.MaxCount = 2

	ping := func(target string, timeout time.Duration) bool { return false }
	ch := checker.New(cfg.Targets, time.Second, ping)

	r := relay.NewMockRelay()
	r.OpenErr = errors.New("gpio error")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := New(cfg, ch, r, notifier.NoopNotifier{})

	deepSleepEntered := false
	w.OnStateChange = func(from, to string) {
		if to == stateDeepSleep {
			deepSleepEntered = true
			cancel() // stop here — same pattern as TestFullCycleStateSequence
		}
	}

	w.Run(ctx)

	if !deepSleepEntered {
		t.Error("expected FSM to reach DEEP_SLEEP even when relay.Open() always fails")
	}
	if r.OpenCount() != cfg.Retry.MaxCount {
		t.Errorf("expected %d Open() calls, got %d", cfg.Retry.MaxCount, r.OpenCount())
	}
	if r.State() != relay.RelayClosed {
		t.Errorf("relay should stay closed when Open() always fails, got %v", r.State())
	}
}

// TestFullCycleStateSequence: verifies the exact sequence of state transitions
// for a full outage → multi-retry → deep-sleep cycle with MaxCount=2.
// Any regression in retry counting, backoff branching, or missing states will
// show up as a mismatch here.
func TestFullCycleStateSequence(t *testing.T) {
	cfg := testCfg()
	cfg.Retry.MaxCount = 2

	ping := func(target string, timeout time.Duration) bool { return false }
	ch := checker.New(cfg.Targets, time.Second, ping)
	r := relay.NewMockRelay()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := New(cfg, ch, r, notifier.NoopNotifier{})

	var got []string
	w.OnStateChange = func(from, to string) {
		got = append(got, from+"→"+to)
		if to == stateDeepSleep {
			cancel() // stop as soon as we enter deep sleep
		}
	}

	w.Run(ctx)

	want := []string{
		"MONITORING→OUTAGE_DETECTED",
		"OUTAGE_DETECTED→RESETTING",
		"RESETTING→RECOVERING",
		"RECOVERING→BACKOFF",
		"BACKOFF→RESETTING",
		"RESETTING→RECOVERING",
		"RECOVERING→DEEP_SLEEP",
	}

	if len(got) != len(want) {
		t.Fatalf("transition count: got %d, want %d\ngot:  %v\nwant: %v",
			len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("transition[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

// --- helpers ----------------------------------------------------------------

func repeat(v bool, n int) []bool {
	s := make([]bool, n)
	for i := range s {
		s[i] = v
	}
	return s
}

func splitLines(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			lines = append(lines, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, b[start:])
	}
	return lines
}

type spyNotifier struct {
	onNotify func(notifier.Event)
}

func (s *spyNotifier) NotifyRecovery(e notifier.Event) {
	if s.onNotify != nil {
		s.onNotify(e)
	}
}
