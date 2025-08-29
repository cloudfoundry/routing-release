package route_test

import (
	"code.cloudfoundry.org/gorouter/config"
	_ "errors"
	"time"

	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			MaxConnsPerBackend:     0,
			LoadBalancingAlgorithm: config.LOAD_BALANCE_HB,
		})
	})

	Describe("Next", func() {

		Context("when pool is empty", func() {
			It("does not select an endpoint", func() {
				iter := route.NewHashBased(logger.Logger, pool, "", false, false, "")
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
				iter := route.NewHashBased(logger.Logger, pool, "", false, false, "")
				iter.(*route.HashBased).HeaderValue = "tenant-1"
				first := iter.Next(0)
				second := iter.Next(0)
				Expect(first).NotTo(BeNil())
				Expect(second).NotTo(BeNil())
				Expect(first).To(Equal(second))
			})

			It("It selects another instance for other hash header value", func() {
				iter := route.NewHashBased(logger.Logger, pool, "", false, false, "")
				iter.(*route.HashBased).HeaderValue = "example.com"
				Expect(iter.Next(0)).NotTo(BeNil())
				Expect(iter.Next(0)).To(Equal(endpoints[1]))
				Expect(iter.Next(0)).To(Equal(endpoints[1]))
				Expect(iter.Next(0)).To(Equal(endpoints[1]))
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
				BeforeEach(func() {
					iter = route.NewHashBased(logger.Logger, pool, "ID1", true, false, "")
				})

				It("returns the sticky endpoint when it exists", func() {
					endpoint := iter.Next(0)
					Expect(endpoint).NotTo(BeNil())
					Expect(endpoint.PrivateInstanceId).To(Equal("ID1"))
				})

				It("returns nil when sticky endpoint doesn't exist", func() {
					iter = route.NewHashBased(logger.Logger, pool, "nonexistent-id", true, false, "")
					Expect(iter.Next(0)).To(BeNil())
				})
			})

			Context("when mustBeSticky is false", func() {
				BeforeEach(func() {
					iter = route.NewHashBased(logger.Logger, pool, "ID1", false, false, "")
				})

				It("returns the sticky endpoint when it exists", func() {
					endpoint := iter.Next(0)
					Expect(endpoint).NotTo(BeNil())
					Expect(endpoint.PrivateInstanceId).To(Equal("ID1"))
				})

				It("falls back to hash-based routing when sticky endpoint doesn't exist", func() {
					iter = route.NewHashBased(logger.Logger, pool, "nonexistent-id", false, false, "")
					hashIter := iter.(*route.HashBased)
					hashIter.HeaderValue = "some-value"
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
			iter = route.NewHashBased(logger.Logger, pool, "", false, false, "")
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

})
