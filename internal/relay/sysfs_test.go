package relay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSysfs creates a minimal fake GPIO sysfs tree under t.TempDir() for pin N:
//
//	<base>/export
//	<base>/gpioN/direction
//	<base>/gpioN/value
//
// Returns the base path.
func fakeSysfs(t *testing.T, pin int) string {
	t.Helper()
	base := t.TempDir()
	pinDir := filepath.Join(base, fmt.Sprintf("gpio%d", pin))
	if err := os.MkdirAll(pinDir, 0o755); err != nil {
		t.Fatalf("create pin dir: %v", err)
	}
	for _, name := range []string{"export", filepath.Join(fmt.Sprintf("gpio%d", pin), "direction"), filepath.Join(fmt.Sprintf("gpio%d", pin), "value")} {
		if err := os.WriteFile(filepath.Join(base, name), nil, 0o644); err != nil {
			t.Fatalf("create sysfs file %s: %v", name, err)
		}
	}
	return base
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// --- Construction ---

func TestSysfsRelay_New_WritesExport(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	_, err := newSysfsRelayAt(pin, true, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := readFile(t, filepath.Join(base, "export"))
	if got != "17" {
		t.Errorf("expected export file to contain \"17\", got %q", got)
	}
}

func TestSysfsRelay_New_SetsDirectionOut(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	_, err := newSysfsRelayAt(pin, true, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := readFile(t, filepath.Join(base, fmt.Sprintf("gpio%d", pin), "direction"))
	if got != "out" {
		t.Errorf("expected direction \"out\", got %q", got)
	}
}

func TestSysfsRelay_New_InitialStateIsClosed(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	r, err := newSysfsRelayAt(pin, true, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.State() != RelayClosed {
		t.Errorf("expected initial state RelayClosed, got %v", r.State())
	}
}

func TestSysfsRelay_ImplementsInterface(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, true, base)
	var _ RelayController = r
}

// --- Open / Close with active-low polarity ---

func TestSysfsRelay_Open_ActiveLow_WritesZero(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, true, base)

	if err := r.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	got := readFile(t, filepath.Join(base, fmt.Sprintf("gpio%d", pin), "value"))
	if got != "0" {
		t.Errorf("active-low Open: expected value \"0\", got %q", got)
	}
	if r.State() != RelayOpen {
		t.Errorf("expected state RelayOpen after Open(), got %v", r.State())
	}
}

func TestSysfsRelay_Close_ActiveLow_WritesOne(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, true, base)
	_ = r.Open()

	if err := r.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	got := readFile(t, filepath.Join(base, fmt.Sprintf("gpio%d", pin), "value"))
	if got != "1" {
		t.Errorf("active-low Close: expected value \"1\", got %q", got)
	}
	if r.State() != RelayClosed {
		t.Errorf("expected state RelayClosed after Close(), got %v", r.State())
	}
}

// --- Open / Close with active-high polarity ---

func TestSysfsRelay_Open_ActiveHigh_WritesOne(t *testing.T) {
	const pin = 18
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, false, base) // activeLow=false

	if err := r.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	got := readFile(t, filepath.Join(base, fmt.Sprintf("gpio%d", pin), "value"))
	if got != "1" {
		t.Errorf("active-high Open: expected value \"1\", got %q", got)
	}
}

func TestSysfsRelay_Close_ActiveHigh_WritesZero(t *testing.T) {
	const pin = 18
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, false, base)
	_ = r.Open()

	if err := r.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	got := readFile(t, filepath.Join(base, fmt.Sprintf("gpio%d", pin), "value"))
	if got != "0" {
		t.Errorf("active-high Close: expected value \"0\", got %q", got)
	}
}

// --- Open/Close toggle sequence ---

func TestSysfsRelay_ToggleSequence(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, true, base)
	valuePath := filepath.Join(base, fmt.Sprintf("gpio%d", pin), "value")

	_ = r.Open()
	if readFile(t, valuePath) != "0" {
		t.Error("after Open: expected \"0\"")
	}
	_ = r.Close()
	if readFile(t, valuePath) != "1" {
		t.Error("after Close: expected \"1\"")
	}
	_ = r.Open()
	if readFile(t, valuePath) != "0" {
		t.Error("after second Open: expected \"0\"")
	}
}

// --- Error handling ---

func TestSysfsRelay_Open_ErrorWhenValueUnwritable(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, true, base)

	// Make the value file unwritable.
	valuePath := filepath.Join(base, fmt.Sprintf("gpio%d", pin), "value")
	os.Chmod(valuePath, 0o444)
	t.Cleanup(func() { os.Chmod(valuePath, 0o644) })

	err := r.Open()
	if err == nil {
		t.Fatal("expected error when value file is unwritable")
	}
	if !strings.Contains(err.Error(), "relay open") {
		t.Errorf("error should mention \"relay open\", got: %v", err)
	}
	// State must not change on error.
	if r.State() != RelayClosed {
		t.Errorf("state should remain RelayClosed on error, got %v", r.State())
	}
}

func TestSysfsRelay_Close_ErrorWhenValueUnwritable(t *testing.T) {
	const pin = 17
	base := fakeSysfs(t, pin)
	r, _ := newSysfsRelayAt(pin, true, base)
	_ = r.Open()

	valuePath := filepath.Join(base, fmt.Sprintf("gpio%d", pin), "value")
	os.Chmod(valuePath, 0o444)
	t.Cleanup(func() { os.Chmod(valuePath, 0o644) })

	err := r.Close()
	if err == nil {
		t.Fatal("expected error when value file is unwritable")
	}
	if !strings.Contains(err.Error(), "relay close") {
		t.Errorf("error should mention \"relay close\", got: %v", err)
	}
	if r.State() != RelayOpen {
		t.Errorf("state should remain RelayOpen on error, got %v", r.State())
	}
}

func TestSysfsRelay_New_ErrorWhenExportMissing(t *testing.T) {
	base := t.TempDir() // no export file, no pin directory
	_, err := newSysfsRelayAt(17, true, base)
	if err == nil {
		t.Fatal("expected error when export file is missing")
	}
}
