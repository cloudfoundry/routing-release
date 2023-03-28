package helpers

import (
	"time"

	helpersinternal "github.com/cloudfoundry/cf-test-helpers/v2/helpers/internal"
)

const CURL_TIMEOUT = 60 * time.Second

// Gets an app's endpoint with the specified path
func AppUri(appName, path string, config helpersinternal.CurlConfig) string {
	uriCreator := &helpersinternal.AppUriCreator{CurlConfig: config}

	return uriCreator.AppUri(appName, path)
}

// Curls an app's endpoint and exit successfully before the specified timeout
func CurlAppWithTimeout(cfg helpersinternal.CurlConfig, appName, path string, timeout time.Duration, args ...string) string {
	appCurler := helpersinternal.NewAppCurler(Curl, cfg)
	return appCurler.CurlAndWait(cfg, appName, path, timeout, args...)
}

// Curls an app's endpoint and exit successfully before the default timeout
func CurlApp(cfg helpersinternal.CurlConfig, appName, path string, args ...string) string {
	appCurler := helpersinternal.NewAppCurler(Curl, cfg)
	return appCurler.CurlAndWait(cfg, appName, path, CURL_TIMEOUT, args...)
}

// Curls an app's root endpoint and exit successfully before the default timeout
func CurlAppRoot(cfg helpersinternal.CurlConfig, appName string) string {
	appCurler := helpersinternal.NewAppCurler(Curl, cfg)
	return appCurler.CurlAndWait(cfg, appName, "/", CURL_TIMEOUT)
}

// Returns a function that curls an app's root endpoint and exit successfully before the default timeout
func CurlingAppRoot(cfg helpersinternal.CurlConfig, appName string) func() string {
	appCurler := helpersinternal.NewAppCurler(Curl, cfg)
	return func() string { return appCurler.CurlAndWait(cfg, appName, "/", CURL_TIMEOUT) }
}
