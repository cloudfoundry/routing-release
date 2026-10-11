package internal

import (
	"os/exec"
	"strings"
	"time"
)

// ObservingReporter is the only type that touches the observer; it redacts the
// args before forwarding, keeping redaction inside the reporter.
type ObservingReporter struct {
	observer CommandObserver
	redactor Redactor
}

var _ Reporter = new(ObservingReporter)
var _ CompletionReporter = new(ObservingReporter)

func NewObservingReporter(observer CommandObserver, redactor Redactor) *ObservingReporter {
	return &ObservingReporter{
		observer: observer,
		redactor: redactor,
	}
}

func (r *ObservingReporter) redactedArgs(cmd *exec.Cmd) string {
	return r.redactor.Redact(strings.Join(cmd.Args, " "))
}

func (r *ObservingReporter) Report(startTime time.Time, cmd *exec.Cmd) {
	r.observer.CommandStarted(r.redactedArgs(cmd), startTime)
}

func (r *ObservingReporter) ReportCompletion(cmd *exec.Cmd, elapsed time.Duration, exitCode int) {
	r.observer.CommandCompleted(r.redactedArgs(cmd), elapsed, exitCode)
}
