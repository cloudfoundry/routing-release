package route_test

import (
	"fmt"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("Maglev", func() {
	var (
		logger *test_util.TestLogger
		maglev *route.Maglev
	)

	BeforeEach(func() {
		logger = test_util.NewTestLogger("test")

		maglev = route.NewMaglev(logger.Logger)
	})

	Describe("NewMaglev", func() {
		It("should create a new Maglev instance", func() {
			Expect(maglev).NotTo(BeNil())
		})
	})

	Describe("Add", func() {
		Context("when adding a new backend", func() {
			It("should add the backend successfully", func() {
				maglev.Add("backend1")

				Expect(maglev.GetEndpointList()).To(HaveLen(1))
				Expect(maglev.GetLookupTable()).To(HaveLen(int(maglev.GetLookupTableSize())))
				Expect(maglev.GetPermutationTable()).To(HaveLen(1))
				Expect(maglev.GetPermutationTable()[0]).To(HaveLen(int(maglev.GetLookupTableSize())))

				result, err := maglev.Get("test-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("backend1"))
			})
		})

		Context("when adding a backend twice", func() {
			It("should skip adding subsequent adds", func() {
				maglev.Add("backend1")
				maglev.Add("backend1")

				Expect(maglev.GetEndpointList()).To(HaveLen(1))
				Expect(maglev.GetLookupTable()).To(HaveLen(int(maglev.GetLookupTableSize())))
				Expect(maglev.GetPermutationTable()).To(HaveLen(1))
				Expect(maglev.GetPermutationTable()[0]).To(HaveLen(int(maglev.GetLookupTableSize())))

				result, err := maglev.Get("test-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("backend1"))
			})
		})

		Context("when adding multiple backends", func() {
			It("should make all backends reachable", func() {
				maglev.Add("backend1")
				maglev.Add("backend2")
				maglev.Add("backend3")

				Expect(maglev.GetEndpointList()).To(HaveLen(3))
				Expect(maglev.GetLookupTable()).To(HaveLen(int(maglev.GetLookupTableSize())))
				Expect(maglev.GetPermutationTable()).To(HaveLen(len(maglev.GetEndpointList())))
				for i := range len(maglev.GetEndpointList()) {
					Expect(maglev.GetPermutationTable()[i]).To(HaveLen(int(maglev.GetLookupTableSize())))
				}

				backends := make(map[string]bool)
				for i := 0; i < 1000; i++ {
					result, err := maglev.Get(string(rune(i)))
					Expect(err).NotTo(HaveOccurred())
					backends[result] = true
				}

				Expect(backends["backend1"]).To(BeTrue())
				Expect(backends["backend2"]).To(BeTrue())
				Expect(backends["backend3"]).To(BeTrue())
			})
		})
	})

	Describe("Remove", func() {
		Context("when removing an existing backend", func() {
			It("should remove the backend successfully", func() {
				maglev.Add("backend1")
				maglev.Add("backend2")

				maglev.Remove("backend1")

				Expect(maglev.GetEndpointList()).To(HaveLen(1))
				Expect(maglev.GetLookupTable()).To(HaveLen(int(maglev.GetLookupTableSize())))
				Expect(maglev.GetPermutationTable()).To(HaveLen(1))
				Expect(maglev.GetPermutationTable()[0]).To(HaveLen(int(maglev.GetLookupTableSize())))

			})
		})

		Context("when removing a non-existent backend", func() {
			It("should handle gracefully without error", func() {
				maglev.Add("backend1")

				Expect(func() { maglev.Remove("non-existent") }).NotTo(Panic())

				Expect(maglev.GetEndpointList()).To(HaveLen(1))
				Expect(maglev.GetLookupTable()).To(HaveLen(int(maglev.GetLookupTableSize())))
				Expect(maglev.GetPermutationTable()).To(HaveLen(1))
				Expect(maglev.GetPermutationTable()[0]).To(HaveLen(int(maglev.GetLookupTableSize())))
			})
		})
	})

	Describe("Get", func() {
		Context("when no backends were added", func() {
			It("should return an error", func() {
				_, err := maglev.Get("test-key")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when backends are added", func() {
			BeforeEach(func() {
				maglev.Add("backend1")
				maglev.Add("backend2")
			})

			It("should return consistent results for the same key", func() {
				var counter = make(map[string]int)
				var result1 string
				var err error
				for _ = range 100 {
					result1, err = maglev.Get("consistent-key")
					Expect(err).NotTo(HaveOccurred())
					counter[result1]++
				}

				Expect(counter[result1]).To(Equal(100))
			})

			It("should distribute keys across backends", func() {
				maglev.Add("backend1")
				maglev.Add("backend2")
				maglev.Add("backend3")

				distribution := make(map[string]int)
				for i := range 1000 {
					result, err := maglev.Get(string(rune(i)))
					Expect(err).NotTo(HaveOccurred())
					distribution[result]++
				}

				Expect(distribution["backend1"]).To(BeNumerically(">", 0))
				Expect(distribution["backend2"]).To(BeNumerically(">", 0))
				Expect(distribution["backend3"]).To(BeNumerically(">", 0))
			})
		})

		Context("when backends are removed", func() {
			BeforeEach(func() {
				maglev.Add("backend1")
				maglev.Add("backend2")
				maglev.Remove("backend1")
			})

			It("should not return the removed backend", func() {
				for _ = range 100 {
					endpoint, err := maglev.Get("consistent-key")
					Expect(err).NotTo(HaveOccurred())
					Expect(endpoint).To(Equal("backend2"))
				}
			})
		})
	})

	Describe("Consistency", func() {
		// We test that at most half the keys are reassigned to new backends, when one backend is added.
		// This ensures a minimal level of consistency.
		It("should minimize disruption when adding backends", func() {
			for i := range 10 {
				maglev.Add(fmt.Sprintf("backend%d", i+1))
			}
			keys := make([]string, 1000)
			for i := range keys {
				keys[i] = fmt.Sprintf("key%d", i+1)
			}

			initialMappings := make(map[string]string)

			for _, key := range keys {
				backend, err := maglev.Get(key)
				Expect(err).NotTo(HaveOccurred())
				initialMappings[key] = backend
			}

			maglev.Add("newbackend")

			changedMappings := 0
			for _, key := range keys {
				backend, err := maglev.Get(key)
				Expect(err).NotTo(HaveOccurred())
				if initialMappings[key] != backend {
					changedMappings++
				}
			}

			Expect(changedMappings).To(BeNumerically("<=", len(keys)/2))
		})
	})

	Describe("Concurrency", func() {
		It("should handle concurrent reads safely", func() {
			maglev.Add("backend1")

			done := make(chan bool, 10)
			for i := 0; i < 10; i++ {
				go func() {
					defer GinkgoRecover()
					for j := 0; j < 100; j++ {
						_, err := maglev.Get("test-key")
						Expect(err).NotTo(HaveOccurred())
					}
					done <- true
				}()
			}

			for i := 0; i < 10; i++ {
				Eventually(done).Should(Receive())
			}
		})
		It("should handle concurrent endpoint registrations safely", func() {
			done := make(chan bool, 10)
			for i := 0; i < 10; i++ {
				go func() {
					defer GinkgoRecover()
					for j := 0; j < 100; j++ {
						Expect(func() { maglev.Add("endpoint" + strconv.Itoa(j)) }).NotTo(Panic())
					}
					done <- true
				}()
			}

			for i := 0; i < 10; i++ {
				Eventually(done).Should(Receive())
			}
			Expect(len(maglev.GetEndpointList())).To(Equal(100))
		})

	})
})
