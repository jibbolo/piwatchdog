package relay

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	gpioHandlesMax          = 64
	gpiohandleRequestOutput = uint32(1 << 1)
)

// gpiohandleRequest mirrors struct gpiohandle_request from linux/gpio.h.
type gpiohandleRequest struct {
	LineOffsets   [gpioHandlesMax]uint32
	Flags         uint32
	DefaultValues [gpioHandlesMax]uint8
	ConsumerLabel [32]byte
	Lines         uint32
	Fd            int32
}

// gpiohandleData mirrors struct gpiohandle_data from linux/gpio.h.
type gpiohandleData struct {
	Values [gpioHandlesMax]uint8
}

// iowr computes a Linux _IOWR ioctl number (dir=3, read+write).
func iowr(t, nr, size uintptr) uintptr {
	return (3 << 30) | (t << 8) | nr | (size << 16)
}

// ioctlFn abstracts the ioctl syscall; replaceable in tests.
type ioctlFn func(fd, ioc uintptr, arg unsafe.Pointer) syscall.Errno

func realIoctl(fd, ioc uintptr, arg unsafe.Pointer) syscall.Errno {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioc, uintptr(arg))
	return errno
}

// ChardevRelay controls a GPIO relay via the Linux GPIO character device (/dev/gpiochip0).
// This is the modern approach required on Raspberry Pi OS Bookworm and later kernels
// where the legacy sysfs GPIO interface is unavailable.
type ChardevRelay struct {
	gpioPin   int
	activeLow bool
	state     RelayState
	lineFd    int
	doIoctl   ioctlFn
}

// NewChardevRelay requests exclusive output control of gpioPin via /dev/gpiochip0.
func NewChardevRelay(gpioPin int, activeLow bool) (*ChardevRelay, error) {
	return newChardevRelayAt(gpioPin, activeLow, "/dev/gpiochip0", realIoctl)
}

// newChardevRelayAt is the internal constructor used by tests to inject a fake
// chip path and mock ioctl.
func newChardevRelayAt(gpioPin int, activeLow bool, chipPath string, ioctl ioctlFn) (*ChardevRelay, error) {
	chip, err := os.Open(chipPath)
	if err != nil {
		return nil, fmt.Errorf("gpio open %s: %w", chipPath, err)
	}
	defer chip.Close()

	req := gpiohandleRequest{
		Lines: 1,
		Flags: gpiohandleRequestOutput,
	}
	req.LineOffsets[0] = uint32(gpioPin)
	copy(req.ConsumerLabel[:], "piwatchdog")
	req.DefaultValues[0] = chardevOnValue(activeLow) // start with relay closed (power on)

	ioc := iowr(0xB4, 0x03, unsafe.Sizeof(req)) // GPIO_GET_LINEHANDLE_IOCTL
	if errno := ioctl(chip.Fd(), ioc, unsafe.Pointer(&req)); errno != 0 {
		return nil, fmt.Errorf("gpio request line %d: %w", gpioPin, errno)
	}

	return &ChardevRelay{
		gpioPin:   gpioPin,
		activeLow: activeLow,
		state:     RelayClosed,
		lineFd:    int(req.Fd),
		doIoctl:   ioctl,
	}, nil
}

func (r *ChardevRelay) Open() error {
	if err := r.setLine(chardevOffValue(r.activeLow)); err != nil {
		return fmt.Errorf("relay open: %w", err)
	}
	r.state = RelayOpen
	return nil
}

func (r *ChardevRelay) Close() error {
	if err := r.setLine(chardevOnValue(r.activeLow)); err != nil {
		return fmt.Errorf("relay close: %w", err)
	}
	r.state = RelayClosed
	return nil
}

func (r *ChardevRelay) State() RelayState {
	return r.state
}

func (r *ChardevRelay) setLine(val uint8) error {
	var data gpiohandleData
	data.Values[0] = val
	ioc := iowr(0xB4, 0x09, unsafe.Sizeof(data)) // GPIOHANDLE_SET_LINE_VALUES_IOCTL
	if errno := r.doIoctl(uintptr(r.lineFd), ioc, unsafe.Pointer(&data)); errno != 0 {
		return fmt.Errorf("gpio set value pin %d: %w", r.gpioPin, errno)
	}
	return nil
}

// chardevOnValue returns the GPIO level that keeps the relay closed (power on).
func chardevOnValue(activeLow bool) uint8 {
	if activeLow {
		return 1 // active-low: high = relay off = power on
	}
	return 0 // active-high: low = relay off = power on
}

// chardevOffValue returns the GPIO level that opens the relay (cuts power).
func chardevOffValue(activeLow bool) uint8 {
	if activeLow {
		return 0 // active-low: low = relay on = power cut
	}
	return 1 // active-high: high = relay on = power cut
}
