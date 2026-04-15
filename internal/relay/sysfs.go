package relay

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

const gpioBasePath = "/sys/class/gpio"

// SysfsRelay controls a GPIO-connected relay via the Linux sysfs interface.
type SysfsRelay struct {
	gpioPin   int
	activeLow bool
	state     RelayState
}

// NewSysfsRelay creates a SysfsRelay and exports + configures the GPIO pin.
// activeLow=true means writing "0" opens the relay (cuts power).
func NewSysfsRelay(gpioPin int, activeLow bool) (*SysfsRelay, error) {
	r := &SysfsRelay{
		gpioPin:   gpioPin,
		activeLow: activeLow,
		state:     RelayClosed,
	}
	if err := r.export(); err != nil {
		return nil, err
	}
	if err := r.setDirection("out"); err != nil {
		return nil, err
	}
	return r, nil
}

// Open cuts power to the router by opening the relay circuit.
func (r *SysfsRelay) Open() error {
	val := r.offValue()
	if err := r.writeValue(val); err != nil {
		return fmt.Errorf("relay open: %w", err)
	}
	r.state = RelayOpen
	return nil
}

// Close restores power to the router by closing the relay circuit.
func (r *SysfsRelay) Close() error {
	val := r.onValue()
	if err := r.writeValue(val); err != nil {
		return fmt.Errorf("relay close: %w", err)
	}
	r.state = RelayClosed
	return nil
}

// State returns the current relay state.
func (r *SysfsRelay) State() RelayState {
	return r.state
}

func (r *SysfsRelay) offValue() string {
	if r.activeLow {
		return "0"
	}
	return "1"
}

func (r *SysfsRelay) onValue() string {
	if r.activeLow {
		return "1"
	}
	return "0"
}

func (r *SysfsRelay) export() error {
	path := gpioBasePath + "/export"
	err := os.WriteFile(path, []byte(strconv.Itoa(r.gpioPin)), 0o644)
	if err != nil && !errors.Is(err, os.ErrExist) {
		// EBUSY means the pin is already exported — that is fine.
		if !isEBUSY(err) {
			return fmt.Errorf("gpio export pin %d: %w", r.gpioPin, err)
		}
	}
	return nil
}

func (r *SysfsRelay) setDirection(dir string) error {
	path := fmt.Sprintf("%s/gpio%d/direction", gpioBasePath, r.gpioPin)
	if err := os.WriteFile(path, []byte(dir), 0o644); err != nil {
		return fmt.Errorf("gpio set direction pin %d: %w", r.gpioPin, err)
	}
	return nil
}

func (r *SysfsRelay) writeValue(val string) error {
	path := fmt.Sprintf("%s/gpio%d/value", gpioBasePath, r.gpioPin)
	if err := os.WriteFile(path, []byte(val), 0o644); err != nil {
		return fmt.Errorf("gpio write value pin %d: %w", r.gpioPin, err)
	}
	return nil
}

func isEBUSY(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error() == "device or resource busy"
	}
	return false
}
