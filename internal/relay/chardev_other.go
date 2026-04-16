//go:build !linux

package relay

import "errors"

// ChardevRelay is unsupported on non-Linux platforms.
type ChardevRelay struct{}

func NewChardevRelay(_ int, _ bool) (*ChardevRelay, error) {
	return nil, errors.New("GPIO character device not supported on this platform")
}

func (r *ChardevRelay) Open() error       { return errors.New("not supported") }
func (r *ChardevRelay) Close() error      { return errors.New("not supported") }
func (r *ChardevRelay) State() RelayState { return RelayClosed }
