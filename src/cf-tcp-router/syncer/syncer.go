package syncer

import (
	"os"
	"time"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager"
)

type Syncer struct {
	clock        clock.Clock
	syncInterval time.Duration
	syncChannel  chan struct{}
	logger       lager.Logger
}

func New(
	clock clock.Clock,
	syncInterval time.Duration,
	syncChannel chan struct{},
	logger lager.Logger,
) *Syncer {
	return &Syncer{
		clock:        clock,
		syncInterval: syncInterval,
		syncChannel:  syncChannel,

		logger: logger.Session("syncer"),
	}
}

func (s *Syncer) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)
	s.logger.Info("started")

	s.sync()

	//now keep emitting at the desired interval, syncing with etcd every syncInterval
	syncTicker := s.clock.NewTicker(s.syncInterval)

	for {
		select {
		case <-syncTicker.C():
			s.sync()
		case <-signals:
			s.logger.Info("stopping")
			syncTicker.Stop()
			return nil
		}
	}
}

func (s *Syncer) sync() {
	select {
	case s.syncChannel <- struct{}{}:
	default:
		s.logger.Debug("sync-already-in-progress")
	}
}
