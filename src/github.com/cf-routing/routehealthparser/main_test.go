package main_test

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Results", func() {
	var (
		server   *httptest.Server
		response string
	)

	BeforeEach(func() {
		var m sync.Mutex

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			m.Lock()
			w.Write([]byte(response))
			m.Unlock()
		}))
	})

	Context("when downtime is reported", func() {
		Context("when the success rate is > 99", func() {
			It("displays the results from the server and succeeds", func() {
				response = `{"totalrequests": 100, "responses": {"200": 99, "500": 1}}`
				cmd := exec.Command(healthBinary, "-address", server.URL)
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(0))

				Expect(session.Out).Should(gbytes.Say("Response:\n "))
				Expect(session.Out).Should(gbytes.Say(response))
				Expect(session.Out).Should(gbytes.Say(`Success rate \(0.990000\)`))
			})
		})

		Context("when the success rate is < 99", func() {
			It("displays the results from the server and fails", func() {
				response = `{"totalrequests": 100, "responses": {"200": 98, "500": 2}}`
				cmd := exec.Command(healthBinary, "-address", server.URL)
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(1))

				Expect(session.Out).Should(gbytes.Say("Response:\n "))
				Expect(session.Out).Should(gbytes.Say(response))
				Expect(session.Err).Should(gbytes.Say(`Success rate \(0.980000\)`))
			})
		})
		Context("when custom threshold is passed in success rates >= threshold", func() {
			It("displays the results from the server and succeeds", func() {
				response = `{"totalrequests": 100, "responses": {"200": 98, "500": 2}}`
				cmd := exec.Command(healthBinary, "-address", server.URL, "-threshold", "98")
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(0))

				Expect(session.Out).Should(gbytes.Say("Response:\n "))
				Expect(session.Out).Should(gbytes.Say(response))
				Expect(session.Out).Should(gbytes.Say(`Success rate \(0.980000\)`))
			})
		})

		Context("when custom threshold is passed in success rates < threshold", func() {
			It("displays the results from the server and fails", func() {
				response = `{"totalrequests": 100, "responses": {"200": 97, "500": 2}}`
				cmd := exec.Command(healthBinary, "-address", server.URL, "-threshold", "98")
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(1))

				Expect(session.Out).Should(gbytes.Say("Response:\n "))
				Expect(session.Out).Should(gbytes.Say(response))
				Expect(session.Err).Should(gbytes.Say(`Success rate \(0.970000\)`))
			})
		})
	})

	Context("when no downtime is reported", func() {
		It("displays the results from the server", func() {
			response = `{"totalrequests": 100, "responses": {"200": 100}}`
			cmd := exec.Command(healthBinary, "-address", server.URL)
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(session).Should(gexec.Exit(0))

			Expect(session.Out).Should(gbytes.Say("Response:\n "))
			Expect(session.Out).Should(gbytes.Say(response))
			Expect(session.Out).Should(gbytes.Say("No downtime for this app!\n"))
		})
	})

	Context("when an error occurs", func() {
		Context("when no server flag has been provided", func() {
			It("returns an error", func() {
				cmd := exec.Command(healthBinary)
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(1))

				Expect(session.Err).Should(gbytes.Say("address not provided"))
			})
		})

		Context("when the server address is unreachable", func() {
			It("returns an error", func() {
				cmd := exec.Command(healthBinary, "-address", "localhost:7890")
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(1))

				Expect(session.Err).Should(gbytes.Say("GET request failed"))
			})
		})

		Context("when the server returns invalid json", func() {
			It("returns an error", func() {
				response = `""""""`
				cmd := exec.Command(healthBinary, "-address", server.URL)
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(1))

				Expect(session.Err.Contents()).Should((ContainSubstring("invalid character")))
			})
		})
	})
})
