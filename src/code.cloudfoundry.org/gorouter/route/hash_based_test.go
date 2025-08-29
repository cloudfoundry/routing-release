package route_test

import (
	_ "errors"
	"hash/fnv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("HashBased", func() {
	var (
		pool   *route.EndpointPool
		logger *test_util.TestLogger
	)

	BeforeEach(func() {
		logger = test_util.NewTestLogger("test")
		pool = route.NewPool(&route.PoolOpts{
			Logger:                 logger.Logger,
			RetryAfterFailure:      2 * time.Minute,
			Host:                   "",
			ContextPath:            "",
			MaxConnsPerBackend:     500,
			LoadBalancingAlgorithm: config.LOAD_BALANCE_HB,
			HashHeader:             "tenant-id",
		})
	})

	Describe("Next", func() {

		Context("when pool is empty", func() {
			It("does not select an endpoint", func() {
				iter := route.NewHashBased(logger.Logger, pool, "", false, "tenant-1")
				Expect(iter.Next(0)).To(BeNil())
			})
		})

		Context("when pool has endpoints", func() {
			var (
				endpoints []*route.Endpoint
			)
			BeforeEach(func() {
				e1 := route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", PrivateInstanceId: "ID1"})
				e2 := route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", PrivateInstanceId: "ID2"})
				endpoints = []*route.Endpoint{e1, e2}
				for _, e := range endpoints {
					pool.Put(e)
				}

			})
			It("It returns the same endpoint for the same header value", func() {
				iter := route.NewHashBased(logger.Logger, pool, "", false, "tenant-1")
				first := iter.Next(0)
				second := iter.Next(0)
				Expect(first).NotTo(BeNil())
				Expect(second).NotTo(BeNil())
				Expect(first).To(Equal(second))
			})
		})

		Context("when endpoint overloaded", func() {
			var (
				endpoints []*route.Endpoint
				e1        *route.Endpoint
				e2        *route.Endpoint
				e3        *route.Endpoint
			)
			It("It returns the next endpoint for the same header value when balancer factor set", func() {
				e1 = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID1"})
				e2 = route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID2"})
				e3 = route.NewEndpoint(&route.EndpointOpts{Host: "3.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID3"})
				endpoints = []*route.Endpoint{e1, e2, e3}
				for _, e := range endpoints {
					pool.Put(e)
				}
				iter := route.NewHashBased(logger.Logger, pool, "", false, "tenant-1")
				first := iter.Next(0)
				Expect(iter.Next(0)).To(Equal(first))
				for i := 0; i < 6; i++ {
					iter.PreRequest(first)
				}
				second := iter.Next(0)
				Expect(second).NotTo(Equal(first))
			})
			It("It returns the same overloaded endpoint for the same header value when balancer factor not set", func() {
				e1 = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 0, PrivateInstanceId: "ID1"})
				e2 = route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 0, PrivateInstanceId: "ID2"})
				e3 = route.NewEndpoint(&route.EndpointOpts{Host: "3.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 0, PrivateInstanceId: "ID3"})
				endpoints = []*route.Endpoint{e1, e2, e3}
				for _, e := range endpoints {
					pool.Put(e)
				}
				iter := route.NewHashBased(logger.Logger, pool, "", false, "tenant-1")
				iter.(*route.HashBased).HeaderValue = "tenant-1"
				first := iter.Next(0)
				Expect(iter.Next(0)).To(Equal(first))
				for i := 0; i < 6; i++ {
					iter.PreRequest(first)
				}
				second := iter.Next(0)
				Expect(second).To(Equal(first))
			})

		})

		Context("with retries", func() {
			var (
				endpoints         []*route.Endpoint
				e1                *route.Endpoint
				e2                *route.Endpoint
				e3                *route.Endpoint
				e4                *route.Endpoint
				MaglevLookupTable = []int16{2, 2, 1, 0, 1, 0, 0, 0, 2, 0, 1, 3, 1, 0, 1, 0, 3, 0, 3, 0, 0, 0, 1, 0, 1, 2, 2, 0, 3, 2, 3, 0, 1, 0, 1, 0, 3, 3, 2, 0, 3, 1, 2, 0, 3, 0, 1, 0, 2, 3, 2, 3, 2, 0, 1, 2, 1, 0, 3, 2, 2, 1, 1, 2, 1, 3, 1, 2, 2, 0, 3, 2, 3, 1, 1, 3, 1, 3, 1, 0, 2, 1, 3, 1, 2, 2, 1, 3, 2, 2, 2, 3, 3, 1, 3, 0, 3, 2, 3, 3, 0}
			)
			It("It returns next endpoint from maglev lookup table", func() {
				e1 = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", PrivateInstanceId: "ID1"})
				e2 = route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", PrivateInstanceId: "ID2"})
				e3 = route.NewEndpoint(&route.EndpointOpts{Host: "3.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", PrivateInstanceId: "ID3"})
				e4 = route.NewEndpoint(&route.EndpointOpts{Host: "4.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", PrivateInstanceId: "ID4"})

				endpoints = []*route.Endpoint{e1, e2, e3, e4}
				endpointIDList := make([]string, 0, 4)
				for _, e := range endpoints {
					pool.Put(e)
					endpointIDList = append(endpointIDList, e.PrivateInstanceId)
				}
				maglevMock := NewMockHashLookupTable(MaglevLookupTable, endpointIDList)
				pool.HashLookupTable = maglevMock
				iter := route.NewHashBased(logger.Logger, pool, "", false, "tenant-1")
				// The returned endpoint has always ID3 according to the Maglev lookup table
				first := iter.Next(0)
				Expect(first).To(Equal(e4))
				second := iter.Next(1)
				Expect(second).To(Equal(e1))
				third := iter.Next(2)
				Expect(third).To(Equal(e4))
			})
			It("It returns the next not overloaded endpoint for the second attempt", func() {
				e1 = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID1"})
				e2 = route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID2"})
				e3 = route.NewEndpoint(&route.EndpointOpts{Host: "3.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID3"})
				e4 = route.NewEndpoint(&route.EndpointOpts{Host: "4.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID4"})

				endpoints = []*route.Endpoint{e1, e2, e3, e4}
				for _, e := range endpoints {
					pool.Put(e)
				}
				iter := route.NewHashBased(logger.Logger, pool, "", false, "tenant-1")
				firstAttemptResult := iter.Next(0)
				Expect(iter.Next(0)).To(Equal(firstAttemptResult))
				for i := 0; i < 6; i++ {
					// Simulate requests to overload the endpoints
					iter.PreRequest(e1)
					iter.PreRequest(e2)
				}
				secondAttemptResult := iter.Next(1)
				Expect(secondAttemptResult).NotTo(Equal(firstAttemptResult))
				Expect(secondAttemptResult).NotTo(Equal(e1))
				Expect(secondAttemptResult).NotTo(Equal(e2))
			})
		})

		Context("when using sticky sessions", func() {
			var (
				endpoints []*route.Endpoint
				iter      route.EndpointIterator
			)

			BeforeEach(func() {
				e1 := route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID1"})
				e2 := route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID2"})
				e3 := route.NewEndpoint(&route.EndpointOpts{Host: "3.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", PrivateInstanceId: "ID3"})
				endpoints = []*route.Endpoint{e1, e2, e3}
				for _, e := range endpoints {
					pool.Put(e)
				}
			})

			Context("when mustBeSticky is true", func() {
				It("returns the sticky endpoint when it exists", func() {
					iter = route.NewHashBased(logger.Logger, pool, "ID1", true, "abc")
					endpoint := iter.Next(0)
					Expect(endpoint).NotTo(BeNil())
					Expect(endpoint.PrivateInstanceId).To(Equal("ID1"))
				})

				It("returns nil when sticky endpoint doesn't exist", func() {
					iter = route.NewHashBased(logger.Logger, pool, "nonexistent-id", true, "abc")
					Expect(iter.Next(0)).To(BeNil())
				})
				It("returns nil when sticky endpoint is overloaded and mustBeSticky is true", func() {
					iter = route.NewHashBased(logger.Logger, pool, "ID1", true, "abc")
					for i := 0; i < 1000; i++ {
						iter.PreRequest(endpoints[0])
					}
					Expect(iter.Next(0)).To(BeNil())
				})
			})

			Context("when mustBeSticky is false", func() {
				BeforeEach(func() {
					iter = route.NewHashBased(logger.Logger, pool, "ID1", false, "some-value")
				})

				It("returns the sticky endpoint when it exists", func() {
					endpoint := iter.Next(0)
					Expect(endpoint).NotTo(BeNil())
					Expect(endpoint.PrivateInstanceId).To(Equal("ID1"))
				})

				It("falls back to hash-based routing when sticky endpoint doesn't exist", func() {
					iter = route.NewHashBased(logger.Logger, pool, "nonexistent-id", false, "some-value")
					endpoint := iter.Next(0)
					Expect(endpoint).NotTo(BeNil())
				})
			})
		})
	})

	Context("when testing PreRequest and PostRequest", func() {
		var (
			endpoint *route.Endpoint
			iter     route.EndpointIterator
		)

		BeforeEach(func() {
			endpoint = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID1"})
			pool.Put(endpoint)
			iter = route.NewHashBased(logger.Logger, pool, "", false, "abc")
		})

		It("increments connection count on PreRequest", func() {
			initialCount := endpoint.Stats.NumberConnections.Count()
			iter.PreRequest(endpoint)
			Expect(endpoint.Stats.NumberConnections.Count()).To(Equal(initialCount + 1))
		})

		It("decrements connection count on PostRequest", func() {
			iter.PreRequest(endpoint)
			initialCount := endpoint.Stats.NumberConnections.Count()
			iter.PostRequest(endpoint)
			Expect(endpoint.Stats.NumberConnections.Count()).To(Equal(initialCount - 1))
		})
	})
	Describe("IsImbalancedOrOverloaded", func() {
		var iter *route.HashBased
		var endpoints []*route.Endpoint

		BeforeEach(func() {
			iter = route.NewHashBased(logger.Logger, pool, "", false, "abc").(*route.HashBased)
		})

		Context("when endpoints have a lot of in-flight requests", func() {
			var e1, e2, e3 *route.Endpoint
			BeforeEach(func() {
				e1 = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID1"})
				e2 = route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID2"})
				e3 = route.NewEndpoint(&route.EndpointOpts{Host: "3.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", HashHeaderName: "tenant-id", HashBalanceFactor: 1.2, PrivateInstanceId: "ID3"})
				endpoints = []*route.Endpoint{e1, e2, e3}
				for _, e := range endpoints {
					pool.Put(e)
				}

			})
			It("do not mark as imbalanced if every endpoint has 499 in-flight requests", func() {
				for i := 0; i < 498; i++ {
					iter.PreRequest(e1)
				}
				for i := 0; i < 498; i++ {
					iter.PreRequest(e2)
				}
				for i := 0; i < 498; i++ {
					iter.PreRequest(e3)
				}
				// in general 500 in flight requests counted by e1
				Expect(iter.IsImbalanced(e1)).To(BeFalse())
			})
			It("mark as imbalanced if it has more in-flight requests", func() {
				for i := 0; i < 300; i++ {
					iter.PreRequest(e1)
				}
				for i := 0; i < 200; i++ {
					iter.PreRequest(e2)
				}
				for i := 0; i < 200; i++ {
					iter.PreRequest(e3)
				}
				Expect(iter.IsImbalanced(e1)).To(BeTrue())
				Eventually(logger).Should(gbytes.Say("hash-based-routing-endpoint-imbalanced"))
				Expect(iter.IsImbalanced(e2)).To(BeFalse())
				Expect(iter.IsImbalanced(e3)).To(BeFalse())
			})
		})
	})

	Describe("CalculateAverageNumberOfConnections", func() {
		var iter *route.HashBased
		var endpoints []*route.Endpoint

		BeforeEach(func() {
			iter = route.NewHashBased(logger.Logger, pool, "", false, "abc").(*route.HashBased)
		})

		Context("when there are no endpoints", func() {
			It("returns 0", func() {
				Expect(iter.CalculateAverageLoad()).To(Equal(float64(0)))
			})
		})

		Context("when all endpoints have zero connections", func() {
			BeforeEach(func() {
				pool.Put(route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID1"}))
				pool.Put(route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID2"}))
			})
			It("returns 0", func() {
				Expect(iter.CalculateAverageLoad()).To(Equal(float64(0)))
			})
		})

		Context("when endpoints have varying connection counts", func() {
			var e1, e2, e3 *route.Endpoint
			BeforeEach(func() {
				e1 = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID1"})
				e2 = route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID2"})
				e3 = route.NewEndpoint(&route.EndpointOpts{Host: "3.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID3"})
				endpoints = []*route.Endpoint{e1, e2, e3}
				for _, e := range endpoints {
					pool.Put(e)
				}
				for i := 0; i < 2; i++ {
					iter.PreRequest(e1)
				}
				for i := 0; i < 4; i++ {
					iter.PreRequest(e2)
				}
				for i := 0; i < 6; i++ {
					iter.PreRequest(e3)
				}
			})
			It("returns the correct average", func() {
				// in general 12 in flight requests
				Expect(iter.CalculateAverageLoad()).To(Equal(float64(4)))
			})
		})

		Context("when one endpoint has many connections", func() {
			var e1, e2 *route.Endpoint
			BeforeEach(func() {
				e1 = route.NewEndpoint(&route.EndpointOpts{Host: "1.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID1"})
				e2 = route.NewEndpoint(&route.EndpointOpts{Host: "2.2.3.4", Port: 5678, LoadBalancingAlgorithm: "hash", PrivateInstanceId: "ID2"})
				endpoints = []*route.Endpoint{e1, e2}
				for _, e := range endpoints {
					pool.Put(e)
				}
				for i := 0; i < 10; i++ {
					iter.PreRequest(e1)
				}
			})
			It("returns the correct average", func() {
				Expect(iter.CalculateAverageLoad()).To(Equal(float64(5)))
			})
		})
	})

})

