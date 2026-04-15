package notifier

import "time"

// Event carries context for a recovery notification.
type Event struct {
	ResetCount     int
	OutageDuration time.Duration
	AfterDeepSleep bool
}

// Notifier sends push notifications on internet recovery events.
// Phase 2 will add NtfyNotifier; Phase 1 uses NoopNotifier.
type Notifier interface {
	NotifyRecovery(e Event)
}

// NoopNotifier discards all notifications. Used in Phase 1 and in tests
// that do not exercise notification behaviour.
type NoopNotifier struct{}

func (NoopNotifier) NotifyRecovery(Event) {}
