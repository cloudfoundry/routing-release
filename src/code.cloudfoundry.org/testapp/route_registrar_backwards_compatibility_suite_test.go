package main_test

import (
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRouteRegistrarBackwardsCompatibility(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Route Registrar Backwards Compatability Suite")
}

var domain string
var encryptedURL string
var unencryptedURL string
var TCPURL string
var isLocal bool

var _ = BeforeSuite(func() {
	domain = os.Getenv("DOMAIN")
	if domain == "" {
		panic("must set DOMAIN")
	}

	if os.Getenv("RUN_LOCALLY") == "true" {
		isLocal = true
		unencryptedURL = "localhost:8081/http"
		encryptedURL = "localhost:8082/https"
		TCPURL = "localhost:1033"
	} else {
		isLocal = false
		unencryptedURL = fmt.Sprintf("https://unencrypted.%s/http", domain)
		encryptedURL = fmt.Sprintf("https://encrypted.%s/https", domain)
		TCPURL = fmt.Sprintf("tcp.%s:1033", domain)
	}
})
