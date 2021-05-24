package test_helpers

import (
	"sync"

	. "github.com/onsi/ginkgo/config"
)

var (
	lastPortUsed int
	portLock     sync.Mutex
	once         sync.Once
)

func NextAvailPort() int {
	portLock.Lock()
	defer portLock.Unlock()

	if lastPortUsed == 0 {
		once.Do(func() {
			const portRangeStart = 61000
			lastPortUsed = portRangeStart + GinkgoConfig.ParallelNode
		})
	}

	lastPortUsed += GinkgoConfig.ParallelTotal
	return lastPortUsed
}
