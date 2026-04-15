package checker

import (
	"net"
	"time"
)

// PingFunc checks whether a single target is reachable within the given timeout.
// The default implementation uses a TCP dial to port 80; it can be replaced in
// tests with a mock that returns a scripted sequence of results.
type PingFunc func(target string, timeout time.Duration) bool

// DefaultPing uses net.DialTimeout to probe target:80 over TCP.
// This avoids raw ICMP (which requires root) while remaining reliable for
// well-known IP addresses such as 8.8.8.8 and 1.1.1.1.
func DefaultPing(target string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(target, "80"), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Checker probes a list of targets and reports whether any of them is reachable.
type Checker struct {
	targets  []string
	timeout  time.Duration
	pingFunc PingFunc
}

// New creates a Checker. If pingFn is nil, DefaultPing is used.
func New(targets []string, timeout time.Duration, pingFn PingFunc) *Checker {
	if pingFn == nil {
		pingFn = DefaultPing
	}
	return &Checker{
		targets:  targets,
		timeout:  timeout,
		pingFunc: pingFn,
	}
}

// AnyReachable returns true if at least one configured target responds.
// It tries targets in order and short-circuits on the first success.
func (c *Checker) AnyReachable() bool {
	for _, t := range c.targets {
		if c.pingFunc(t, c.timeout) {
			return true
		}
	}
	return false
}
