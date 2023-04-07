package internal

import (
	"fmt"
	"os"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/v2/internal"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

const VerboseAuth = "RELINT_VERBOSE_AUTH"
const CFAuthRetries = 2

func CfAuth(cmdStarter internal.Starter, reporter internal.Reporter, user string, password string, origin string, timeout time.Duration) error {
	args := []string{"auth", user, password}

	if origin != "" {
		args = append(args, "--origin", origin)
	}

	if os.Getenv(VerboseAuth) == "true" {
		args = append(args, "-v")
	}

	return executeAuthWithRetries(cmdStarter, reporter, args, timeout)
}

func CfClientAuth(cmdStarter internal.Starter, reporter internal.Reporter, client string, clientSecret string, timeout time.Duration) error {
	args := []string{"auth", client, clientSecret, "--client-credentials"}

	return executeAuthWithRetries(cmdStarter, reporter, args, timeout)
}

func executeAuthWithRetries(cmdStarter internal.Starter, reporter internal.Reporter, args []string, timeout time.Duration) error {
	var auth *gexec.Session
	var err error
	var failures []string

	for i := 0; i < CFAuthRetries; i++ {
		auth, err = cmdStarter.Start(reporter, "cf", args...)
		if err != nil {
			return err
		}

		failures = InterceptGomegaFailures(func() {
			auth.Wait(timeout)
		})

		if len(failures) == 0 && auth.ExitCode() == 0 {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	if len(failures) != 0 {
		return fmt.Errorf("cf auth command timed out: %s", failures)
	}

	if auth.ExitCode() != 0 {
		return fmt.Errorf("cf auth command exited with %d", auth.ExitCode())
	}

	return nil
}
