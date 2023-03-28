package commandreporter

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
)

const timeFormat = "2006-01-02 15:04:05.00 (MST)"

type CommandReporter struct {
	Writer io.Writer
}

func NewCommandReporter(writers ...io.Writer) *CommandReporter {
	var writer io.Writer
	switch len(writers) {
	case 0:
		writer = ginkgo.GinkgoWriter
	case 1:
		writer = writers[0]
	default:
		panic("NewCommandReporter should only take one writer")
	}

	return &CommandReporter{
		Writer: writer,
	}
}

func (r *CommandReporter) Report(startTime time.Time, cmd *exec.Cmd) {
	startColor := ""
	endColor := ""
	_, reporterConfig := ginkgo.GinkgoConfiguration()
	if !reporterConfig.NoColor {
		startColor = "\x1b[32m"
		endColor = "\x1b[0m"
	}

	fmt.Fprintf(
		r.Writer,
		"\n%s[%s]> %s %s\n",
		startColor,
		startTime.UTC().Format(timeFormat),
		strings.Join(cmd.Args, " "),
		endColor,
	)
}
