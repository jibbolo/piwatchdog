package checker

import (
	"testing"
	"time"
)

func alwaysTrue(_ string, _ time.Duration) bool  { return true }
func alwaysFalse(_ string, _ time.Duration) bool { return false }

func TestAnyReachable_AllPass(t *testing.T) {
	c := New([]string{"a", "b", "c"}, time.Second, alwaysTrue)
	if !c.AnyReachable() {
		t.Error("expected true when all targets pass")
	}
}

func TestAnyReachable_AllFail(t *testing.T) {
	c := New([]string{"a", "b", "c"}, time.Second, alwaysFalse)
	if c.AnyReachable() {
		t.Error("expected false when all targets fail")
	}
}

func TestAnyReachable_FirstFails_SecondPasses(t *testing.T) {
	ping := func(target string, _ time.Duration) bool { return target == "b" }
	c := New([]string{"a", "b", "c"}, time.Second, ping)
	if !c.AnyReachable() {
		t.Error("expected true when at least one target passes")
	}
}

func TestAnyReachable_ShortCircuits(t *testing.T) {
	// Verify that no further pings are made after the first success.
	calls := 0
	ping := func(_ string, _ time.Duration) bool {
		calls++
		return true // first call succeeds
	}
	c := New([]string{"a", "b", "c"}, time.Second, ping)
	c.AnyReachable()
	if calls != 1 {
		t.Errorf("expected 1 ping call after short-circuit, got %d", calls)
	}
}

func TestAnyReachable_EmptyTargets(t *testing.T) {
	c := New([]string{}, time.Second, alwaysTrue)
	if c.AnyReachable() {
		t.Error("expected false for empty target list")
	}
}

func TestAnyReachable_PassesTimeoutToFunc(t *testing.T) {
	want := 42 * time.Millisecond
	var got time.Duration
	ping := func(_ string, timeout time.Duration) bool {
		got = timeout
		return false
	}
	c := New([]string{"x"}, want, ping)
	c.AnyReachable()
	if got != want {
		t.Errorf("expected timeout %v passed to PingFunc, got %v", want, got)
	}
}

func TestNew_NilPingUsesDefault(t *testing.T) {
	c := New([]string{"x"}, time.Second, nil)
	if c.pingFunc == nil {
		t.Error("expected DefaultPing to be set when nil is passed")
	}
}

func TestAnyReachable_PassesTargetToFunc(t *testing.T) {
	var got []string
	ping := func(target string, _ time.Duration) bool {
		got = append(got, target)
		return false
	}
	c := New([]string{"a", "b"}, time.Second, ping)
	c.AnyReachable()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("unexpected targets passed to PingFunc: %v", got)
	}
}
