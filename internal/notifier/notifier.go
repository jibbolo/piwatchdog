package notifier

import "time"

// Event carries context for a recovery notification.
type Event struct {
	ResetCount     int
	OutageDuration time.Duration
	AfterDeepSleep bool
}

// Notifier sends push notifications on internet recovery events.
type Notifier interface {
	NotifyRecovery(e Event)
}

// NoopNotifier discards all notifications. Used in tests that do not
// exercise notification behaviour, and when no ntfy topic is configured.
type NoopNotifier struct{}

func (NoopNotifier) NotifyRecovery(Event) {}
