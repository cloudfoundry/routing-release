package syncer_test

import (
	"os"
	"time"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/syncer"
	pclock "code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Syncer", func() {
	var (
		syncerRunner *syncer.Syncer
		syncChannel  chan struct{}
		clock        pclock.Clock
		syncInterval time.Duration
		logger       lager.Logger
		process      ifrit.Process
	)

	BeforeEach(func() {
		syncChannel = make(chan struct{})
		clock = pclock.NewClock()
		syncInterval = 1 * time.Second
		logger = lagertest.NewTestLogger("test")
		syncerRunner = syncer.New(clock, syncInterval, syncChannel, logger)
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait()).Should(Receive(BeNil()))
		close(syncChannel)
	})

	Context("on a specified interval", func() {
		var (
			watchChannel chan struct{}
			readyChannel chan struct{}
			closeChannel chan struct{}
		)

		BeforeEach(func() {
			watchChannel = make(chan struct{}, 1)
			readyChannel = make(chan struct{})
			closeChannel = make(chan struct{})
			syncChannel := syncChannel
			go func() {
				close(readyChannel)
			OUTERLOOP:
				for {
					select {
					case <-syncChannel:
						logger.Debug("received-sync")
						watchChannel <- struct{}{}
					case <-closeChannel:
						break OUTERLOOP
					}
				}
			}()
		})

		It("should sync", func() {
			duration := syncInterval + 100*time.Millisecond
			Eventually(readyChannel).Should(BeClosed())
			process = ifrit.Invoke(syncerRunner)
			// Consume the startup sync event
			Eventually(watchChannel).Should(Receive())

			logger.Debug("first-tick")
			Eventually(watchChannel, duration).Should(Receive())

			logger.Debug("second-tick")
			Eventually(watchChannel, duration).Should(Receive())
			close(closeChannel)
		})
	})

	Context("on startup", func() {
		var (
			watchChannel chan struct{}
			readyChannel chan struct{}
		)
		BeforeEach(func() {
			watchChannel = make(chan struct{})
			readyChannel = make(chan struct{})
			go func() {
				close(readyChannel)
				select {
				case <-syncChannel:
					logger.Debug("received-sync")
					watchChannel <- struct{}{}
				}

			}()
		})
		It("should sync", func() {
			Eventually(readyChannel).Should(BeClosed())
			process = ifrit.Invoke(syncerRunner)
			Eventually(watchChannel).Should(Receive())
		})
	})
})
