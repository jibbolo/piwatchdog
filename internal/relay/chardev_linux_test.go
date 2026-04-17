package relay

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// mockIoctl intercepts GPIO_GET_LINEHANDLE_IOCTL and GPIOHANDLE_SET_LINE_VALUES_IOCTL.
// On the linehandle request it writes fakeFd into the response and records the request
// fields so tests can assert the correct arguments were sent to the kernel.
type mockIoctl struct {
	fakeFd      int32
	initErr     syscall.Errno // returned on GPIO_GET_LINEHANDLE_IOCTL
	setErr      syscall.Errno // returned on GPIOHANDLE_SET_LINE_VALUES_IOCTL
	capturedReq gpiohandleRequest
	lastValue   uint8
}

func (m *mockIoctl) fn() ioctlFn {
	lineHandleIoc := iowr(0xB4, 0x03, unsafe.Sizeof(gpiohandleRequest{}))
	setValuesIoc := iowr(0xB4, 0x09, unsafe.Sizeof(gpiohandleData{}))
	return func(fd, ioc uintptr, arg unsafe.Pointer) syscall.Errno {
		switch ioc {
		case lineHandleIoc:
			req := (*gpiohandleRequest)(arg)
			m.capturedReq = *req
			req.Fd = m.fakeFd
			return m.initErr
		case setValuesIoc:
			m.lastValue = (*gpiohandleData)(arg).Values[0]
			return m.setErr
		}
		return 0
	}
}

// fakeChip returns a path to a temporary readable file that stands in for
// /dev/gpiochip0. The mock ioctl ignores the actual fd value.
func fakeChip(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "gpiochip")
	if err != nil {
		t.Fatalf("create fake chip: %v", err)
	}
	f.Close()
	return f.Name()
}

// --- Construction ---

func TestChardevRelay_New_InitialStateIsClosed(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, err := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.State() != RelayClosed {
		t.Errorf("expected initial state RelayClosed, got %v", r.State())
	}
}

func TestChardevRelay_ImplementsInterface(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, _ := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	var _ RelayController = r
}

func TestChardevRelay_New_SendsCorrectPin(t *testing.T) {
	const pin = 14
	m := &mockIoctl{fakeFd: 10}
	_, err := newChardevRelayAt(pin, false, fakeChip(t), m.fn())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.capturedReq.LineOffsets[0] != pin {
		t.Errorf("expected LineOffsets[0]=%d, got %d", pin, m.capturedReq.LineOffsets[0])
	}
	if m.capturedReq.Lines != 1 {
		t.Errorf("expected Lines=1, got %d", m.capturedReq.Lines)
	}
}

func TestChardevRelay_New_RequestsOutputFlag(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	_, err := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.capturedReq.Flags&gpiohandleRequestOutput == 0 {
		t.Errorf("expected OUTPUT flag set, got flags=0x%x", m.capturedReq.Flags)
	}
}

func TestChardevRelay_New_ConsumerLabel(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	_, err := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	label := strings.TrimRight(string(m.capturedReq.ConsumerLabel[:]), "\x00")
	if label != "piwatchdog" {
		t.Errorf("expected consumer label \"piwatchdog\", got %q", label)
	}
}

func TestChardevRelay_New_DefaultValue_ActiveHigh(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	_, err := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// active-high: default (relay closed/power on) = GPIO low (0)
	if m.capturedReq.DefaultValues[0] != 0 {
		t.Errorf("active-high: expected default value 0, got %d", m.capturedReq.DefaultValues[0])
	}
}

func TestChardevRelay_New_DefaultValue_ActiveLow(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	_, err := newChardevRelayAt(14, true, fakeChip(t), m.fn())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// active-low: default (relay closed/power on) = GPIO high (1)
	if m.capturedReq.DefaultValues[0] != 1 {
		t.Errorf("active-low: expected default value 1, got %d", m.capturedReq.DefaultValues[0])
	}
}

// --- Open / Close with active-high polarity ---

