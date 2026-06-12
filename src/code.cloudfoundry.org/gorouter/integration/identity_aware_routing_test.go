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

var _ = Describe("Identity-Aware Routing", func() {
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
				testState.cfg.Domains = []config.MtlsDomainConfig{
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
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				_, err := client.Do(req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tls: certificate required"))
			})

			It("accepts valid client certificate from the configured CA", func() {
				// Create instance identity certificate (need to use the same CA!)
				appInstanceCert = &test_util.CertChain{}
				// Recreate with SAME CA as configured in GoRouter
				*appInstanceCert = test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "app-instance",
					AppGUID:    "app-guid-123",
					SpaceGUID:  "space-guid-456",
					OrgGUID:    "org-guid-789",
				}, mtlsDomainCA)

				// Register route on mTLS domain with allowed sources
				testState.registerWithAccessRules(
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
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
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
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				_, err := client.Do(req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tls: unknown certificate authority"))
			})
		})

		Context("when requests are made to non-mTLS domains", func() {
			var regularDomain string

			BeforeEach(func() {
				regularDomain = "my-app.apps.internal"

				// Configure only the mTLS domain
				testState.cfg.Domains = []config.MtlsDomainConfig{
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
			testState.cfg.Domains = []config.MtlsDomainConfig{
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
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps": []string{callerAppGUID, "other-app-guid"},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    callerAppGUID,
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "caller-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				Expect(string(body)).To(Equal("authorized"))
			})

			It("denies requests from apps not in the allowed list", func() {
				// Register route with app-level allowed sources
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps": []string{"allowed-app-guid"},
					},
				)

				// Create caller certificate with different app GUID
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "unauthorized-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "caller-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("space-level authorization", func() {
			It("allows requests from apps in allowed spaces", func() {
				callerSpaceGUID := "dev-space-guid"

				// Register route with space-level allowed sources
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"spaces": []string{callerSpaceGUID, "other-space-guid"},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  callerSpaceGUID,
					OrgGUID:    "caller-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				Expect(string(body)).To(Equal("authorized"))
			})

			It("denies requests from apps in non-allowed spaces", func() {
				// Register route with space-level allowed sources
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"spaces": []string{"allowed-space-guid"},
					},
				)

				// Create caller certificate with different space GUID
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "unauthorized-space-guid",
					OrgGUID:    "caller-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("org-level authorization", func() {
			It("allows requests from apps in allowed orgs", func() {
				callerOrgGUID := "my-org-guid"

				// Register route with org-level allowed sources
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"orgs": []string{callerOrgGUID},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    callerOrgGUID,
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("denies requests from apps in non-allowed orgs", func() {
				// Register route with org-level allowed sources
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"orgs": []string{"allowed-org-guid"},
					},
				)

				// Create caller certificate with different org GUID
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "unauthorized-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("multi-level authorization", func() {
			It("allows requests if ANY authorization level matches", func() {
				// Register route with multiple authorization levels
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps":   []string{"specific-app-guid"},
						"spaces": []string{"dev-space-guid"},
						"orgs":   []string{"my-org-guid"},
					},
				)

				// Create caller that matches space level but not app level
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "different-app-guid",
					SpaceGUID:  "dev-space-guid", // Matches allowed space
					OrgGUID:    "different-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should succeed because space matches
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("denies requests if NO authorization level matches", func() {
				// Register route with multiple authorization levels
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps":   []string{"allowed-app-guid"},
						"spaces": []string{"allowed-space-guid"},
						"orgs":   []string{"allowed-org-guid"},
					},
				)

				// Create caller that matches none
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "different-app-guid",
					SpaceGUID:  "different-space-guid",
					OrgGUID:    "different-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should fail
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("'any authenticated app' authorization", func() {
			It("allows any authenticated app when any=true", func() {
				// Register route with any=true
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"any": true,
					},
				)

				// Create arbitrary caller certificate
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "any-app-instance",
					AppGUID:    "random-app-guid-999",
					SpaceGUID:  "random-space-guid",
					OrgGUID:    "random-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should succeed
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})
		})

		Describe("default-deny behavior", func() {
			It("allows requests when route policy enforcement is not enabled", func() {
				// Register route without route policy scope (enforcement disabled)
				// Cloud Controller only sets RoutePolicyScope when the domain is configured
				// with --enforce-route-policies flag
				testState.register(backendApp, mtlsDomain)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should succeed (no enforcement, backend handles auth)
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("denies requests when route policies are empty", func() {
				// Register route with empty allowed sources
				testState.registerWithAccessRules(
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
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request - should fail
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})

		Describe("X-Forwarded-Client-Cert header", func() {
			It("forwards sanitized client certificate to backend on mTLS domains", func() {
				// Register route with allowed sources
				testState.registerWithAccessRules(
					backendApp,
					mtlsDomain,
					map[string]interface{}{
						"apps": []string{"caller-app-guid"},
					},
				)

				// Create caller certificate
				callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
					CommonName: "caller-app-instance",
					AppGUID:    "caller-app-guid",
					SpaceGUID:  "caller-space-guid",
					OrgGUID:    "caller-org-guid",
				}, mtlsDomainCA)

				// Configure client
				testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
					callerCert.TLSCert(),
				}

				// Make request
				req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", mtlsDomain))
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				resp.Body.Close()

				// Check backend received XFCC header
				var backendReq *http.Request
				Eventually(backendReceivedReqs).Should(Receive(&backendReq))
				Expect(backendReq.Header.Get("X-Forwarded-Client-Cert")).NotTo(BeEmpty())
			})
		})

		// RFC Scenario: Shared routes with post-selection authorization
		// This test validates the expected intermittent 403 behavior described in
		// RFC lines 475-517 (Post-Selection Authorization).
		Describe("shared routes with scope boundaries (intermittent 403s)", func() {
			var (
				sharedDomain string
				backendApp1  *httptest.Server
				backendApp2  *httptest.Server
				app1Requests chan *http.Request
				app2Requests chan *http.Request
			)

			BeforeEach(func() {
				sharedDomain = "shared.apps.mtls.internal"
				app1Requests = make(chan *http.Request, 10)
				app2Requests = make(chan *http.Request, 10)

				// Setup two backend apps in DIFFERENT spaces
				backendApp1 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					app1Requests <- r
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("backend-app-1"))
				}))

				backendApp2 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					app2Requests <- r
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("backend-app-2"))
				}))
			})

			AfterEach(func() {
				if backendApp1 != nil {
					backendApp1.Close()
				}
				if backendApp2 != nil {
					backendApp2.Close()
				}
			})

			Context("when two apps register the same route in different spaces", func() {
				It("allows requests to the same space and denies to different space (intermittent 403s)", func() {
					// Register SAME route from two different spaces with scope=space
					// Backend 1 is in space-alpha
					testState.registerWithScopeAndAccessRules(
						backendApp1,
						sharedDomain,
						"space",
						map[string]interface{}{
							"any": true,
						},
						map[string]string{
							"space_id": "space-alpha",
						},
					)

					// Backend 2 is in space-beta
					testState.registerWithScopeAndAccessRules(
						backendApp2,
						sharedDomain,
						"space",
						map[string]interface{}{
							"any": true,
						},
						map[string]string{
							"space_id": "space-beta",
						},
					)

					// Create caller from space-alpha
					callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
						CommonName: "caller-app-instance",
						AppGUID:    "caller-app-guid",
						SpaceGUID:  "space-alpha",
						OrgGUID:    "org-123",
					}, mtlsDomainCA)

					testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
						callerCert.TLSCert(),
					}

					// Make multiple requests and observe intermittent behavior
					successCount := 0
					forbiddenCount := 0
					attempts := 10

					for i := 0; i < attempts; i++ {
						req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", sharedDomain))
						resp, err := client.Do(req)
						Expect(err).NotTo(HaveOccurred())

						if resp.StatusCode == http.StatusOK {
							body, _ := io.ReadAll(resp.Body)
							// Should only succeed when routed to space-alpha backend
							Expect(string(body)).To(Equal("backend-app-1"))
							successCount++
						} else if resp.StatusCode == http.StatusForbidden {
							// Expected: post-selection check failed (routed to space-beta backend)
							forbiddenCount++
						}
						resp.Body.Close()
					}

					// Verify we got BOTH outcomes (RFC-compliant intermittent 403s)
					// With round-robin load balancing, both endpoints should be hit
					Expect(successCount).To(BeNumerically(">", 0), "Should have some successful requests (same-space)")
					Expect(forbiddenCount).To(BeNumerically(">", 0), "Should have some 403 responses (cross-space)")
					Expect(successCount + forbiddenCount).To(Equal(attempts))
				})

				It("always succeeds when caller is in same org with scope=org", func() {
					// Register SAME route from two different spaces but SAME org with scope=org
					// Backend 1 is in org-alpha/space-alpha
					testState.registerWithScopeAndAccessRules(
						backendApp1,
						sharedDomain,
						"org",
						map[string]interface{}{
							"any": true,
						},
						map[string]string{
							"organization_id": "org-alpha",
							"space_id":        "space-alpha",
						},
					)

					// Backend 2 is in org-alpha/space-beta (same org, different space)
					testState.registerWithScopeAndAccessRules(
						backendApp2,
						sharedDomain,
						"org",
						map[string]interface{}{
							"any": true,
						},
						map[string]string{
							"organization_id": "org-alpha",
							"space_id":        "space-beta",
						},
					)

					// Create caller from org-alpha
					callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
						CommonName: "caller-app-instance",
						AppGUID:    "caller-app-guid",
						SpaceGUID:  "space-gamma", // Different space, but same org
						OrgGUID:    "org-alpha",
					}, mtlsDomainCA)

					testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
						callerCert.TLSCert(),
					}

					// Make multiple requests - ALL should succeed (same org)
					for i := 0; i < 10; i++ {
						req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", sharedDomain))
						resp, err := client.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
						resp.Body.Close()
					}
				})

				It("always fails when caller is in different org with scope=org", func() {
					// Register SAME route from two different orgs with scope=org
					// Backend 1 is in org-alpha
					testState.registerWithScopeAndAccessRules(
						backendApp1,
						sharedDomain,
						"org",
						map[string]interface{}{
							"any": true,
						},
						map[string]string{
							"organization_id": "org-alpha",
						},
					)

					// Backend 2 is in org-beta
					testState.registerWithScopeAndAccessRules(
						backendApp2,
						sharedDomain,
						"org",
						map[string]interface{}{
							"any": true,
						},
						map[string]string{
							"organization_id": "org-beta",
						},
					)

					// Create caller from org-gamma (different from both backends)
					callerCert := test_util.CreateInstanceIdentityCertWithCA(test_util.InstanceIdentityCertNames{
						CommonName: "caller-app-instance",
						AppGUID:    "caller-app-guid",
						SpaceGUID:  "space-123",
						OrgGUID:    "org-gamma",
					}, mtlsDomainCA)

					testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{
						callerCert.TLSCert(),
					}

					// Make multiple requests - ALL should fail (different org)
					for i := 0; i < 10; i++ {
						req, client := testState.newMtlsGetRequest(fmt.Sprintf("https://%s", sharedDomain))
						resp, err := client.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
						resp.Body.Close()
					}
				})
			})
		})
	})
})
