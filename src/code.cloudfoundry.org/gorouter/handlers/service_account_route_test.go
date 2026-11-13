package handlers_test

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/handlers/postselection"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
	"github.com/onsi/gomega"
	"github.com/urfave/negroni/v3"
)

func TestServiceAccountRouteUsesVerifiedCertificateSAN(t *testing.T) {
	gomega.RegisterTestingT(t)
	for _, tc := range []struct {
		name     string
		sans     []string
		verified bool
		allow    bool
	}{
		{"matching qualified SAN", []string{"worker.payments-team.svc.identity"}, true, true},
		{"wrong account", []string{"other.payments-team.svc.identity"}, true, false},
		{"no account", nil, true, false},
		{"ambiguous accounts", []string{"worker.payments-team.svc.identity", "other.payments-team.svc.identity"}, true, false},
		{"malformed account", []string{"worker.payments-team.svc.identity", "invalid.svc.identity"}, true, false},
		{"unverified certificate", []string{"worker.payments-team.svc.identity"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := config.DefaultConfig()
			ca := test_util.CreateSignedCertWithRootCA(test_util.CertNames{SANs: test_util.SubjectAltNames{DNS: "test.com"}})
			cfg.Domains = []config.MtlsDomainConfig{{Domain: "*.apps.identity", XFCCFormat: "raw", CACerts: string(ca.CACertPEM)}}
			if err := cfg.Process(); err != nil {
				t.Fatal(err)
			}
			cert := generateTestCert("app:caller")
			// Use a separately parsed public certificate for the authoritative TLS
			// identity. The header certificate has no account SAN and cannot grant it.
			verifiedLeaf := &x509.Certificate{Subject: pkix.Name{OrganizationalUnit: cert.Subject.OrganizationalUnit}, DNSNames: tc.sans}
			req := httptest.NewRequest("GET", "https://backend.apps.identity/", nil)
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{verifiedLeaf}}
			if tc.verified {
				req.TLS.VerifiedChains = [][]*x509.Certificate{{verifiedLeaf}}
			}
			req.Header.Set("X-Forwarded-Client-Cert", base64.StdEncoding.EncodeToString(cert.Raw))
			endpoint := route.NewEndpoint(&route.EndpointOpts{Host: "127.0.0.1", Port: 8080, RoutePolicyScope: route.RoutePolicyScopeAny, RoutePolicies: []string{"cf:svc:payments-team:worker"}})
			pool := route.NewPool(&route.PoolOpts{Host: "backend.apps.identity"})
			pool.Put(endpoint)
			chain := negroni.New(handlers.NewRequestInfo(), handlers.NewCfIdentity(cfg))
			chain.UseHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				info, err := handlers.ContextRequestInfo(r)
				if err != nil {
					t.Fatal(err)
				}
				info.RoutePool = pool
				err = postselection.NewMtlsRoutePoliciesAuth(cfg, nil).Check(endpoint, info)
				if (err == nil) != tc.allow {
					t.Fatalf("allow=%v, error=%v", tc.allow, err)
				}
				if tc.allow && info.AuthResult.Rule != "route:cf:svc:payments-team:worker" {
					t.Fatalf("wrong matched rule: %v", info.AuthResult)
				}
				// Ensure no accidental dependence on private credentials in identity.
				if _, err := json.Marshal(info.CallerIdentity); err != nil {
					t.Fatal(err)
				}
			})
			chain.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
