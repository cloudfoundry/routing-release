package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	ApiEndpoint string `json:"api"`
	AppsDomain  string `json:"apps_domain"`
	UseHttp     bool   `json:"use_http"`

	AdminUser     string `json:"admin_user"`
	AdminPassword string `json:"admin_password"`
	AdminOrigin   string `json:"admin_origin"`

	AdminClient       string `json:"admin_client"`
	AdminClientSecret string `json:"admin_client_secret"`

	UseExistingUser      bool   `json:"use_existing_user"`
	ShouldKeepUser       bool   `json:"keep_user_at_suite_end"`
	ExistingUser         string `json:"existing_user"`
	ExistingUserPassword string `json:"existing_user_password"`

	UserOrigin string `json:"user_origin"`

	ExistingClient       string `json:"existing_client"`
	ExistingClientSecret string `json:"existing_client_secret"`

	ConfigurableTestPassword string `json:"test_password"`

	UseExistingOrganization bool   `json:"use_existing_organization"`
	ExistingOrganization    string `json:"existing_organization"`

	AddExistingUserToExistingSpace bool   `json:"add_existing_user_to_existing_space"`
	UseExistingSpace               bool   `json:"use_existing_space"`
	ExistingSpace                  string `json:"existing_space"`

	SkipSSLValidation bool   `json:"skip_ssl_validation"`
	Backend           string `json:"backend"`

	ArtifactsDirectory string `json:"artifacts_directory"`

	DefaultTimeout               int `json:"default_timeout"`
	SleepTimeout                 int `json:"sleep_timeout"`
	DetectTimeout                int `json:"detect_timeout"`
	CfPushTimeout                int `json:"cf_push_timeout"`
	LongCurlTimeout              int `json:"long_curl_timeout"`
	BrokerStartTimeout           int `json:"broker_start_timeout"`
	AsyncServiceOperationTimeout int `json:"async_service_operation_timeout"`

	TimeoutScale float64 `json:"timeout_scale"`

	SecureAddress string `json:"secure_address"`

	DockerExecutable      string   `json:"docker_executable"`
	DockerParameters      []string `json:"docker_parameters"`
	DockerRegistryAddress string   `json:"docker_registry_address"`
	DockerPrivateImage    string   `json:"docker_private_image"`
	DockerUser            string   `json:"docker_user"`
	DockerPassword        string   `json:"docker_password"`
	DockerEmail           string   `json:"docker_email"`

	StaticFileBuildpackName string `json:"staticfile_buildpack_name"`
	JavaBuildpackName       string `json:"java_buildpack_name"`
	RubyBuildpackName       string `json:"ruby_buildpack_name"`
	NodejsBuildpackName     string `json:"nodejs_buildpack_name"`
	GoBuildpackName         string `json:"go_buildpack_name"`
	PythonBuildpackName     string `json:"python_buildpack_name"`
	PhpBuildpackName        string `json:"php_buildpack_name"`
	BinaryBuildpackName     string `json:"binary_buildpack_name"`

	IncludeApps                bool `json:"include_apps"`
	IncludeBackendCompatiblity bool `json:"include_backend_compatibility"`
	IncludeDetect              bool `json:"include_detect"`
	IncludeDocker              bool `json:"include_docker"`
	IncludeInternetDependent   bool `json:"include_internet_dependent"`
	IncludeRouteServices       bool `json:"include_route_services"`
	IncludeRouting             bool `json:"include_routing"`
	IncludeSecurityGroups      bool `json:"include_security_groups"`
	IncludeServices            bool `json:"include_services"`
	IncludeSsh                 bool `json:"include_ssh"`
	IncludeV3                  bool `json:"include_v3"`
	IncludeTasks               bool `json:"include_tasks"`
	IncludeSSO                 bool `json:"include_sso"`

	NamePrefix string `json:"name_prefix"`
}

var defaults = Config{
	StaticFileBuildpackName: "staticfile_buildpack",
	JavaBuildpackName:       "java_buildpack",
	RubyBuildpackName:       "ruby_buildpack",
	NodejsBuildpackName:     "nodejs_buildpack",
	GoBuildpackName:         "go_buildpack",
	PythonBuildpackName:     "python_buildpack",
	PhpBuildpackName:        "php_buildpack",
	BinaryBuildpackName:     "binary_buildpack",

	IncludeApps:                    true,
	IncludeBackendCompatiblity:     true,
	IncludeDetect:                  true,
	IncludeDocker:                  true,
	IncludeInternetDependent:       true,
	IncludeRouteServices:           true,
	IncludeRouting:                 true,
	IncludeSecurityGroups:          true,
	IncludeServices:                true,
	IncludeSsh:                     true,
	IncludeV3:                      true,
	AddExistingUserToExistingSpace: true,

	DefaultTimeout:               30,
	CfPushTimeout:                2,
	LongCurlTimeout:              2,
	BrokerStartTimeout:           5,
	AsyncServiceOperationTimeout: 2,
	DetectTimeout:                5,
	SleepTimeout:                 30,

	ArtifactsDirectory: filepath.Join("..", "results"),

	NamePrefix: "CATS",
}

