package integration

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("App-to-App mTLS Routing", func() {
	var testState *testState

	BeforeEach(func() {
		testState = NewTestState()
	})

	AfterEach(func() {
		if testState != nil {
			testState.StopAndCleanup()
		}
	})

	Describe("mTLS domain configuration", func() {
		var (
			mtlsDomainCA        *test_util.CertChain
			appInstanceCert     *test_util.CertChain
			backendApp          *httptest.Server
			backendReceivedReqs chan *http.Request
		)

		BeforeEach(func() {
			// Create CA for mTLS domain (simulates Diego instance identity CA)
			mtlsDomainCA = &test_util.CertChain{}
			*mtlsDomainCA = test_util.CreateSignedCertWithRootCA(test_util.CertNames{CommonName: "Diego Instance Identity CA"})

			// Setup backend app
			backendReceivedReqs = make(chan *http.Request, 10)
			backendApp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				backendReceivedReqs <- r
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("backend-response"))
			}))

			// Configure GoRouter with mTLS domain
			testState.cfg.EnableSSL = true
			testState.cfg.ClientCertificateValidationString = "request"
		})

		AfterEach(func() {
			if backendApp != nil {
				backendApp.Close()
			}
		})

		Context("when a request is made to an mTLS domain", func() {
			var mtlsDomain string

			BeforeEach(func() {
				mtlsDomain = "my-app.apps.mtls.internal"

				// Configure mTLS domain in GoRouter
				testState.cfg.MtlsDomains = []config.MtlsDomainConfig{
					{
						Domain:              "*.apps.mtls.internal",
						CACerts:             string(mtlsDomainCA.CACertPEM),
						ForwardedClientCert: config.SANITIZE_SET,
					},
				}

				testState.StartGorouterOrFail()
			})

			It("requires a client certificate", func() {
				// Register route on mTLS domain
				testState.register(backendApp, mtlsDomain)

				// Attempt request without client certificate
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				_, err := testState.client.Do(req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tls"))
			})

			It("accepts valid client certificate from the configured CA", func() {
				// Create instance identity certificate (need to use the same CA!)
				appInstanceCert = &test_util.CertChain{}
				// Recreate with SAME CA as configured in GoRouter
				*appInstanceCert = test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "app-instance",
					AppGUID:    "app-guid-123",
					SpaceGUID:  "space-guid-456",
					OrgGUID:    "org-guid-789",
				})

				// Register route on mTLS domain with allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps": []string{"app-guid-123"},
					},
				)

				// Configure client to use instance identity cert
				clientTLSConfig := &tls.Config{
					RootCAs: testState.client.Transport.(*http.Transport).TLSClientConfig.RootCAs,
					Certificates: []tls.Certificate{
						appInstanceCert.TLSCert(),
					},
				}
				testState.client.Transport.(*http.Transport).TLSClientConfig = clientTLSConfig

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				Expect(string(body)).To(Equal("backend-response"))

				// Verify backend received the request
				Eventually(backendReceivedReqs).Should(Receive())
			})

			It("rejects client certificate from unknown CA", func() {
				// Create certificate from different CA (not the configured mtlsDomainCA)
				unknownCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "app-instance",
					AppGUID:    "app-guid-123",
				})

				// Register route
				testState.register(backendApp, mtlsDomain)

				// Configure client with unknown cert
				clientTLSConfig := &tls.Config{
					RootCAs: testState.client.Transport.(*http.Transport).TLSClientConfig.RootCAs,
					Certificates: []tls.Certificate{
						unknownCert.TLSCert(),
					},
				}
				testState.client.Transport.(*http.Transport).TLSClientConfig = clientTLSConfig

				// Make request - should fail TLS handshake
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				_, err := testState.client.Do(req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tls"))
			})
		})

		Context("when requests are made to non-mTLS domains", func() {
			var regularDomain string

			BeforeEach(func() {
				regularDomain = "my-app.apps.internal"

				// Configure only the mTLS domain
				testState.cfg.MtlsDomains = []config.MtlsDomainConfig{
					{
						Domain:              "*.apps.mtls.internal",
						CACerts:             string(mtlsDomainCA.CACertPEM),
						ForwardedClientCert: config.SANITIZE_SET,
					},
				}

				testState.StartGorouterOrFail()
			})

			It("does not require client certificates", func() {
				// Register route on regular domain
				testState.register(backendApp, regularDomain)

				// Make request without client certificate (using HTTPS)
				req := testState.newGetRequest(fmt.Sprintf("https://%s", regularDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, _ := io.ReadAll(resp.Body)
				Expect(string(body)).To(Equal("backend-response"))
			})
		})
	})

	Describe("App-to-App authorization", func() {
		var (
			mtlsDomainCA        *test_util.CertChain
			backendApp          *httptest.Server
			backendReceivedReqs chan *http.Request
			mtlsDomain          string
		)

		BeforeEach(func() {
			mtlsDomain = "secure-api.apps.mtls.internal"

			// Create CA for mTLS domain
			mtlsDomainCA = &test_util.CertChain{}
			*mtlsDomainCA = test_util.CreateSignedCertWithRootCA(test_util.CertNames{CommonName: "Diego Instance Identity CA"})

			// Setup backend app
			backendReceivedReqs = make(chan *http.Request, 10)
			backendApp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				backendReceivedReqs <- r
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("authorized"))
			}))

			// Configure GoRouter
			testState.cfg.EnableSSL = true
			testState.cfg.ClientCertificateValidationString = "request"
			testState.cfg.MtlsDomains = []config.MtlsDomainConfig{
				{
					Domain:              "*.apps.mtls.internal",
					CACerts:             string(mtlsDomainCA.CACertPEM),
					ForwardedClientCert: config.SANITIZE_SET,
				},
			}

			testState.StartGorouterOrFail()
		})

		AfterEach(func() {
			if backendApp != nil {
				backendApp.Close()
			}
		})

		Describe("app-level authorization", func() {
			It("allows requests from apps in the allowed list", func() {
				callerAppGUID := "caller-app-guid-123"

				// Register route with app-level allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps": []string{callerAppGUID, "other-app-guid"},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    callerAppGUID,
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "caller-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				Expect(string(body)).To(Equal("authorized"))
			})

			It("denies requests from apps not in the allowed list", func() {
				// Register route with app-level allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps": []string{"allowed-app-guid"},
					},
				)

				// Create caller certificate with different app GUID
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "unauthorized-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "caller-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("space-level authorization", func() {
			It("allows requests from apps in allowed spaces", func() {
				callerSpaceGUID := "dev-space-guid"

				// Register route with space-level allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"spaces": []string{callerSpaceGUID, "other-space-guid"},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  callerSpaceGUID,
					OrgGUID:    "caller-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				Expect(string(body)).To(Equal("authorized"))
			})

			It("denies requests from apps in non-allowed spaces", func() {
				// Register route with space-level allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"spaces": []string{"allowed-space-guid"},
					},
				)

				// Create caller certificate with different space GUID
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "unauthorized-space-guid",
					OrgGUID:    "caller-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("org-level authorization", func() {
			It("allows requests from apps in allowed orgs", func() {
				callerOrgGUID := "my-org-guid"

				// Register route with org-level allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"orgs": []string{callerOrgGUID},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    callerOrgGUID,
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("denies requests from apps in non-allowed orgs", func() {
				// Register route with org-level allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"orgs": []string{"allowed-org-guid"},
					},
				)

				// Create caller certificate with different org GUID
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "unauthorized-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("multi-level authorization", func() {
			It("allows requests if ANY authorization level matches", func() {
				// Register route with multiple authorization levels
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps":   []string{"specific-app-guid"},
						"spaces": []string{"dev-space-guid"},
						"orgs":   []string{"my-org-guid"},
					},
				)

				// Create caller that matches space level but not app level
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "different-app-guid",
					SpaceGUID:  "dev-space-guid", // Matches allowed space
					OrgGUID:    "different-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should succeed because space matches
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("denies requests if NO authorization level matches", func() {
				// Register route with multiple authorization levels
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps":   []string{"allowed-app-guid"},
						"spaces": []string{"allowed-space-guid"},
						"orgs":   []string{"allowed-org-guid"},
					},
				)

				// Create caller that matches none
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "different-app-guid",
					SpaceGUID:  "different-space-guid",
					OrgGUID:    "different-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should fail
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("'any authenticated app' authorization", func() {
			It("allows any authenticated app when any=true", func() {
				// Register route with any=true
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"any": true,
					},
				)

				// Create arbitrary caller certificate
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "any-app-instance",
					AppGUID:    "random-app-guid-999",
					SpaceGUID:  "random-space-guid",
					OrgGUID:    "random-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should succeed
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})
		})

		Describe("default-deny behavior", func() {
			It("denies requests when no mtls_allowed_sources are configured", func() {
				// Register route WITHOUT allowed sources
				testState.register(backendApp, mtlsDomain)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should fail (default deny)
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})

			It("denies requests when mtls_allowed_sources are empty", func() {
				// Register route with empty allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps":   []string{},
						"spaces": []string{},
						"orgs":   []string{},
						"any":    false,
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should fail
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("X-Forwarded-Client-Cert header", func() {
			It("forwards sanitized client certificate to backend on mTLS domains", func() {
				// Register route with allowed sources
				testState.registerWithAllowedSources(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps": []string{"caller-app-guid"},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "caller-org-guid",
				})

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req := testState.newGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := testState.client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				resp.Body.Close()

				// Check backend received XFCC header
				var backendReq *http.Request
				Eventually(backendReceivedReqs).Should(Receive(&backendReq))
				Expect(backendReq.Header.Get("X-Forwarded-Client-Cert")).NotTo(BeEmpty())
			})
		})
	})
})
