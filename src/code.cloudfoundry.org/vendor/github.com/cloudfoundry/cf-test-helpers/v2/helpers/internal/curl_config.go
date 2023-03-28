package helpersinternal

type CurlConfig interface {
	GetAppsDomain() string
	Protocol() string
	GetSkipSSLValidation() bool
}
