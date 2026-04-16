package notifier

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// capture records the last request received by the mock HTTP server.
type capture struct {
	req  *http.Request
	body string
}

func mockServer(t *testing.T) (*httptest.Server, *capture) {
	t.Helper()
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.req = r
		c.body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

// TC-2.2: recovery after reset → exactly one POST with correct body.
func TestNtfyNotifyRecovery(t *testing.T) {
	srv, c := mockServer(t)

	n := NewNtfyNotifier(srv.URL, "piwatchdog", "")
	n.NotifyRecovery(Event{ResetCount: 2, OutageDuration: 14*time.Minute + 32*time.Second})

	if c.req == nil {
		t.Fatal("expected a POST to the mock server, got none")
	}
	if c.req.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", c.req.Method)
	}
	if !strings.Contains(c.body, "2 reset(s)") {
		t.Errorf("body missing reset count: %q", c.body)
	}
	if !strings.Contains(c.body, "14m32s") {
		t.Errorf("body missing outage duration: %q", c.body)
	}
	if c.req.Header.Get("Title") != "PiWatchDog" {
		t.Errorf("missing or wrong Title header: %q", c.req.Header.Get("Title"))
	}
	if c.req.Header.Get("Priority") != "default" {
		t.Errorf("expected priority=default, got %q", c.req.Header.Get("Priority"))
	}
}

// TC-2.4: recovery after deep sleep → POST with high priority and deep sleep message.
func TestNtfyNotifyRecoveryAfterDeepSleep(t *testing.T) {
	srv, c := mockServer(t)

	n := NewNtfyNotifier(srv.URL, "piwatchdog", "")
	n.NotifyRecovery(Event{ResetCount: 3, OutageDuration: 9*time.Hour + 5*time.Minute, AfterDeepSleep: true})

	if c.req == nil {
		t.Fatal("expected a POST, got none")
	}
	if !strings.Contains(c.body, "deep sleep") {
		t.Errorf("body missing 'deep sleep': %q", c.body)
	}
	if c.req.Header.Get("Priority") != "high" {
		t.Errorf("expected priority=high for deep sleep event, got %q", c.req.Header.Get("Priority"))
	}
}

// TC-2.6: token configured → Authorization header present on POST.
func TestNtfyBearerToken(t *testing.T) {
	srv, c := mockServer(t)

	n := NewNtfyNotifier(srv.URL, "piwatchdog", "secret-token")
	n.NotifyRecovery(Event{ResetCount: 1, OutageDuration: time.Minute})

	if c.req == nil {
		t.Fatal("expected a POST, got none")
	}
	auth := c.req.Header.Get("Authorization")
	if auth != "Bearer secret-token" {
		t.Errorf("expected Authorization: Bearer secret-token, got %q", auth)
	}
}

// TC-2.5: ntfy server unreachable → no panic, no crash.
func TestNtfyUnreachable(t *testing.T) {
	// Use an address that will immediately refuse connections.
	n := NewNtfyNotifier("http://127.0.0.1:1", "piwatchdog", "")
	// Must not panic.
	n.NotifyRecovery(Event{ResetCount: 1, OutageDuration: time.Minute})
}

// TC-2.1 (notifier layer): no POST when no NotifyRecovery call is made.
// This is enforced by the watchdog state machine; verified here by ensuring
// the notifier does nothing unless explicitly called.
func TestNtfyNoCallMeansNoPost(t *testing.T) {
	srv, c := mockServer(t)
	_ = NewNtfyNotifier(srv.URL, "piwatchdog", "")
	// NotifyRecovery is never called.
	if c.req != nil {
		t.Error("unexpected POST to mock server without calling NotifyRecovery")
	}
}

func TestFormatMessageReset(t *testing.T) {
	msg := formatMessage(Event{ResetCount: 1, OutageDuration: 5*time.Minute + 3*time.Second})
	if !strings.Contains(msg, "1 reset(s)") {
		t.Errorf("unexpected message: %q", msg)
	}
	if !strings.Contains(msg, "5m3s") {
		t.Errorf("expected duration in message: %q", msg)
	}
}

func TestFormatMessageDeepSleep(t *testing.T) {
	msg := formatMessage(Event{AfterDeepSleep: true, OutageDuration: 8 * time.Hour})
	if !strings.Contains(msg, "deep sleep") {
		t.Errorf("expected 'deep sleep' in message: %q", msg)
	}
}
