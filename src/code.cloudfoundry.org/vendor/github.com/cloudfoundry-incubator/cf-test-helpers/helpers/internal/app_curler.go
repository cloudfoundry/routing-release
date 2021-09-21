package helpersinternal

import (
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

type uriCreator interface {
	AppUri(appName, path string) string
}

type AppCurler struct {
	CurlFunc   func(CurlConfig, ...string) *gexec.Session
	UriCreator uriCreator
}

func NewAppCurler(curlFunc func(CurlConfig, ...string) *gexec.Session, cfg CurlConfig) *AppCurler {
	uriCreator := &AppUriCreator{CurlConfig: cfg}
	return &AppCurler{
		UriCreator: uriCreator,
		CurlFunc:   curlFunc,
	}
}

func (appCurler *AppCurler) CurlAndWait(cfg CurlConfig, appName string, path string, timeout time.Duration, args ...string) string {
	appUri := appCurler.UriCreator.AppUri(appName, path)
	curlArgs := append([]string{appUri}, args...)

	curlCmd := appCurler.CurlFunc(cfg, curlArgs...).Wait(timeout)

	ExpectWithOffset(3, curlCmd).To(gexec.Exit(0))
	ExpectWithOffset(3, string(curlCmd.Err.Contents())).To(HaveLen(0))
	return string(curlCmd.Out.Contents())
}