func TestChardevRelay_Open_ActiveHigh_SetsOne(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, _ := newChardevRelayAt(14, false, fakeChip(t), m.fn())

	if err := r.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if m.lastValue != 1 {
		t.Errorf("active-high Open: expected value 1, got %d", m.lastValue)
	}
	if r.State() != RelayOpen {
		t.Errorf("expected state RelayOpen after Open(), got %v", r.State())
	}
}

func TestChardevRelay_Close_ActiveHigh_SetsZero(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, _ := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	_ = r.Open()

	if err := r.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if m.lastValue != 0 {
		t.Errorf("active-high Close: expected value 0, got %d", m.lastValue)
	}
	if r.State() != RelayClosed {
		t.Errorf("expected state RelayClosed after Close(), got %v", r.State())
	}
}

// --- Open / Close with active-low polarity ---

func TestChardevRelay_Open_ActiveLow_SetsZero(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, _ := newChardevRelayAt(14, true, fakeChip(t), m.fn())

	if err := r.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if m.lastValue != 0 {
		t.Errorf("active-low Open: expected value 0, got %d", m.lastValue)
	}
	if r.State() != RelayOpen {
		t.Errorf("expected state RelayOpen after Open(), got %v", r.State())
	}
}

func TestChardevRelay_Close_ActiveLow_SetsOne(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, _ := newChardevRelayAt(14, true, fakeChip(t), m.fn())
	_ = r.Open()

	if err := r.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if m.lastValue != 1 {
		t.Errorf("active-low Close: expected value 1, got %d", m.lastValue)
	}
	if r.State() != RelayClosed {
		t.Errorf("expected state RelayClosed after Close(), got %v", r.State())
	}
}

// --- Toggle sequence ---

func TestChardevRelay_ToggleSequence(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, _ := newChardevRelayAt(14, false, fakeChip(t), m.fn())

	_ = r.Open()
	if m.lastValue != 1 {
		t.Errorf("after Open: expected 1, got %d", m.lastValue)
	}
	_ = r.Close()
	if m.lastValue != 0 {
		t.Errorf("after Close: expected 0, got %d", m.lastValue)
	}
	_ = r.Open()
	if m.lastValue != 1 {
		t.Errorf("after second Open: expected 1, got %d", m.lastValue)
	}
}

// --- Error handling ---

func TestChardevRelay_New_ErrorWhenChipMissing(t *testing.T) {
	m := &mockIoctl{}
	_, err := newChardevRelayAt(14, false, "/nonexistent/gpiochip0", m.fn())
	if err == nil {
		t.Fatal("expected error when chip path does not exist")
	}
}

func TestChardevRelay_New_ErrorWhenIoctlFails(t *testing.T) {
	m := &mockIoctl{initErr: syscall.ENODEV}
	_, err := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	if err == nil {
		t.Fatal("expected error when linehandle ioctl fails")
	}
	if !strings.Contains(err.Error(), "gpio request line") {
		t.Errorf("error should mention \"gpio request line\", got: %v", err)
	}
}

func TestChardevRelay_Open_ErrorPropagated(t *testing.T) {
	m := &mockIoctl{fakeFd: 10, setErr: syscall.EIO}
	r, _ := newChardevRelayAt(14, false, fakeChip(t), m.fn())

	err := r.Open()
	if err == nil {
		t.Fatal("expected error when set-values ioctl fails")
	}
	if !strings.Contains(err.Error(), "relay open") {
		t.Errorf("error should mention \"relay open\", got: %v", err)
	}
	// State must not change on error.
	if r.State() != RelayClosed {
		t.Errorf("state should remain RelayClosed on error, got %v", r.State())
	}
}

func TestChardevRelay_Close_ErrorPropagated(t *testing.T) {
	m := &mockIoctl{fakeFd: 10}
	r, _ := newChardevRelayAt(14, false, fakeChip(t), m.fn())
	_ = r.Open()

	m.setErr = syscall.EIO
	err := r.Close()
	if err == nil {
		t.Fatal("expected error when set-values ioctl fails")
	}
	if !strings.Contains(err.Error(), "relay close") {
		t.Errorf("error should mention \"relay close\", got: %v", err)
	}
	// State must not change on error.
	if r.State() != RelayOpen {
		t.Errorf("state should remain RelayOpen on error, got %v", r.State())
	}
}
