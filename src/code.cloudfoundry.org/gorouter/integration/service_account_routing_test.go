package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	mrand "math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/mbus"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var (
	instanceKeyOnce sync.Once
	instanceKey     *rsa.PrivateKey
)

// instanceCertWithSANs signs a Diego-style instance identity certificate with
// the given DNS SANs, which test_util's helper (a single DNS name) cannot do.
func instanceCertWithSANs(ca *test_util.CertChain, ous, dnsNames []string) tls.Certificate {
	instanceKeyOnce.Do(func() {
		var err error
		instanceKey, err = rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
	})

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization:       []string{"Cloud Foundry"},
			OrganizationalUnit: ous,
			CommonName:         "caller-instance",
		},
		DNSNames:              dnsNames,
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.CACert, &instanceKey.PublicKey, ca.CAPrivKey)
	Expect(err).NotTo(HaveOccurred())
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: instanceKey}
}

// registerWithRoutePolicies registers a backend whose route policies are the
// given raw sources, so tests can use source types the older helpers cannot.
func (s *testState) registerWithRoutePolicies(backend *httptest.Server, routeURI, scope string, sources []string, tags map[string]string) {
	_, backendPort := hostnameAndPort(backend.Listener.Addr().String())
	s.registerAndWait(mbus.RegistryMessage{
		Host:                    "127.0.0.1",
		Port:                    uint16(backendPort),
		Uris:                    []route.Uri{route.Uri(routeURI)},
		StaleThresholdInSeconds: 10,
		PrivateInstanceID:       fmt.Sprintf("%x", mrand.Int31()),
		Tags:                    tags,
		Options: mbus.RegistryMessageOpts{
			RoutePolicyScope:   scope,
			RoutePolicySources: strings.Join(sources, ","),
		},
	})
}

