package relay

import (
	"errors"
	"testing"
)

// --- RelayState ---

func TestRelayState_String(t *testing.T) {
	if RelayOpen.String() != "open" {
		t.Errorf("expected \"open\", got %q", RelayOpen.String())
	}
	if RelayClosed.String() != "closed" {
		t.Errorf("expected \"closed\", got %q", RelayClosed.String())
	}
}

// --- MockRelay ---

func TestMockRelay_InitialState(t *testing.T) {
	r := NewMockRelay()
	if r.State() != RelayClosed {
		t.Errorf("expected initial state RelayClosed, got %v", r.State())
	}
	if len(r.Calls) != 0 {
		t.Errorf("expected no calls initially, got %v", r.Calls)
	}
}

func TestMockRelay_Open(t *testing.T) {
	r := NewMockRelay()
	if err := r.Open(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.State() != RelayOpen {
		t.Errorf("expected RelayOpen after Open(), got %v", r.State())
	}
	if len(r.Calls) != 1 || r.Calls[0] != "Open" {
		t.Errorf("unexpected call record: %v", r.Calls)
	}
}

func TestMockRelay_Close(t *testing.T) {
	r := NewMockRelay()
	_ = r.Open()
	if err := r.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.State() != RelayClosed {
		t.Errorf("expected RelayClosed after Close(), got %v", r.State())
	}
}

func TestMockRelay_CallSequence(t *testing.T) {
	r := NewMockRelay()
	_ = r.Open()
	_ = r.Close()
	_ = r.Open()
	want := []string{"Open", "Close", "Open"}
	if len(r.Calls) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, r.Calls)
	}
	for i, c := range want {
		if r.Calls[i] != c {
			t.Errorf("call[%d]: expected %q, got %q", i, c, r.Calls[i])
		}
	}
}

func TestMockRelay_OpenCount(t *testing.T) {
	r := NewMockRelay()
	_ = r.Open()
	_ = r.Close()
	_ = r.Open()
	if r.OpenCount() != 2 {
		t.Errorf("expected OpenCount=2, got %d", r.OpenCount())
	}
}

func TestMockRelay_OpenError_StateUnchanged(t *testing.T) {
	r := NewMockRelay()
	r.OpenErr = errors.New("gpio fault")
	err := r.Open()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.State() != RelayClosed {
		t.Errorf("state should not change on Open error, got %v", r.State())
	}
	// Call is still recorded even on error.
	if r.OpenCount() != 1 {
		t.Errorf("expected call to be recorded despite error, OpenCount=%d", r.OpenCount())
	}
}

func TestMockRelay_CloseError_StateUnchanged(t *testing.T) {
	r := NewMockRelay()
	_ = r.Open()
	r.CloseErr = errors.New("gpio fault")
	err := r.Close()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.State() != RelayOpen {
		t.Errorf("state should not change on Close error, got %v", r.State())
	}
}

func TestMockRelay_ImplementsInterface(t *testing.T) {
	// Compile-time check that MockRelay satisfies RelayController.
	var _ RelayController = NewMockRelay()
}
