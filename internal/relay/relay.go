package relay

// RelayState represents whether the relay is open (power cut) or closed (power on).
type RelayState int

const (
	RelayOpen   RelayState = iota // power cut to router
	RelayClosed                   // power restored to router
)

func (s RelayState) String() string {
	if s == RelayOpen {
		return "open"
	}
	return "closed"
}

// RelayController abstracts GPIO relay operations. The production implementation
// uses Linux sysfs; a mock is provided for testing without hardware.
type RelayController interface {
	// Open cuts power to the router (relay open = circuit broken).
	Open() error
	// Close restores power to the router (relay closed = circuit complete).
	Close() error
	// State returns the current relay state.
	State() RelayState
}
