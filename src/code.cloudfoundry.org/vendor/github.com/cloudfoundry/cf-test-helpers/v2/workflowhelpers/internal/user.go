package internal

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/v2/generator"
	"github.com/cloudfoundry/cf-test-helpers/v2/internal"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

type TestUser struct {
	username       string
	password       string
	origin         string
	cmdStarter     internal.Starter
	timeout        time.Duration
	shouldKeepUser bool
}

type UserConfig interface {
	GetUseExistingUser() bool
	GetExistingUser() string
	GetExistingUserPassword() string
	GetUserOrigin() string
	GetShouldKeepUser() bool
	GetConfigurableTestPassword() string
}

type userConfig interface {
	UserConfig

	GetScaledTimeout(time.Duration) time.Duration
	GetNamePrefix() string
}

type AdminUserConfig interface {
	GetAdminUser() string
	GetAdminPassword() string
	GetAdminOrigin() string
}

type ClientConfig interface {
	GetExistingClient() string
	GetExistingClientSecret() string
}

type AdminClientConfig interface {
	GetAdminClient() string
	GetAdminClientSecret() string
}

func NewTestUser(config userConfig, cmdStarter internal.Starter) *TestUser {
	var regUser, regUserPass, regUserOrigin string

	if config.GetUseExistingUser() {
		regUser = config.GetExistingUser()
		regUserPass = config.GetExistingUserPassword()
		regUserOrigin = config.GetUserOrigin()
	} else {
		regUser = generator.PrefixedRandomName(config.GetNamePrefix(), "USER")
		regUserPass = generatePassword()
		regUserOrigin = config.GetUserOrigin()
	}

	if config.GetConfigurableTestPassword() != "" {
		regUserPass = config.GetConfigurableTestPassword()
	}

	return &TestUser{
		username:       regUser,
		password:       regUserPass,
		origin:         regUserOrigin,
		cmdStarter:     cmdStarter,
		timeout:        config.GetScaledTimeout(1 * time.Minute),
		shouldKeepUser: config.GetShouldKeepUser(),
	}
}

func NewAdminUser(config AdminUserConfig, cmdStarter internal.Starter) *TestUser {
	return &TestUser{
		username:   config.GetAdminUser(),
		password:   config.GetAdminPassword(),
		origin:     config.GetAdminOrigin(),
		cmdStarter: cmdStarter,
	}
}

func NewAdminClient(config AdminClientConfig, cmdStarter internal.Starter) *TestUser {
	return &TestUser{
		username:   config.GetAdminClient(),
		password:   config.GetAdminClientSecret(),
		cmdStarter: cmdStarter,
	}
}

func NewTestClient(config ClientConfig, cmdStarter internal.Starter) *TestUser {
	return &TestUser{
		username:   config.GetExistingClient(),
		password:   config.GetExistingClientSecret(),
		cmdStarter: cmdStarter,
	}
}

func (user *TestUser) Create() {
	redactor := internal.NewRedactor(user.password)
	redactingReporter := internal.NewRedactingReporter(ginkgo.GinkgoWriter, redactor)

	session := internal.CfWithCustomReporter(user.cmdStarter, redactingReporter, "create-user", user.username, user.password)
	EventuallyWithOffset(1, session, user.timeout).Should(Exit(), "Failed to create user")

	if session.ExitCode() != 0 {
		ExpectWithOffset(1, combineOutputAndRedact(session, redactor)).Should(Say("scim_resource_already_exists"), "Failed to create user")
	}
}

func (user *TestUser) Destroy() {
	session := internal.Cf(user.cmdStarter, "delete-user", "-f", user.username)
	EventuallyWithOffset(1, session, user.timeout).Should(Exit(0), "Failed to delete user")
}

func (user *TestUser) Username() string {
	return user.username
}

func (user *TestUser) Password() string {
	return user.password
}

func (user *TestUser) Origin() string {
	return user.origin
}

func (user *TestUser) ShouldRemain() bool {
	return user.shouldKeepUser
}

func combineOutputAndRedact(session *Session, redactor internal.Redactor) *Buffer {
	stdout := redactor.Redact(string(session.Out.Contents()))
	stderr := redactor.Redact(string(session.Err.Contents()))

	return BufferWithBytes(append([]byte(stdout), []byte(stderr)...))
}

// The key thing that makes a password secure is the _entropy_ that comes from a
// generator of true random numbers.  But many password rules require a mixure
// of cases, numbers and special characters.  Here we meet these rules by starting
// the password with the required upper/lower case, number and special.  Then we make
// it secure by adding truly random characters.
func generatePassword() string {
	const randomBytesLength = 16
	encoding := base64.RawURLEncoding

	randomBytes := make([]byte, encoding.DecodedLen(randomBytesLength))
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic(fmt.Errorf("Could not generate random password: %s", err.Error()))
	}

	return "A0a!" + encoding.EncodeToString(randomBytes)
}
