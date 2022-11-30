package watchdog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
)

const (
	numRetries = 3
)

type Watchdog struct {
	host          string
	componentName string
	pollInterval  time.Duration
	client        http.Client
	logger        lager.Logger
}

func NewWatchdog(host string, componentName string, pollInterval time.Duration, healthcheckTimeout time.Duration, logger lager.Logger) *Watchdog {
	client := http.Client{
		Timeout: healthcheckTimeout,
	}
	return &Watchdog{
		host:          host,
		componentName: componentName,
		pollInterval:  pollInterval,
		client:        client,
		logger:        logger,
	}
}

func (w *Watchdog) WatchHealthcheckEndpoint(ctx context.Context, signals <-chan os.Signal) error {
	pollTimer := time.NewTimer(w.pollInterval)
	errCounter := 0
	defer pollTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Context done, exiting")
			return nil
		case sig := <-signals:
			if sig == syscall.SIGUSR1 {
				w.logger.Info("Received USR1 signal, exiting")
				return nil
			}
		case <-pollTimer.C:
			w.logger.Debug("Verifying endpoint", lager.Data{"component": w.componentName, "poll-interval": w.pollInterval})
			err := w.HitHealthcheckEndpoint()
			if err != nil {
				errCounter += 1
				if errCounter >= numRetries {
					select {
					case sig := <-signals:
						if sig == syscall.SIGUSR1 {
							w.logger.Info("Received USR1 signal, exiting")
							return nil
						}
					default:
						return err
					}
				} else {
					w.logger.Debug("Received error", lager.Data{"error": err.Error(), "attempt": errCounter})
				}
			} else {
				errCounter = 0
			}
			pollTimer.Reset(w.pollInterval)
		}
	}
}

func (w *Watchdog) HitHealthcheckEndpoint() error {
	response, err := w.client.Get(w.host)
	if err != nil {
		return err
	}
	// fmt.Printf("status: %d", response.StatusCode)
	if response.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf(
			"%v received from healthcheck endpoint (200 expected)",
			response.StatusCode))
	}
	return nil
}
