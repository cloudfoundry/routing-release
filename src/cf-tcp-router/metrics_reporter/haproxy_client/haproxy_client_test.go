package haproxy_client_test

import (
	"io/ioutil"
	"net"
	"os"
	"path"
	"time"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/metrics_reporter/haproxy_client"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/testutil"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/utils"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("HaproxyClient", func() {
	var (
		haproxyClient     haproxy_client.HaproxyClient
		logger            lager.Logger
		haproxyUnixSocket string
		timeout           time.Duration
	)

	setupUnixSocketServer := func(data []byte, unixSocket string, ready chan struct{}) {
		defer GinkgoRecover()
		l, err := net.Listen("unix", unixSocket)
		Expect(err).NotTo(HaveOccurred())
		close(ready)

		defer func() {
			err := l.Close()
			Expect(err).NotTo(HaveOccurred())
		}()

		fd, err := l.Accept()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			err := fd.Close()
			Expect(err).NotTo(HaveOccurred())
		}()

		buf := make([]byte, 512)
		_, err = fd.Read(buf)
		Expect(string(buf)).To(ContainSubstring("show stat"))

		_, err = fd.Write(data)
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		timeout = 100 * time.Millisecond
		logger = lagertest.NewTestLogger("test")
	})

	Describe("GetStats", func() {
		BeforeEach(func() {
			randomFileName := testutil.RandomFileName("haproxy_", ".sock")
			haproxyUnixSocket = path.Join(os.TempDir(), randomFileName)
		})

		AfterEach(func() {
			Eventually(func() bool {
				return utils.FileExists(haproxyUnixSocket)
			}, 5*time.Second).Should(BeFalse())
		})

		Context("when haproxy provides statistics", func() {
			BeforeEach(func() {
				readyChannel := make(chan struct{})
				csvPayload, err := ioutil.ReadFile("fixtures/testdata.csv")
				Expect(err).NotTo(HaveOccurred())

				go setupUnixSocketServer(csvPayload, haproxyUnixSocket, readyChannel)
				haproxyClient = haproxy_client.NewClient(logger, haproxyUnixSocket, timeout)
				Eventually(readyChannel).Should(BeClosed())
			})

			It("returns haproxy statistics", func() {
				stats := haproxyClient.GetStats()
				Expect(stats).Should(HaveLen(9))

				r0 := haproxy_client.HaproxyStat{
					ProxyName:            "stats",
					CurrentQueued:        100,
					CurrentSessions:      101,
					ErrorConnecting:      102,
					AverageQueueTimeMs:   103,
					AverageConnectTimeMs: 104,
					AverageSessionTimeMs: 105,
				}

				r8 := haproxy_client.HaproxyStat{
					ProxyName:            "listen_cfg_60001",
					CurrentQueued:        1000,
					CurrentSessions:      1001,
					ErrorConnecting:      1002,
					AverageQueueTimeMs:   1003,
					AverageConnectTimeMs: 1004,
					AverageSessionTimeMs: 1005,
				}

				Expect(stats[0]).Should(Equal(r0))
				Expect(stats[8]).Should(Equal(r8))
			})
		})

		Context("when haproxy does not provide statistics", func() {
			BeforeEach(func() {
				readyChannel := make(chan struct{})
				go setupUnixSocketServer([]byte{}, haproxyUnixSocket, readyChannel)
				haproxyClient = haproxy_client.NewClient(logger, haproxyUnixSocket, timeout)
				Eventually(readyChannel).Should(BeClosed())
			})

			It("returns empty haproxy statistics", func() {
				stats := haproxyClient.GetStats()
				Expect(stats).Should(HaveLen(0))
			})
		})

		Context("when haproxy is not listening on unix domain socket", func() {
			BeforeEach(func() {
				haproxyClient = haproxy_client.NewClient(logger, haproxyUnixSocket, timeout)
			})

			It("returns empty haproxy statistics", func() {
				stats := haproxyClient.GetStats()
				Expect(stats).Should(HaveLen(0))
				Expect(logger).Should(gbytes.Say("test.get-stats.error-connecting-to-haproxy-stats"))
			})
		})

		Context("when haproxy returns invalid csv", func() {
			BeforeEach(func() {
				readyChannel := make(chan struct{})
				csvPayload, err := ioutil.ReadFile("fixtures/invalid.csv")
				Expect(err).NotTo(HaveOccurred())

				go setupUnixSocketServer(csvPayload, haproxyUnixSocket, readyChannel)
				haproxyClient = haproxy_client.NewClient(logger, haproxyUnixSocket, timeout)
				Eventually(readyChannel).Should(BeClosed())
			})

			It("returns empty haproxy statistics", func() {
				stats := haproxyClient.GetStats()
				Expect(stats).Should(HaveLen(0))
				Expect(logger).Should(gbytes.Say("test.get-stats.error-reading-csv-stats"))
			})
		})
	})
})