// MockHashLookupTable provides a simple mock implementation of MaglevLookup interface for testing.
type MockHashLookupTable struct {
	lookupTable  []int16
	endpointList []string
}

// NewMockHashLookupTable creates a new mock lookup table with predefined mappings
func NewMockHashLookupTable(lookupTable []int16, endpointList []string) *MockHashLookupTable {
	return &MockHashLookupTable{
		lookupTable:  lookupTable,
		endpointList: endpointList,
	}
}

func (m *MockHashLookupTable) GetInstanceForHashHeader(hashHeaderValue string) (uint64, string, error) {
	if len(m.endpointList) == 0 {
		return 0, "", nil
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(hashHeaderValue))
	key := h.Sum64()
	index := key % m.GetLookupTableSize()
	return index, m.endpointList[m.lookupTable[index]], nil

}

func (m *MockHashLookupTable) GetLookupTableSize() uint64 {
	return uint64(len(m.lookupTable))
}

func (m *MockHashLookupTable) GetEndpointId(lookupTableIndex uint64) string {
	return m.endpointList[m.lookupTable[lookupTableIndex]]
}

func (m *MockHashLookupTable) Add(endpoint string) {
	// Check if endpoint already exists
	for _, existing := range m.endpointList {
		if existing == endpoint {
			return
		}
	}
	m.endpointList = append(m.endpointList, endpoint)
}

func (m *MockHashLookupTable) Remove(endpoint string) {
	for i, existing := range m.endpointList {
		if existing == endpoint {
			m.endpointList = append(m.endpointList[:i], m.endpointList[i+1:]...)
			return
		}
	}
}

func (m *MockHashLookupTable) GetEndpointList() []string {
	return append([]string(nil), m.endpointList...) // return a copy
}

// GetLookupTable returns a copy of the current lookup table (for testing)
func (m *MockHashLookupTable) GetLookupTable() []int16 {
	return m.lookupTable // return a copy
}

// GetPermutationTable returns a copy of the current permutation table (for testing)
func (m *MockHashLookupTable) GetPermutationTable() [][]uint16 {
	return nil // not implemented in mock
}

// Compile-time check to ensure MockHashLookupTable implements MaglevLookup interface
var _ route.MaglevLookup = (*MockHashLookupTable)(nil)
