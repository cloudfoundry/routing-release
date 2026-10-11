package internal

import (
	"os/exec"
	"time"
)

type Reporter interface {
	Report(time.Time, *exec.Cmd)
}

type CompletionReporter interface {
	ReportCompletion(cmd *exec.Cmd, elapsed time.Duration, exitCode int)
}

// CommandObserver receives a redacted joined-args identity, never raw args.
type CommandObserver interface {
	CommandStarted(redactedArgs string, startTime time.Time)
	CommandCompleted(redactedArgs string, elapsed time.Duration, exitCode int)
}
