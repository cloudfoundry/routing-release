package internal

import (
	"os/exec"
	"time"
)

type Reporter interface {
	Report(time.Time, *exec.Cmd)
}
