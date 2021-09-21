package internal

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"io"

	"github.com/onsi/ginkgo/config"
)

const timeFormat string = "2006-01-02 15:04:05.00 (MST)"

type RedactingReporter struct {
	writer   io.Writer
	redactor Redactor
}

func NewRedactingReporter(writer io.Writer, redactor Redactor) Reporter {
	return &RedactingReporter{
		writer:   writer,
		redactor: redactor,
	}
}

func (r *RedactingReporter) Report(startTime time.Time, cmd *exec.Cmd) {
	startColor := ""
	endColor := ""
	if !config.DefaultReporterConfig.NoColor {
		startColor = "\x1b[32m"
		endColor = "\x1b[0m"
	}

	fmt.Fprintf(
		r.writer,
		"\n%s[%s]> %s %s\n",
		startColor,
		startTime.UTC().Format(timeFormat),
		r.redactor.Redact(strings.Join(cmd.Args, " ")),
		endColor,
	)
}
