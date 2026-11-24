package handlers_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/errorwriter"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/handlers/postselection"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

const (
	accountSAN    = "payments-worker.acme-payments.svc.identity"
	accountSource = "cf:svc:acme-payments:payments-worker"

	otherAccountSAN = "other-worker.acme-payments.svc.identity"
)

var (
	sharedKeyOnce sync.Once
	sharedKey     *ecdsa.PrivateKey
)

// generateTestCertWithSANs creates a self-signed instance-style certificate with
// the given OUs and DNS SANs. All certificates share one key to keep tables fast.
func generateTestCertWithSANs(ous, dnsNames []string) *x509.Certificate {
	sharedKeyOnce.Do(func() {
		var err error
		sharedKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).NotTo(HaveOccurred())
	})

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         "test-instance",
			OrganizationalUnit: ous,
		},
		DNSNames:              dnsNames,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &sharedKey.PublicKey, sharedKey)
	Expect(err).NotTo(HaveOccurred())
	cert, err := x509.ParseCertificate(der)
	Expect(err).NotTo(HaveOccurred())
	return cert
}

var _ = Describe("Service account caller identity", func() {
	const (
		rawHost         = "backend.raw.apps.identity"
		envoyHost       = "backend.envoy.apps.identity"
		forwardHost     = "backend.forward.apps.identity"
		alwaysFwdHost   = "backend.always.apps.identity"
		callerApp       = "caller-app"
		callerSpace     = "caller-space"
		callerOrg       = "caller-org"
		xfccHeaderName  = "X-Forwarded-Client-Cert"
		envoyNoSVCRegex = `^Hash=[0-9a-f]{64};Subject="`
	)

	var (
		cfg    *config.Config
		logger *test_util.TestLogger
	)

	BeforeEach(func() {
		logger = test_util.NewTestLogger("service-account-identity")
		cfg, _ = config.DefaultConfig()
		ca := test_util.CreateSignedCertWithRootCA(test_util.CertNames{SANs: test_util.SubjectAltNames{DNS: "test.com"}})
		pem := string(ca.CACertPEM)
		cfg.Domains = []config.MtlsDomainConfig{
			{Domain: "*.raw.apps.identity", CACerts: pem, XFCCFormat: config.XFCC_FORMAT_RAW, ForwardedClientCert: config.SANITIZE_SET},
			{Domain: "*.envoy.apps.identity", CACerts: pem, XFCCFormat: config.XFCC_FORMAT_ENVOY, ForwardedClientCert: config.SANITIZE_SET},
			{Domain: "*.forward.apps.identity", CACerts: pem, XFCCFormat: config.XFCC_FORMAT_ENVOY, ForwardedClientCert: config.FORWARD},
			{Domain: "*.always.apps.identity", CACerts: pem, XFCCFormat: config.XFCC_FORMAT_RAW, ForwardedClientCert: config.ALWAYS_FORWARD},
		}
		Expect(cfg.Process()).To(Succeed())
	})

	callerLeaf := func(dnsNames ...string) *x509.Certificate {
		return generateTestCertWithSANs([]string{
			"app:" + callerApp, "space:" + callerSpace, "organization:" + callerOrg,
		}, dnsNames)
	}

	// request builds a request whose TLS connection presents (and has verified)
	// leaf. forwarded, when set, is the XFCC header the client sent.
	request := func(host string, leaf *x509.Certificate, forwarded string) *http.Request {
		req := test_util.NewRequest("GET", host, "/", nil)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{leaf},
			VerifiedChains:   [][]*x509.Certificate{{leaf}},
		}
		if forwarded != "" {
			req.Header.Set(xfccHeaderName, forwarded)
		}
		return req
	}

	type outcome struct {
		xfcc     string
		identity *handlers.CallerIdentity
		info     *handlers.RequestInfo
	}

	// serve runs ClientCert and CfIdentity as the proxy does and reports the XFCC
	// header the backend would receive and the identity the router derived.
	serve := func(req *http.Request, after ...func(*handlers.RequestInfo)) outcome {
		var out outcome
		n := negroni.New(
			handlers.NewRequestInfo(),
			handlers.NewClientCert(
				func(*http.Request) bool { return false },
				func(*http.Request) (bool, error) { return false, nil },
				config.SANITIZE_SET, cfg, logger.Logger, errorwriter.NewPlaintextErrorWriter(),
			),
			handlers.NewCfIdentity(cfg),
		)
		n.UseHandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			info, err := handlers.ContextRequestInfo(r)
			Expect(err).NotTo(HaveOccurred())
			out.xfcc = r.Header.Get(xfccHeaderName)
			out.identity = info.CallerIdentity
			out.info = info
			for _, f := range after {
				f(info)
			}
		})
		n.ServeHTTP(httptest.NewRecorder(), req)
		return out
	}

	accountOf := func(o outcome) string {
		Expect(o.identity).NotTo(BeNil(), "caller identity should be present")
		return o.identity.ServiceAccountSource
	}

	// accountSANSets are certificate SAN sets and the account GoRouter must derive.
	type sanCase struct {
		sans    []string
		account string
	}
	sanTable := []TableEntry{
		Entry("one account SAN", sanCase{[]string{accountSAN}, accountSource}),
		Entry("account SAN beside unrelated SANs",
			sanCase{[]string{"7c1e-instance-guid", "orders.apps.internal", accountSAN}, accountSource}),
		Entry("no SANs", sanCase{nil, ""}),
		Entry("only unrelated SANs", sanCase{[]string{"orders.apps.internal"}, ""}),
		Entry("two different accounts are ambiguous", sanCase{[]string{accountSAN, otherAccountSAN}, ""}),
		Entry("the same account twice is ambiguous", sanCase{[]string{accountSAN, accountSAN}, ""}),
		Entry("an account beside a malformed account SAN", sanCase{[]string{accountSAN, "bad.svc.identity"}, ""}),
		Entry("only a malformed SAN (single label)", sanCase{[]string{"bad.svc.identity"}, ""}),
		Entry("prefix shorter than 3 characters", sanCase{[]string{"payments-worker.ac.svc.identity"}, ""}),
		Entry("name shorter than 3 characters", sanCase{[]string{"pw.acme-payments.svc.identity"}, ""}),
		Entry("prefix longer than 63 characters", sanCase{[]string{"payments-worker." + strings.Repeat("a", 64) + ".svc.identity"}, ""}),
		Entry("name longer than 63 characters", sanCase{[]string{strings.Repeat("a", 64) + ".acme-payments.svc.identity"}, ""}),
		Entry("too many labels", sanCase{[]string{"payments-worker.extra.acme-payments.svc.identity"}, ""}),
		Entry("uppercase labels", sanCase{[]string{"Payments-Worker.acme-payments.svc.identity"}, ""}),
		Entry("trailing dot", sanCase{[]string{accountSAN + "."}, ""}),
		Entry("longer suffix is not the account domain", sanCase{[]string{accountSAN + ".example.org"}, ""}),
	}

	Describe("with certificate data GoRouter itself derived (sanitize_set)", func() {
		DescribeTable("derives the account from the peer certificate",
			func(c sanCase) {
				for _, host := range []string{rawHost, envoyHost} {
					out := serve(request(host, callerLeaf(c.sans...), ""))
					Expect(accountOf(out)).To(Equal(c.account), "host %s", host)
					// The rest of the identity is unaffected by the account.
					Expect(out.identity.AppGUID).To(Equal(callerApp))
					Expect(out.identity.SpaceGUID).To(Equal(callerSpace))
					Expect(out.identity.OrgGUID).To(Equal(callerOrg))
				}
			},
			sanTable,
		)

		It("replaces a client-supplied header, so a forged account is never trusted", func() {
			forged := `Hash=abc;SVC=` + accountSAN + `;Subject="CN=x,OU=app:` + callerApp + `"`
			out := serve(request(envoyHost, callerLeaf(), forged))

			Expect(accountOf(out)).To(BeEmpty())
			Expect(out.xfcc).NotTo(ContainSubstring("SVC="))
		})

		It("replaces a client-supplied raw certificate carrying an account", func() {
			forgedCert := generateTestCertWithSANs([]string{"app:" + callerApp}, []string{accountSAN})
			out := serve(request(rawHost, callerLeaf(), base64.StdEncoding.EncodeToString(forgedCert.Raw)))

			Expect(accountOf(out)).To(BeEmpty())
		})

		It("does not let a Subject value smuggle an account into the envoy header", func() {
			leaf := generateTestCertWithSANs([]string{
				"app:" + callerApp,
				`x";SVC=` + otherAccountSAN + `;Foo="`,
			}, nil)
			out := serve(request(envoyHost, leaf, ""))

			Expect(accountOf(out)).To(BeEmpty())
		})

		It("keeps the genuine account when a Subject value tries to add another", func() {
			leaf := generateTestCertWithSANs([]string{
				"app:" + callerApp,
				`x";SVC=` + otherAccountSAN + `;Foo="`,
			}, []string{accountSAN})
			out := serve(request(envoyHost, leaf, ""))

			Expect(accountOf(out)).To(Equal(accountSource))
		})
	})

	Describe("the envoy XFCC header sent to backends", func() {
		It("carries the validated account as SVC=, ahead of the Subject", func() {
			out := serve(request(envoyHost, callerLeaf(accountSAN), ""))

			Expect(out.xfcc).To(MatchRegexp(
				`^Hash=[0-9a-f]{64};SVC=` + strings.ReplaceAll(accountSAN, ".", `\.`) + `;Subject="`))
			Expect(strings.Count(out.xfcc, "SVC=")).To(Equal(1))
		})

		It("does not expose other DNS SANs of the instance certificate", func() {
			out := serve(request(envoyHost, callerLeaf("orders.apps.internal", "7c1e-instance-guid", accountSAN), ""))

			Expect(out.xfcc).NotTo(ContainSubstring("DNS="))
			Expect(out.xfcc).NotTo(ContainSubstring("orders.apps.internal"))
			Expect(len(out.xfcc)).To(BeNumerically("<", 500))
		})

		It("keeps the existing Hash and Subject format when there is no account", func() {
			out := serve(request(envoyHost, callerLeaf("orders.apps.internal"), ""))

			Expect(out.xfcc).To(MatchRegexp(envoyNoSVCRegex))
			Expect(out.xfcc).NotTo(ContainSubstring("SVC="))
		})

		DescribeTable("omits SVC= unless there is exactly one well-formed account",
			func(c sanCase) {
				out := serve(request(envoyHost, callerLeaf(c.sans...), ""))

				if c.account == "" {
					Expect(out.xfcc).NotTo(ContainSubstring("SVC="))
				} else {
					Expect(out.xfcc).To(ContainSubstring("SVC=" + accountSAN + ";"))
				}
			},
			sanTable,
		)

		It("lets the raw format carry the account in the certificate itself", func() {
			out := serve(request(rawHost, callerLeaf(accountSAN), ""))

			Expect(out.xfcc).NotTo(ContainSubstring("SVC="))
			der, err := base64.StdEncoding.DecodeString(out.xfcc)
			Expect(err).NotTo(HaveOccurred())
			cert, err := x509.ParseCertificate(der)
			Expect(err).NotTo(HaveOccurred())
			Expect(cert.DNSNames).To(ContainElement(accountSAN))
		})
	})

	Describe("parsing the envoy header", func() {
		identityFor := func(header string) *handlers.CallerIdentity {
			// Forward mode passes the client header through untouched, which
			// isolates the parser from the header's emission.
			return serve(request(forwardHost, callerLeaf(), header)).identity
		}

		subject := `Subject="CN=instance,OU=app:` + callerApp + `,OU=space:` + callerSpace + `,OU=organization:` + callerOrg + `"`

		It("reads SVC= ahead of the Subject", func() {
			identity := identityFor("Hash=abc;SVC=" + accountSAN + ";" + subject)

			Expect(identity).NotTo(BeNil())
			Expect(identity.ServiceAccountSource).To(Equal(accountSource))
			Expect(identity.AppGUID).To(Equal(callerApp))
		})

		It("does not depend on the order of keys ahead of the Subject", func() {
			identity := identityFor("SVC=" + accountSAN + ";Hash=abc;" + subject)

			Expect(identity.ServiceAccountSource).To(Equal(accountSource))
		})

		It("has no account when SVC= is absent", func() {
			identity := identityFor("Hash=abc;" + subject)

			Expect(identity).NotTo(BeNil())
			Expect(identity.ServiceAccountSource).To(BeEmpty())
		})

		DescribeTable("ignores a malformed SVC= value but keeps the rest of the identity",
			func(value string) {
				identity := identityFor("Hash=abc;SVC=" + value + ";" + subject)

				Expect(identity).NotTo(BeNil())
				Expect(identity.ServiceAccountSource).To(BeEmpty())
				Expect(identity.AppGUID).To(Equal(callerApp))
			},
			Entry("empty", ""),
			Entry("single label", "bad.svc.identity"),
			Entry("source form instead of SAN form", accountSource),
			Entry("uppercase", "Payments-Worker.acme-payments.svc.identity"),
			Entry("trailing dot", accountSAN+"."),
			Entry("too many labels", "payments-worker.extra.acme-payments.svc.identity"),
			Entry("unrelated name", "orders.apps.internal"),
			Entry("quoted", `"`+accountSAN+`"`),
		)

		It("treats repeated SVC= keys as ambiguous", func() {
			identity := identityFor("Hash=abc;SVC=" + accountSAN + ";SVC=" + otherAccountSAN + ";" + subject)

			Expect(identity.ServiceAccountSource).To(BeEmpty())
		})

		It("ignores SVC= placed after the Subject", func() {
			identity := identityFor("Hash=abc;" + subject + ";SVC=" + accountSAN)

			Expect(identity).NotTo(BeNil())
			Expect(identity.ServiceAccountSource).To(BeEmpty())
		})

		It("ignores SVC= text inside the quoted Subject", func() {
			identity := identityFor(`Hash=abc;Subject="CN=instance,OU=app:` + callerApp + `;SVC=` + accountSAN + `"`)

			Expect(identity).NotTo(BeNil())
			Expect(identity.ServiceAccountSource).To(BeEmpty())
		})
	})

	// The account must come from the same place as app, space and org: the XFCC
	// data the router hands between its handlers. In sanitize_set that data is
	// derived from the verified peer certificate. In forward modes the operator
	// has chosen to trust the forwarded header, as they do for app, space and org.
	Describe("using one source for the whole identity", func() {
		forwardedEnvoy := func(account string) string {
			header := "Hash=abc;"
			if account != "" {
				header += "SVC=" + account + ";"
			}
			return header + `Subject="CN=forwarded,OU=app:forwarded-app,OU=space:forwarded-space,OU=organization:forwarded-org"`
		}

		forwardedRaw := func(dnsNames ...string) string {
			cert := generateTestCertWithSANs([]string{
				"app:forwarded-app", "space:forwarded-space", "organization:forwarded-org",
			}, dnsNames)
			return base64.StdEncoding.EncodeToString(cert.Raw)
		}

		type forwardCase struct {
			host      string
			forwarded func(account string) string
		}
		forwardCases := []TableEntry{
			Entry("forward, envoy format", forwardCase{forwardHost, forwardedEnvoy}),
			Entry("always_forward, raw format", forwardCase{alwaysFwdHost, func(account string) string {
				if account == "" {
					return forwardedRaw()
				}
				return forwardedRaw(account)
			}}),
		}

		DescribeTable("ignores an account on the TLS leaf that the forwarded header does not carry",
			func(c forwardCase) {
				out := serve(request(c.host, callerLeaf(accountSAN), c.forwarded("")))

				Expect(out.identity).NotTo(BeNil())
				Expect(out.identity.AppGUID).To(Equal("forwarded-app"))
				Expect(out.identity.ServiceAccountSource).To(BeEmpty())
			},
			forwardCases,
		)

		DescribeTable("takes the account from the forwarded header together with app, space and org",
			func(c forwardCase) {
				out := serve(request(c.host, callerLeaf(), c.forwarded(accountSAN)))

				Expect(out.identity).NotTo(BeNil())
				Expect(out.identity.AppGUID).To(Equal("forwarded-app"))
				Expect(out.identity.SpaceGUID).To(Equal("forwarded-space"))
				Expect(out.identity.OrgGUID).To(Equal("forwarded-org"))
				Expect(out.identity.ServiceAccountSource).To(Equal(accountSource))
			},
			forwardCases,
		)

		DescribeTable("prefers the forwarded account over a different account on the TLS leaf",
			func(c forwardCase) {
				out := serve(request(c.host, callerLeaf(otherAccountSAN), c.forwarded(accountSAN)))

				Expect(accountOf(out)).To(Equal(accountSource))
			},
			forwardCases,
		)
	})

	Describe("authorizing route policies end to end", func() {
		type authCase struct {
			host     string
			leafSANs []string
			// forwarded is the client XFCC header, only meaningful in forward modes.
			forwarded string
			scope     string
			// endpointSpace and endpointOrg are the selected backend's tags.
			endpointSpace string
			endpointOrg   string
			policies      []string
			allowed       bool
			rule          string
		}

		authorize := func(c authCase) {
			if c.endpointSpace == "" {
				c.endpointSpace = callerSpace
			}
			if c.endpointOrg == "" {
				c.endpointOrg = callerOrg
			}
			if c.scope == "" {
				c.scope = route.RoutePolicyScopeAny
			}
			endpoint := route.NewEndpoint(&route.EndpointOpts{
				Host:             "127.0.0.1",
				Port:             8080,
				Tags:             map[string]string{"space_id": c.endpointSpace, "organization_id": c.endpointOrg},
				RoutePolicyScope: c.scope,
				RoutePolicies:    c.policies,
			})
			pool := route.NewPool(&route.PoolOpts{Host: "backend.apps.identity"})
			pool.Put(endpoint)

			var scopeErr, policyErr error
			out := serve(request(c.host, callerLeaf(c.leafSANs...), c.forwarded), func(info *handlers.RequestInfo) {
				info.RoutePool = pool
				scopeErr = postselection.NewMtlsScopeAuth(cfg, logger.Logger).Check(endpoint, info)
				policyErr = postselection.NewMtlsRoutePoliciesAuth(cfg, logger.Logger).Check(endpoint, info)
			})

			if c.allowed {
				Expect(scopeErr).NotTo(HaveOccurred())
				Expect(policyErr).NotTo(HaveOccurred())
				Expect(out.info.AuthResult.Rule).To(Equal(c.rule))
			} else {
				Expect(scopeErr != nil || policyErr != nil).To(BeTrue(),
					fmt.Sprintf("expected denial; scope=%v policy=%v", scopeErr, policyErr))
			}
		}

		DescribeTable("evaluates cf:svc: sources against the verified account",
			authorize,
			Entry("raw: matching account is allowed", authCase{
				host: rawHost, leafSANs: []string{accountSAN},
				policies: []string{accountSource}, allowed: true, rule: "route:" + accountSource,
			}),
			Entry("envoy: matching account is allowed", authCase{
				host: envoyHost, leafSANs: []string{"orders.apps.internal", accountSAN},
				policies: []string{accountSource}, allowed: true, rule: "route:" + accountSource,
			}),
			Entry("a different account is denied", authCase{
				host: envoyHost, leafSANs: []string{otherAccountSAN},
				policies: []string{accountSource},
			}),
			Entry("the same name under another prefix is denied", authCase{
				host: envoyHost, leafSANs: []string{"payments-worker.globex-billing.svc.identity"},
				policies: []string{accountSource},
			}),
			Entry("a caller without an account is denied", authCase{
				host: envoyHost, leafSANs: nil,
				policies: []string{accountSource},
			}),
			Entry("ambiguous accounts are denied", authCase{
				host: envoyHost, leafSANs: []string{accountSAN, otherAccountSAN},
				policies: []string{accountSource},
			}),
			Entry("a malformed account SAN is denied", authCase{
				host: rawHost, leafSANs: []string{accountSAN, "bad.svc.identity"},
				policies: []string{accountSource},
			}),
			Entry("the account is allowed in the destination space under scope=space", authCase{
				host: envoyHost, leafSANs: []string{accountSAN}, scope: route.RoutePolicyScopeSpace,
				policies: []string{accountSource}, allowed: true, rule: "route:" + accountSource,
			}),
			Entry("the account is denied by scope=space for a backend in another space", authCase{
				host: envoyHost, leafSANs: []string{accountSAN}, scope: route.RoutePolicyScopeSpace,
				endpointSpace: "destination-space", policies: []string{accountSource},
			}),
			Entry("the account is denied by scope=org for a backend in another org", authCase{
				host: rawHost, leafSANs: []string{accountSAN}, scope: route.RoutePolicyScopeOrg,
				endpointOrg: "destination-org", policies: []string{accountSource},
			}),
			Entry("the account is allowed by scope=org for a backend in the same org", authCase{
				host: rawHost, leafSANs: []string{accountSAN}, scope: route.RoutePolicyScopeOrg,
				endpointSpace: "destination-space", policies: []string{accountSource},
				allowed: true, rule: "route:" + accountSource,
			}),
			Entry("a forwarded header without the account denies a leaf that has one", authCase{
				host: forwardHost, leafSANs: []string{accountSAN},
				forwarded: `Hash=abc;Subject="CN=f,OU=app:forwarded-app,OU=space:` + callerSpace + `,OU=organization:` + callerOrg + `"`,
				policies:  []string{accountSource},
			}),
			Entry("a forwarded account is evaluated like the forwarded app identity", authCase{
				host: forwardHost, leafSANs: nil,
				forwarded: `Hash=abc;SVC=` + accountSAN + `;Subject="CN=f,OU=app:forwarded-app,OU=space:` + callerSpace + `,OU=organization:` + callerOrg + `"`,
				policies:  []string{accountSource}, allowed: true, rule: "route:" + accountSource,
			}),
			Entry("a forwarded identity from another space cannot pass scope=space via the leaf's account", authCase{
				host: forwardHost, leafSANs: []string{accountSAN}, scope: route.RoutePolicyScopeSpace,
				endpointSpace: "destination-space",
				forwarded:     `Hash=abc;Subject="CN=f,OU=app:forwarded-app,OU=space:destination-space,OU=organization:` + callerOrg + `"`,
				policies:      []string{accountSource},
			}),
		)
	})
})
