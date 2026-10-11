package internal

import (
	"os/exec"
	"time"
)

// TeeReporter fans Report to every reporter; ReportCompletion reaches only
// those that also implement CompletionReporter.
type TeeReporter struct {
	reporters []Reporter
}

var _ Reporter = new(TeeReporter)
var _ CompletionReporter = new(TeeReporter)

func NewTeeReporter(reporters ...Reporter) *TeeReporter {
	return &TeeReporter{reporters: reporters}
}

func (t *TeeReporter) Report(startTime time.Time, cmd *exec.Cmd) {
	for _, r := range t.reporters {
		r.Report(startTime, cmd)
	}
}

func (t *TeeReporter) ReportCompletion(cmd *exec.Cmd, elapsed time.Duration, exitCode int) {
	for _, r := range t.reporters {
		if cr, ok := r.(CompletionReporter); ok {
			cr.ReportCompletion(cmd, elapsed, exitCode)
		}
	}
}
