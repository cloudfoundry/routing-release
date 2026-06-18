package test_helpers

import (
	"sync"

	"github.com/onsi/ginkgo/v2"
)

var (
	lastPortUsed uint16
	portLock     sync.Mutex
	once         sync.Once
)

func NextAvailPort() uint16 {
	portLock.Lock()
	defer portLock.Unlock()

	if lastPortUsed == 0 {
		once.Do(func() {
			const portRangeStart = 24000
			// #nosec G115 - if we have more than 65k or negative parallel processes, there's a bigger problem
			lastPortUsed = portRangeStart + uint16(ginkgo.GinkgoParallelProcess())
		})
	}

	suiteCfg, _ := ginkgo.GinkgoConfiguration()
	// #nosec G115 - if we have more than 65k or negative parallel processes, there's a bigger problem
	lastPortUsed += uint16(suiteCfg.ParallelTotal)
	return lastPortUsed
}
