package monitor

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o fakes/fake_monitor.go . Monitor
type Monitor interface {
	StartWatching()
	StopWatching()
	Run(signals <-chan os.Signal, ready chan<- struct{}) error
}

type monitor struct {
	haproxyPIDFile string
	stopWatching   int32
	logger         lager.Logger
}

func New(haproxyPIDFile string, logger lager.Logger) Monitor {
	return &monitor{
		haproxyPIDFile: haproxyPIDFile,
		stopWatching:   0,
		logger:         logger.Session("monitor"),
	}
}

func (m *monitor) StartWatching() {
	atomic.StoreInt32(&m.stopWatching, 0)
}

func (m *monitor) StopWatching() {
	atomic.StoreInt32(&m.stopWatching, 1)
}

func (m *monitor) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	m.logger.Debug("starting")
	defer m.logger.Debug("finished")

	close(ready)
	m.logger.Debug("started")

	var err error

	for {
		select {
		case <-signals:
			m.logger.Info("stopping")
			return nil
		case <-time.After(time.Second):
			if atomic.LoadInt32(&m.stopWatching) == 0 {
				err = watchPID(m.haproxyPIDFile, m.logger)
				if err != nil {
					m.logger.Info("stopping")
					return err
				}
			}
		}
	}
}

func watchPID(pidFile string, logger lager.Logger) error {
	fileBytes, err := ioutil.ReadFile(pidFile)
	if err != nil {
		logger.Error("exiting", fmt.Errorf("Cannot read file %s", pidFile))
		return err
	}
	data := strings.TrimSpace(string(fileBytes))
	pid, err := strconv.Atoi(data)
	if err != nil {
		logger.Error("exiting", fmt.Errorf("Cannot convert file %s to integer", pidFile), lager.Data{"contents": data})
		return err
	}

	if !running(pid) {
		err := fmt.Errorf("PID %d not found", pid)
		logger.Error("exiting", err)
		return err
	}
	return nil

}

func running(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	} else {
		err := process.Signal(syscall.Signal(0))
		if err != nil {
			return false
		}
	}

	return true
}
