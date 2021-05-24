package haproxy

import (
	"os/exec"

	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o fakes/fake_script_runner.go . ScriptRunner
type ScriptRunner interface {
	Run() error
}

type CommandRunner struct {
	scriptPath string
	logger     lager.Logger
}

func CreateCommandRunner(scriptPath string, logger lager.Logger) *CommandRunner {
	return &CommandRunner{scriptPath, logger}
}

func (cmd *CommandRunner) Run() error {
	output, err := exec.Command(cmd.scriptPath).CombinedOutput()
	cmd.logger.Info("running-script", lager.Data{"output": string(output)})
	return err
}