func (c Config) GetScaledTimeout(timeout time.Duration) time.Duration {
	return time.Duration(float64(timeout) * c.TimeoutScale)
}

var loadedConfig *Config

func Load(path string, config *Config) error {
	err := loadConfigFromPath(path, config)
	if err != nil {
		return err
	}

	if config.ApiEndpoint == "" {
		return fmt.Errorf("missing configuration 'api'")
	}

	if config.AdminUser == "" {
		return fmt.Errorf("missing configuration 'admin_user'")
	}

	if config.AdminPassword == "" {
		return fmt.Errorf("missing configuration 'admin_password'")
	}

	if config.TimeoutScale <= 0 {
		config.TimeoutScale = 1.0
	}

	return nil
}

func LoadConfig() *Config {
	if loadedConfig != nil {
		return loadedConfig
	}

	loadedConfig = &defaults
	err := Load(ConfigPath(), loadedConfig)
	if err != nil {
		panic(err)
	}
	return loadedConfig
}

func (c Config) Protocol() string {
	if c.UseHttp {
		return "http://"
	} else {
		return "https://"
	}
}

func loadConfigFromPath(path string, config interface{}) error {
	configFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer configFile.Close()

	decoder := json.NewDecoder(configFile)
	return decoder.Decode(config)
}

func ConfigPath() string {
	path := os.Getenv("CONFIG")
	if path == "" {
		panic("Must set $CONFIG to point to an integration config .json file.")
	}

	return path
}

func (c *Config) DefaultTimeoutDuration() time.Duration {
	return time.Duration(c.DefaultTimeout) * time.Second
}
func (c *Config) SleepTimeoutDuration() time.Duration {
	return time.Duration(c.SleepTimeout) * time.Second
}

func (c *Config) DetectTimeoutDuration() time.Duration {
	return time.Duration(c.DetectTimeout) * time.Minute
}

func (c *Config) CfPushTimeoutDuration() time.Duration {
	return time.Duration(c.CfPushTimeout) * time.Minute
}

func (c *Config) LongCurlTimeoutDuration() time.Duration {
	return time.Duration(c.LongCurlTimeout) * time.Minute
}

func (c *Config) BrokerStartTimeoutDuration() time.Duration {
	return time.Duration(c.BrokerStartTimeout) * time.Minute
}

func (c *Config) AsyncServiceOperationTimeoutDuration() time.Duration {
	return time.Duration(c.AsyncServiceOperationTimeout) * time.Minute
}

func (c *Config) GetAppsDomain() string {
	return c.AppsDomain
}

func (c *Config) GetSkipSSLValidation() bool {
	return c.SkipSSLValidation
}

func (c *Config) GetArtifactsDirectory() string {
	return c.ArtifactsDirectory
}

func (c *Config) GetNamePrefix() string {
	return c.NamePrefix
}

func (c *Config) GetUseExistingUser() bool {
	return c.UseExistingUser
}

func (c *Config) GetAddExistingUserToExistingSpace() bool {
	return c.AddExistingUserToExistingSpace
}

func (c *Config) GetUseExistingSpace() bool {
	return c.UseExistingSpace
}

func (c *Config) GetExistingUser() string {
	return c.ExistingUser
}

func (c *Config) GetExistingUserPassword() string {
	return c.ExistingUserPassword
}

func (c *Config) GetUserOrigin() string {
	return c.UserOrigin
}

func (c *Config) GetConfigurableTestPassword() string {
	return c.ConfigurableTestPassword
}

func (c *Config) GetShouldKeepUser() bool {
	return c.ShouldKeepUser
}

func (c *Config) GetAdminUser() string {
	return c.AdminUser
}

func (c *Config) GetAdminPassword() string {
	return c.AdminPassword
}

func (c *Config) GetAdminOrigin() string {
	return c.AdminOrigin
}

func (c *Config) GetUseExistingOrganization() bool {
	return c.UseExistingOrganization
}

func (c *Config) GetExistingOrganization() string {
	return c.ExistingOrganization
}

func (c *Config) GetExistingSpace() string {
	return c.ExistingSpace
}

func (c *Config) GetApiEndpoint() string {
	return c.ApiEndpoint
}

func (c *Config) GetAdminClient() string {
	return c.AdminClient
}

func (c *Config) GetAdminClientSecret() string {
	return c.AdminClientSecret
}

func (c *Config) GetExistingClient() string {
	return c.ExistingClient
}

func (c *Config) GetExistingClientSecret() string {
	return c.ExistingClientSecret
}
