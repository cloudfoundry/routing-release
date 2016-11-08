package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"haproxy-monitor/pid"
	"haproxy-monitor/watcher"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/lager"
)

var pidFile = flag.String("pidFile", "/var/vcap/sys/run/haproxy/pid", "path to monitored process's pid file")

func main() {
	cflager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cflager.New("haproxy-monitor")
	if *pidFile == "" {
		logger.Error("flag-parsing", errors.New("pidfile-not-found"))
		os.Exit(1)
	}

	logger.Info("starting-monitor", lager.Data{"pid-file": *pidFile})

	f, err := os.Open(*pidFile)
	if err != nil {
		logger.Error("exiting", err, lager.Data{"path": *pidFile})
		os.Exit(1)
	}
	defer f.Close()

	fileLock := pid.NewFileLock(f)
	for {
		pid, err := pid.GetPid(fileLock)
		if err != nil {
			logger.Error("exiting", err)
			os.Exit(1)
		}
		logger.Debug("checking-pid", lager.Data{"pid": pid})
		if !watcher.Running(pid) {
			logger.Error("exiting", fmt.Errorf("PID %d not found", pid))
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
}