var _ = Describe("Service-account route policies", func() {
	const (
		accountSAN      = "payments-worker.acme-payments.svc.identity"
		accountSource   = "cf:svc:acme-payments:payments-worker"
		otherAccountSAN = "other-worker.acme-payments.svc.identity"
		callerApp       = "caller-app-guid"
		callerSpace     = "caller-space-guid"
		callerOrg       = "caller-org-guid"
		xfccHeader      = "X-Forwarded-Client-Cert"
	)

	var (
		testState *testState
		domainCA  *test_util.CertChain
		backend   *httptest.Server
		received  chan *http.Request
	)

	BeforeEach(func() {
		testState = NewTestState()

		domainCA = &test_util.CertChain{}
		*domainCA = test_util.CreateSignedCertWithRootCA(test_util.CertNames{CommonName: "Diego Instance Identity CA"})

		received = make(chan *http.Request, 10)
		backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received <- r
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("authorized"))
		}))

		testState.cfg.EnableSSL = true
		testState.cfg.ClientCertificateValidationString = "request"
		testState.cfg.Domains = []config.MtlsDomainConfig{
			{Domain: "*.raw.apps.mtls.internal", CACerts: string(domainCA.CACertPEM), ForwardedClientCert: config.SANITIZE_SET, XFCCFormat: config.XFCC_FORMAT_RAW},
			{Domain: "*.envoy.apps.mtls.internal", CACerts: string(domainCA.CACertPEM), ForwardedClientCert: config.SANITIZE_SET, XFCCFormat: config.XFCC_FORMAT_ENVOY},
			{Domain: "*.forward.apps.mtls.internal", CACerts: string(domainCA.CACertPEM), ForwardedClientCert: config.FORWARD, XFCCFormat: config.XFCC_FORMAT_ENVOY},
		}
		testState.StartGorouterOrFail()
	})

	AfterEach(func() {
		if backend != nil {
			backend.Close()
		}
		if testState != nil {
			testState.StopAndCleanup()
		}
	})

	callerCert := func(dnsNames ...string) tls.Certificate {
		return instanceCertWithSANs(domainCA,
			[]string{"app:" + callerApp, "space:" + callerSpace, "organization:" + callerOrg}, dnsNames)
	}

	// get makes a request as the caller holding cert, optionally sending a
	// client-supplied XFCC header, and returns the status code.
	get := func(host string, cert tls.Certificate, forgedXFCC string) int {
		testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{cert}
		req, client := testState.newMtlsGetRequest("https://" + host)
		if forgedXFCC != "" {
			req.Header.Set(xfccHeader, forgedXFCC)
		}
		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		return resp.StatusCode
	}

	forgedRawCert := func() string {
		forged := instanceCertWithSANs(domainCA,
			[]string{"app:" + callerApp, "space:" + callerSpace, "organization:" + callerOrg}, []string{accountSAN})
		return base64.StdEncoding.EncodeToString(forged.Certificate[0])
	}

	for _, format := range []struct {
		name, host string
		forged     func() string
	}{
		{
			name: "raw format",
			host: "api.raw.apps.mtls.internal",
			// A raw-format forgery is a whole certificate that carries the account.
			forged: forgedRawCert,
		},
		{
			name: "envoy format",
			host: "api.envoy.apps.mtls.internal",
			forged: func() string {
				return `Hash=abc;SVC=` + accountSAN + `;Subject="CN=x,OU=app:` + callerApp + `,OU=space:` + callerSpace + `,OU=organization:` + callerOrg + `"`
			},
		},
	} {
		format := format

		Describe(format.name, func() {
			type routeCase struct {
				sans     []string
				sources  []string
				scope    string
				tags     map[string]string
				expected int
			}

			DescribeTable("authorizes callers by their service account",
				func(c routeCase) {
					scope := c.scope
					if scope == "" {
						scope = "any"
					}
					testState.registerWithRoutePolicies(backend, format.host, scope, c.sources, c.tags)

					Expect(get(format.host, callerCert(c.sans...), "")).To(Equal(c.expected))
				},
				Entry("allows the matching account", routeCase{
					sans: []string{accountSAN}, sources: []string{accountSource}, expected: http.StatusOK,
				}),
				Entry("allows the matching account beside unrelated SANs", routeCase{
					sans: []string{"caller-instance-guid", "orders.apps.internal", accountSAN}, sources: []string{accountSource}, expected: http.StatusOK,
				}),
				Entry("skips a malformed account rule and honours a valid one", routeCase{
					sans: []string{accountSAN}, sources: []string{"cf:svc:acme-payments", accountSource}, expected: http.StatusOK,
				}),
				Entry("denies a different account", routeCase{
					sans: []string{otherAccountSAN}, sources: []string{accountSource}, expected: http.StatusForbidden,
				}),
				Entry("denies a caller without an account", routeCase{
					sans: nil, sources: []string{accountSource}, expected: http.StatusForbidden,
				}),
				Entry("denies a caller with two accounts", routeCase{
					sans: []string{accountSAN, otherAccountSAN}, sources: []string{accountSource}, expected: http.StatusForbidden,
				}),
				Entry("denies a malformed rule that merely resembles the account", routeCase{
					sans: []string{accountSAN}, sources: []string{"cf:svc:acme-payments"}, expected: http.StatusForbidden,
				}),
				Entry("still honours the domain scope: same space", routeCase{
					sans: []string{accountSAN}, sources: []string{accountSource}, scope: "space",
					tags: map[string]string{"space_id": callerSpace, "organization_id": callerOrg}, expected: http.StatusOK,
				}),
				Entry("still honours the domain scope: another space", routeCase{
					sans: []string{accountSAN}, sources: []string{accountSource}, scope: "space",
					tags: map[string]string{"space_id": "destination-space-guid", "organization_id": callerOrg}, expected: http.StatusForbidden,
				}),
			)

			It("does not accept an account claimed in a client-supplied XFCC header", func() {
				testState.registerWithRoutePolicies(backend, format.host, "any", []string{accountSource}, nil)

				Expect(get(format.host, callerCert(), format.forged())).To(Equal(http.StatusForbidden))
				Consistently(received).ShouldNot(Receive())
			})

			It("does not let a client-supplied header override the account of its real certificate", func() {
				testState.registerWithRoutePolicies(backend, format.host, "any", []string{accountSource}, nil)

				Expect(get(format.host, callerCert(otherAccountSAN), format.forged())).To(Equal(http.StatusForbidden))
			})

			It("tells the backend which account called", func() {
				testState.registerWithRoutePolicies(backend, format.host, "any", []string{accountSource}, nil)

				Expect(get(format.host, callerCert(accountSAN), "")).To(Equal(http.StatusOK))

				var backendReq *http.Request
				Eventually(received).Should(Receive(&backendReq))
				xfcc := backendReq.Header.Get(xfccHeader)
				if format.name == "envoy format" {
					Expect(xfcc).To(MatchRegexp(`^Hash=[0-9a-f]{64};SVC=` + strings.ReplaceAll(accountSAN, ".", `\.`) + `;Subject="`))
				} else {
					der, err := base64.StdEncoding.DecodeString(xfcc)
					Expect(err).NotTo(HaveOccurred())
					cert, err := x509.ParseCertificate(der)
					Expect(err).NotTo(HaveOccurred())
					Expect(cert.DNSNames).To(ContainElement(accountSAN))
				}
			})

			It("omits the account from the header when the caller has none", func() {
				testState.registerWithRoutePolicies(backend, format.host, "any", []string{"cf:any"}, nil)

				Expect(get(format.host, callerCert("orders.apps.internal"), "")).To(Equal(http.StatusOK))

				var backendReq *http.Request
				Eventually(received).Should(Receive(&backendReq))
				Expect(backendReq.Header.Get(xfccHeader)).NotTo(ContainSubstring("SVC="))
			})
		})
	}

	Describe("a domain that forwards the client's XFCC header", func() {
		const host = "api.forward.apps.mtls.internal"

		forwardedFor := func(space, account string) string {
			header := "Hash=abc;"
			if account != "" {
				header += "SVC=" + account + ";"
			}
			return header + `Subject="CN=forwarded,OU=app:forwarded-app,OU=space:` + space + `,OU=organization:` + callerOrg + `"`
		}
		forwarded := func(account string) string { return forwardedFor(callerSpace, account) }

		It("does not combine the forwarded identity with the account of the TLS certificate", func() {
			testState.registerWithRoutePolicies(backend, host, "any", []string{accountSource}, nil)

			Expect(get(host, callerCert(accountSAN), forwarded(""))).To(Equal(http.StatusForbidden))
		})

		It("evaluates the forwarded account like the forwarded app, space and org", func() {
			// forward is an operator opt-in to trust the forwarded header for the
			// whole identity; the account is no exception.
			testState.registerWithRoutePolicies(backend, host, "any", []string{accountSource}, nil)

			Expect(get(host, callerCert(), forwarded(accountSAN))).To(Equal(http.StatusOK))
		})

		It("does not let the account of the TLS certificate pass a scope the forwarded space would satisfy", func() {
			testState.registerWithRoutePolicies(backend, host, "space", []string{accountSource},
				map[string]string{"space_id": "destination-space-guid", "organization_id": callerOrg})

			// The header claims the destination space, which satisfies scope=space,
			// but names no account; only the TLS certificate has one.
			Expect(get(host, callerCert(accountSAN), forwardedFor("destination-space-guid", ""))).To(Equal(http.StatusForbidden))
		})
	})
})
