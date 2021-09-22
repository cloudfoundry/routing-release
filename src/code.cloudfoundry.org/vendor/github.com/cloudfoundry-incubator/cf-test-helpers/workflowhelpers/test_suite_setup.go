package workflowhelpers

import (
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/commandstarter"
	"github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers/internal"
)

type remoteResource interface {
	Create()
	Destroy()
	ShouldRemain() bool
}

type testSuiteConfig interface {
	internal.AdminUserConfig
	internal.SpaceAndOrgConfig
	internal.UserConfig
	internal.AdminClientConfig
	internal.ClientConfig

	GetApiEndpoint() string
	GetSkipSSLValidation() bool

	GetNamePrefix() string
	GetScaledTimeout(time.Duration) time.Duration
}

type ReproducibleTestSuiteSetup struct {
	shortTimeout time.Duration
	longTimeout  time.Duration

	organizationName string
	spaceName        string

	TestUser  remoteResource
	TestSpace internal.Space

	regularUserContext UserContext
	adminUserContext   UserContext

	SkipSSLValidation bool

	SkipUserCreation bool

	originalCfHomeDir string
	currentCfHomeDir  string
}

const RUNAWAY_QUOTA_MEM_LIMIT = "99999G"

func NewTestSuiteSetup(config testSuiteConfig) *ReproducibleTestSuiteSetup {
	testSpace := internal.NewRegularTestSpace(config, "10G")

	return NewTestContextSuiteSetup(config, testSpace, config.GetUseExistingUser())
}

func NewSmokeTestSuiteSetup(config testSuiteConfig) *ReproducibleTestSuiteSetup {
	testSpace := internal.NewRegularTestSpace(config, "10G")

	return NewTestContextSuiteSetup(config, testSpace, true)
}

func NewRunawayAppTestSuiteSetup(config testSuiteConfig) *ReproducibleTestSuiteSetup {
	testSpace := internal.NewRegularTestSpace(config, RUNAWAY_QUOTA_MEM_LIMIT)

	return NewTestContextSuiteSetup(config, testSpace, config.GetUseExistingUser())
}

func NewTestContextSuiteSetup(config testSuiteConfig, testSpace internal.Space, skipUserCreation bool) *ReproducibleTestSuiteSetup {
	var testUser *internal.TestUser
	useTestClient := false
	if config.GetExistingClient() != "" && config.GetExistingClientSecret() != "" {
		testUser = internal.NewTestClient(config, commandstarter.NewCommandStarter())
		skipUserCreation = true
		useTestClient = true
	} else {
		testUser = internal.NewTestUser(config, commandstarter.NewCommandStarter())
	}

	var adminUser *internal.TestUser
	useAdminClient := false
	if config.GetAdminClient() != "" && config.GetAdminClientSecret() != "" {
		adminUser = internal.NewAdminClient(config, commandstarter.NewCommandStarter())
		useAdminClient = true
	} else {
		adminUser = internal.NewAdminUser(config, commandstarter.NewCommandStarter())
	}

	shortTimeout := config.GetScaledTimeout(1 * time.Minute)
	regularUserContext := NewUserContext(config.GetApiEndpoint(), testUser, testSpace, config.GetSkipSSLValidation(), shortTimeout)
	adminUserContext := NewUserContext(config.GetApiEndpoint(), adminUser, nil, config.GetSkipSSLValidation(), shortTimeout)
	regularUserContext.UseClientCredentials = useTestClient
	adminUserContext.UseClientCredentials = useAdminClient

	return NewBaseTestSuiteSetup(config, testSpace, testUser, regularUserContext, adminUserContext, skipUserCreation)
}

func NewBaseTestSuiteSetup(config testSuiteConfig, testSpace internal.Space, testUser remoteResource, regularUserContext, adminUserContext UserContext, skipUserCreation bool) *ReproducibleTestSuiteSetup {
	shortTimeout := config.GetScaledTimeout(1 * time.Minute)

	return &ReproducibleTestSuiteSetup{
		shortTimeout: shortTimeout,
		longTimeout:  config.GetScaledTimeout(5 * time.Minute),

		regularUserContext: regularUserContext,
		adminUserContext:   adminUserContext,

		SkipUserCreation: skipUserCreation,
		TestSpace:        testSpace,
		TestUser:         testUser,
	}
}

func (testSetup ReproducibleTestSuiteSetup) GetOrganizationName() string {
	return testSetup.TestSpace.OrganizationName()
}

func (testSetup ReproducibleTestSuiteSetup) ShortTimeout() time.Duration {
	return testSetup.shortTimeout
}

func (testSetup ReproducibleTestSuiteSetup) LongTimeout() time.Duration {
	return testSetup.longTimeout
}

func (testSetup *ReproducibleTestSuiteSetup) Setup() {
	AsUser(testSetup.AdminUserContext(), testSetup.shortTimeout, func() {
		testSetup.TestSpace.Create()
		if !testSetup.SkipUserCreation {
			testSetup.TestUser.Create()
		}
		if !testSetup.RegularUserContext().UseClientCredentials {
			testSetup.regularUserContext.AddUserToSpace()
		}
	})
	testSetup.originalCfHomeDir, testSetup.currentCfHomeDir = testSetup.regularUserContext.SetCfHomeDir()
	testSetup.regularUserContext.Login()
	testSetup.regularUserContext.TargetSpace()
}

func (testSetup *ReproducibleTestSuiteSetup) Teardown() {
	testSetup.regularUserContext.Logout()
	testSetup.regularUserContext.UnsetCfHomeDir(testSetup.originalCfHomeDir, testSetup.currentCfHomeDir)

	AsUser(testSetup.AdminUserContext(), testSetup.shortTimeout, func() {
		if !testSetup.TestUser.ShouldRemain() {
			if !testSetup.SkipUserCreation {
				testSetup.TestUser.Destroy()
			}
		}

		testSetup.TestSpace.Destroy()
	})
}

func (testSetup *ReproducibleTestSuiteSetup) AdminUserContext() UserContext {
	return testSetup.adminUserContext
}

func (testSetup *ReproducibleTestSuiteSetup) RegularUserContext() UserContext {
	return testSetup.regularUserContext
}
