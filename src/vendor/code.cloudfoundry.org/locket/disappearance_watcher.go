package locket

import (
	"os"
	"time"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/consuladapter"
	"code.cloudfoundry.org/lager"
	"github.com/hashicorp/consul/api"
)

type DisappearanceWatcher struct {
	consulClient  consuladapter.Client
	keyPrefix     string
	disappearChan chan []string

	clock  clock.Clock
	logger lager.Logger
}

func NewDisappearanceWatcher(
	logger lager.Logger,
	consulClient consuladapter.Client,
	keyPrefix string,
	clock clock.Clock,
) (DisappearanceWatcher, <-chan []string) {
	disappearChan := make(chan []string)
	return DisappearanceWatcher{
		consulClient:  consulClient,
		keyPrefix:     keyPrefix,
		disappearChan: disappearChan,

		clock:  clock,
		logger: logger,
	}, disappearChan
}

func (d DisappearanceWatcher) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	logger := d.logger.Session("disappearance-watcher", lager.Data{"key-prefix": d.keyPrefix})
	logger.Info("starting")
	defer logger.Info("done")

	stop := make(chan struct{})
	WatchForDisappearancesUnder(logger, d.consulClient, d.disappearChan, stop, d.keyPrefix)
	close(ready)

	select {
	case <-signals:
		logger.Info("signalled")
	}

	close(stop)
	close(d.disappearChan)
	return nil
}

const defaultWatchBlockDuration = 10 * time.Second

var emptyBytes = []byte{}

func WatchForDisappearancesUnder(logger lager.Logger, client consuladapter.Client, disappearanceChan chan []string, stop <-chan struct{}, prefix string) {
	logger = logger.Session("watch-for-disappearances")

	go func() {
		logger.Debug("starting")
		defer logger.Debug("finished")

		keys := keySet{}

		queryOpts := &api.QueryOptions{
			WaitIndex: 0,
			WaitTime:  defaultWatchBlockDuration,
		}

		for {
			newPairs, queryMeta, err := client.KV().List(prefix, queryOpts)

			if err != nil {
				logger.Error("list-failed", err)
				select {
				case <-stop:
					return
				case <-time.After(1 * time.Second):
				}
				queryOpts.WaitIndex = 0
				continue
			}

			select {
			case <-stop:
				return
			default:
			}

			queryOpts.WaitIndex = queryMeta.LastIndex

			if newPairs == nil {
				// key not found
				_, err = client.KV().Put(&api.KVPair{Key: prefix, Value: emptyBytes}, nil)
				if err != nil {
					logger.Error("put-failed", err)
					continue
				}
			}

			newKeys := newKeySet(newPairs)
			if missing := difference(keys, newKeys); len(missing) > 0 {
				select {
				case disappearanceChan <- missing:
				case <-stop:
					return
				}
			}

			keys = newKeys
		}
	}()
}

type keySet map[string]struct{}

func newKeySet(keyPairs api.KVPairs) keySet {
	newKeySet := keySet{}
	for _, kvPair := range keyPairs {
		if kvPair.Session != "" {
			newKeySet[kvPair.Key] = struct{}{}
		}
	}
	return newKeySet
}

func difference(a, b keySet) []string {
	var missing []string
	for key, _ := range a {
		if _, ok := b[key]; !ok {
			missing = append(missing, key)
		}
	}

	return missing
}
