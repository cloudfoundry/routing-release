package helpersinternal

import "strings"

type AppUriCreator struct {
	CurlConfig CurlConfig
}

func (uriCreator *AppUriCreator) AppUri(appName string, path string) string {
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	var subdomain string
	if appName != "" {
		subdomain = appName + "."
	}

	return uriCreator.CurlConfig.Protocol() + subdomain + uriCreator.CurlConfig.GetAppsDomain() + path
}
