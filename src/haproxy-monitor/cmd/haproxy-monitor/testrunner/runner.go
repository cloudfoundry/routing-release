package testrunner

import (
	"os/exec"

	"github.com/tedsuo/ifrit/ginkgomon"
)

type Args struct {
	PidFile string
}

func (args Args) ArgSlice() []string {
	return []string{
		"--pidFile", args.PidFile,
	}
}

func New(binPath string, args Args) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name:    "haproxy-monitor",
		Command: exec.Command(binPath, args.ArgSlice()...),
	})
}
