package notifier

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NtfyNotifier sends push notifications to an ntfy.sh topic (or a
// self-hosted ntfy instance) when internet connectivity is restored.
//
// Notifications are sent synchronously with a short HTTP timeout so that
// a slow or unreachable server does not block the watchdog loop for long.
type NtfyNotifier struct {
	baseURL string
	topic   string
	token   string
	client  *http.Client
}

// NewNtfyNotifier constructs a NtfyNotifier from the notification config.
// baseURL must not include a trailing slash (e.g. "https://ntfy.sh").
func NewNtfyNotifier(baseURL, topic, token string) *NtfyNotifier {
	return &NtfyNotifier{
		baseURL: strings.TrimRight(baseURL, "/"),
		topic:   topic,
		token:   token,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// NotifyRecovery sends a push notification. Failures are logged and silently
// ignored — they never propagate to the caller.
func (n *NtfyNotifier) NotifyRecovery(e Event) {
	msg := formatMessage(e)
	priority := "default"
	if e.AfterDeepSleep {
		priority = "high"
	}

	url := n.baseURL + "/" + n.topic
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(msg))
	if err != nil {
		slog.Warn("ntfy: failed to build request", "error", err)
		return
	}
	req.Header.Set("Title", "PiWatchDog")
	req.Header.Set("Priority", priority)
	req.Header.Set("Content-Type", "text/plain")
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		slog.Warn("ntfy send failed", "error", err, "url", url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("ntfy send failed", "status", resp.StatusCode, "url", url)
	}
}

func formatMessage(e Event) string {
	dur := e.OutageDuration.Round(time.Second).String()
	if e.AfterDeepSleep {
		return fmt.Sprintf("Internet restored after deep sleep. Outage duration: %s.", dur)
	}
	return fmt.Sprintf("Internet restored after %d reset(s). Outage duration: %s.", e.ResetCount, dur)
}
