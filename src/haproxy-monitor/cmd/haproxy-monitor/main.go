package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"haproxy-monitor/watcher"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/pivotal-golang/lager"
)

var pidFile = flag.String("pidFile", "", "path to monitored process's pid file")

func main() {
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cf_lager.New("haproxy-monitor")
	if *pidFile == "" {
		logger.Error("flag-parsing", errors.New("pidfile-not-found"))
		os.Exit(1)
	}

	for {
		fileBytes, err := ioutil.ReadFile(*pidFile)
		if err != nil {
			logger.Error("exiting", fmt.Errorf("Cannot read file %s", *pidFile))
			os.Exit(1)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(fileBytes)))
		if err != nil {
			logger.Error("exiting", fmt.Errorf("Cannot convert file %s to integer", *pidFile))
			os.Exit(1)
		}
		logger.Info("checking-pid", lager.Data{"pid": pid})
		if !watcher.Running(pid) {
			logger.Error("exiting", fmt.Errorf("PID %d not found", pid))
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
}
