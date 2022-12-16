package watchdog

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
)

const (
	numRetries = 3
)

type Watchdog struct {
	url            *url.URL
	componentName  string
	pollInterval   time.Duration
	client         http.Client
	logger         lager.Logger
	failureCounter *FailureCounter
}

func NewWatchdog(u *url.URL, componentName string, failureCounterFileName string, pollInterval time.Duration, healthcheckTimeout time.Duration, logger lager.Logger) *Watchdog {
	client := http.Client{
		Timeout: healthcheckTimeout,
	}
	if strings.HasPrefix(u.Host, "unix") {
		socket := strings.TrimPrefix(u.Host, "unix")
		client.Transport = &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socket)
			},
		}
	}

	failureCounterFile, err := os.OpenFile(failureCounterFileName, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		logger.Fatal("cannot-create-failure-counter-file", err)
		return nil
	}
	failureCounterFile.Close()

	return &Watchdog{
		url:            u,
		componentName:  componentName,
		pollInterval:   pollInterval,
		client:         client,
		logger:         logger,
		failureCounter: &FailureCounter{file: failureCounterFileName, logger: logger},
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
						incrErr := w.failureCounter.Increment()
						if incrErr != nil {
							w.logger.Error("incrementing-failure-count", incrErr, lager.Data{"file": w.failureCounter.file})
						}
						return err
					}
				} else {
					w.logger.Debug("Received error", lager.Data{"error": err.Error(), "attempt": errCounter})
				}
			} else {
				err = w.failureCounter.Set(0)
				if err != nil {
					w.logger.Error("resetting-failure-count", err, lager.Data{"file": w.failureCounter.file})
				}
				errCounter = 0
			}
			pollTimer.Reset(w.pollInterval)
		}
	}
}

func (w *Watchdog) HitHealthcheckEndpoint() error {
	req, err := http.NewRequest("GET", w.url.String(), nil)
	if err != nil {
		return err
	}
	if req.URL.Host == "" {
		req.URL.Host = w.url.Host
	}

	response, err := w.client.Do(req)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf(
			"%v received from healthcheck endpoint (200 expected)",
			response.StatusCode))
	}
	return nil
}

type FailureCounter struct {
	file   string
	logger lager.Logger
}

func (fc *FailureCounter) Set(i int) error {
	return os.WriteFile(fc.file, []byte(fmt.Sprintf("%d\n", i)), 0644)
}

func (fc *FailureCounter) Increment() error {
	failures, err := os.ReadFile(fc.file)
	if err != nil {
		return err
	}

	failuresInt, err := strconv.Atoi(strings.TrimSpace(string(failures)))
	if err != nil {
		fc.logger.Info("converting-failure-counter-string-to-int", lager.Data{"error": err, "message": "Resetting counter to 0"})
		failuresInt = 0
	}

	failuresInt++
	return fc.Set(failuresInt)
}
