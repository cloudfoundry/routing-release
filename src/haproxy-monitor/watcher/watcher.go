package watcher

import (
	"os"
	"syscall"
)

func Running(pid int) bool {
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
