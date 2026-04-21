package test_util

import (
	"fmt"
	"net"
	"sync"

	. "github.com/onsi/ginkgo/v2"
)

var (
	allocatedPorts    = make(map[uint16]bool)
	reservedListeners = make(map[uint16]net.Listener)
	portMu            sync.Mutex
)

// portRange returns the base port and size of the range reserved for the
// current Ginkgo parallel process.  It divides the port space [61000,65534]
// evenly across GinkgoConfiguration().ParallelTotal procs.
//
// Port space starts at 61000 to stay entirely above the Linux kernel's default
// ephemeral port range (32768–60999, see /proc/sys/net/ipv4/ip_local_port_range).
// Ports inside the ephemeral range can be grabbed by the OS for outgoing
// connections in the window between ReleaseAllPorts() and the moment the
// external process (gorouter) actually calls listen(), causing "address already
// in use" failures on loaded systems such as Docker VMs.
func portRange() (base, size uint16) {
	suiteConfig, _ := GinkgoConfiguration()
	total := suiteConfig.ParallelTotal
	if total <= 0 {
		total = 1
	}
	// Stay above the Linux ephemeral range (32768-60999).
	const portSpaceStart = 61000
	const portSpaceEnd = 65534
	size = uint16((portSpaceEnd - portSpaceStart) / total)
	base = portSpaceStart + uint16(GinkgoParallelProcess()-1)*size
	return
}

// nextPortInRange returns the next free port in this process's dedicated range.
// Must be called with portMu held.
func nextPortInRange() uint16 {
	base, size := portRange()
	for port := base; port < base+size; port++ {
		if allocatedPorts[port] {
			continue
		}
		l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			// Port in use by something outside our process – skip it.
			allocatedPorts[port] = true
			continue
		}
		l.Close()
		allocatedPorts[port] = true
		return port
	}
	panic(fmt.Sprintf("nextPortInRange: exhausted %d-port range starting at %d for Ginkgo proc %d", size, base, GinkgoParallelProcess()))
}

// NextAvailPort returns a free port from the current Ginkgo process's dedicated
// port range.  Using per-process ranges eliminates cross-process collisions when
// running with --nodes=N, removing the need for the ReservePort/ReleaseAllPorts
// dance for in-process port bindings.
func NextAvailPort() uint16 {
	portMu.Lock()
	defer portMu.Unlock()
	return nextPortInRange()
}

// ReservePort returns a free port and keeps the listener open so that no other
// process can grab it before the caller is ready.  Call ReleaseAllPorts to
// close all held listeners just before starting the process that will bind
// these ports.  This eliminates the TOCTOU race between port allocation and
// binding when ports are used by external processes (e.g. integration tests
// that spawn gorouter as a separate binary).
func ReservePort() uint16 {
	portMu.Lock()
	defer portMu.Unlock()

	base, size := portRange()
	for port := base; port < base+size; port++ {
		if allocatedPorts[port] {
			continue
		}
		l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			allocatedPorts[port] = true
			continue
		}
		allocatedPorts[port] = true
		reservedListeners[port] = l // keep open!
		return port
	}
	panic(fmt.Sprintf("ReservePort: exhausted %d-port range starting at %d for Ginkgo proc %d", size, base, GinkgoParallelProcess()))
}

// ReleaseAllPorts closes all listeners held by ReservePort.  Call this just
// before starting an external process that needs to bind the reserved ports.
func ReleaseAllPorts() {
	portMu.Lock()
	defer portMu.Unlock()

	for port, l := range reservedListeners {
		l.Close()
		delete(reservedListeners, port)
	}
}

// ReleasePort closes the reservation listener for a single port.  Use this
// when only one reserved port needs to be freed (e.g. before starting NATS
// while keeping the other ports reserved for the gorouter).
func ReleasePort(port uint16) {
	portMu.Lock()
	defer portMu.Unlock()

	if l, ok := reservedListeners[port]; ok {
		l.Close()
		delete(reservedListeners, port)
	}
}
