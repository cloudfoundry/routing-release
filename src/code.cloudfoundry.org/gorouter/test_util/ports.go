package test_util

import (
	"net"
)

// NextAvailPort asks the OS for a free port by binding to :0, then closing
// the listener and returning the assigned port. This avoids cross-suite port
// collisions that occur when multiple suites reuse the same static port range.
func NextAvailPort() uint16 {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("NextAvailPort: " + err.Error())
	}
	defer l.Close()
	// #nosec G115 - ephemeral ports are always in uint16 range
	return uint16(l.Addr().(*net.TCPAddr).Port)
}
