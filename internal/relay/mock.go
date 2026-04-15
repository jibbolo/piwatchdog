package relay

// MockRelay is a test double for RelayController. It records all calls and
// supports injecting errors for fault-injection tests.
type MockRelay struct {
	// OpenErr is returned by Open() if non-nil.
	OpenErr error
	// CloseErr is returned by Close() if non-nil.
	CloseErr error

	state RelayState
	Calls []string
}

func NewMockRelay() *MockRelay {
	return &MockRelay{state: RelayClosed}
}

func (m *MockRelay) Open() error {
	m.Calls = append(m.Calls, "Open")
	if m.OpenErr != nil {
		return m.OpenErr
	}
	m.state = RelayOpen
	return nil
}

func (m *MockRelay) Close() error {
	m.Calls = append(m.Calls, "Close")
	if m.CloseErr != nil {
		return m.CloseErr
	}
	m.state = RelayClosed
	return nil
}

func (m *MockRelay) State() RelayState {
	return m.state
}

// OpenCount returns how many times Open() was called.
func (m *MockRelay) OpenCount() int {
	n := 0
	for _, c := range m.Calls {
		if c == "Open" {
			n++
		}
	}
	return n
}
