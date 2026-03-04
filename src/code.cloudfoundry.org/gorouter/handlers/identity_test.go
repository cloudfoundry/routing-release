package handlers_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("Identity", func() {
	var (
		handler     negroni.Handler
		nextCalled  bool
		nextHandler http.HandlerFunc
		recorder    *httptest.ResponseRecorder
		request     *http.Request
		requestInfo *handlers.RequestInfo
	)

	BeforeEach(func() {
		handler = handlers.NewIdentity()
		nextCalled = false
		recorder = httptest.NewRecorder()

		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			request = r
		})

		request = test_util.NewRequest("GET", "backend.apps.mtls.internal", "/", nil)
	})

	Context("when RequestInfo is not in context", func() {
		It("calls next handler without setting identity", func() {
			handler.ServeHTTP(recorder, request, nextHandler)

			Expect(nextCalled).To(BeTrue())
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	Context("when RequestInfo is in context", func() {
		var runHandler = func() {
			// Add RequestInfo to context
			reqInfoHandler := handlers.NewRequestInfo()
			n := negroni.New()
			n.Use(reqInfoHandler)
			n.Use(handler)
			n.UseHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				request = r
				// Capture RequestInfo for assertions
				var err error
				requestInfo, err = handlers.ContextRequestInfo(r)
				Expect(err).NotTo(HaveOccurred())
			})

			n.ServeHTTP(recorder, request)
		}

		Context("when X-Forwarded-Client-Cert header is not present", func() {
			It("calls next handler without setting identity", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).To(BeNil())
			})
		})

		Context("when X-Forwarded-Client-Cert header is present", func() {
			Context("with valid cert containing app GUID in OU", func() {
				BeforeEach(func() {
					cert := generateTestCert("app:test-app-guid-123")
					certPEM := encodeCertToPEM(cert)
					xfccHeader := buildXFCCHeader(certPEM)
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("extracts caller identity with app GUID", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).NotTo(BeNil())
					Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("test-app-guid-123"))
				})
			})

			Context("with cert containing multiple OUs including app GUID", func() {
				BeforeEach(func() {
					cert := generateTestCertWithMultipleOUs([]string{
						"organization-unit-1",
						"app:another-app-guid",
						"organization-unit-2",
					})
					certPEM := encodeCertToPEM(cert)
					xfccHeader := buildXFCCHeader(certPEM)
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("extracts the app GUID from the correct OU", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).NotTo(BeNil())
					Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("another-app-guid"))
				})
			})

			Context("with malformed XFCC header", func() {
				Context("missing Cert field", func() {
					BeforeEach(func() {
						request.Header.Set("X-Forwarded-Client-Cert", "Hash=123;Subject=\"CN=test\"")
					})

					It("does not set caller identity", func() {
						runHandler()
						Expect(nextCalled).To(BeTrue())
						Expect(requestInfo.CallerIdentity).To(BeNil())
					})
				})

				Context("missing closing quote in Cert field", func() {
					BeforeEach(func() {
						request.Header.Set("X-Forwarded-Client-Cert", "Cert=\"-----BEGIN CERTIFICATE-----\nMIIB")
					})

					It("does not set caller identity", func() {
						runHandler()
						Expect(nextCalled).To(BeTrue())
						Expect(requestInfo.CallerIdentity).To(BeNil())
					})
				})

				Context("invalid PEM data", func() {
					BeforeEach(func() {
						request.Header.Set("X-Forwarded-Client-Cert", "Cert=\"not-a-valid-pem-cert\"")
					})

					It("does not set caller identity", func() {
						runHandler()
						Expect(nextCalled).To(BeTrue())
						Expect(requestInfo.CallerIdentity).To(BeNil())
					})
				})

				Context("invalid certificate data", func() {
					BeforeEach(func() {
						invalidPEM := "-----BEGIN CERTIFICATE-----\nSW52YWxpZCBjZXJ0aWZpY2F0ZSBkYXRh\n-----END CERTIFICATE-----"
						request.Header.Set("X-Forwarded-Client-Cert", buildXFCCHeader(invalidPEM))
					})

					It("does not set caller identity", func() {
						runHandler()
						Expect(nextCalled).To(BeTrue())
						Expect(requestInfo.CallerIdentity).To(BeNil())
					})
				})
			})

			Context("with cert missing app GUID in OU", func() {
				Context("no OU fields", func() {
					BeforeEach(func() {
						cert := generateTestCertWithMultipleOUs([]string{})
						certPEM := encodeCertToPEM(cert)
						xfccHeader := buildXFCCHeader(certPEM)
						request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
					})

					It("does not set caller identity", func() {
						runHandler()
						Expect(nextCalled).To(BeTrue())
						Expect(requestInfo.CallerIdentity).To(BeNil())
					})
				})

				Context("OU fields without app: prefix", func() {
					BeforeEach(func() {
						cert := generateTestCertWithMultipleOUs([]string{
							"organization-unit-1",
							"organization-unit-2",
						})
						certPEM := encodeCertToPEM(cert)
						xfccHeader := buildXFCCHeader(certPEM)
						request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
					})

					It("does not set caller identity", func() {
						runHandler()
						Expect(nextCalled).To(BeTrue())
						Expect(requestInfo.CallerIdentity).To(BeNil())
					})
				})

				Context("OU with app: prefix but empty GUID", func() {
					BeforeEach(func() {
						cert := generateTestCert("app:")
						certPEM := encodeCertToPEM(cert)
						xfccHeader := buildXFCCHeader(certPEM)
						request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
					})

					It("does not set caller identity", func() {
						runHandler()
						Expect(nextCalled).To(BeTrue())
						Expect(requestInfo.CallerIdentity).To(BeNil())
					})
				})
			})
		})
	})
})

func generateTestCertWithOrgAndSpace() *x509.Certificate {
	return generateTestCertWithMultipleOUs([]string{
		"app:test-app-guid",
		"space:test-space-guid",
		"organization:test-org-guid",
	})
}

func buildTestCertWithIdentity(appGUID, spaceGUID, orgGUID string) *x509.Certificate {
	ous := []string{}
	if appGUID != "" {
		ous = append(ous, "app:"+appGUID)
	}
	if spaceGUID != "" {
		ous = append(ous, "space:"+spaceGUID)
	}
	if orgGUID != "" {
		ous = append(ous, "organization:"+orgGUID)
	}
	return generateTestCertWithMultipleOUs(ous)
}

var _ = Describe("Identity with Space and Org extraction", func() {
	var (
		handler     negroni.Handler
		nextCalled  bool
		recorder    *httptest.ResponseRecorder
		request     *http.Request
		requestInfo *handlers.RequestInfo
	)

	BeforeEach(func() {
		handler = handlers.NewIdentity()
		nextCalled = false
		recorder = httptest.NewRecorder()

		request = test_util.NewRequest("GET", "backend.apps.mtls.internal", "/", nil)
	})

	var runHandler = func() {
		// Add RequestInfo to context
		reqInfoHandler := handlers.NewRequestInfo()
		n := negroni.New()
		n.Use(reqInfoHandler)
		n.Use(handler)
		n.UseHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			request = r
			// Capture RequestInfo for assertions
			var err error
			requestInfo, err = handlers.ContextRequestInfo(r)
			Expect(err).NotTo(HaveOccurred())
		})

		n.ServeHTTP(recorder, request)
	}

	Context("when cert contains app, space, and org GUIDs", func() {
		BeforeEach(func() {
			cert := generateTestCertWithOrgAndSpace()
			certPEM := encodeCertToPEM(cert)
			xfccHeader := buildXFCCHeader(certPEM)
			request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
		})

		It("extracts all three GUIDs", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).NotTo(BeNil())
			Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("test-app-guid"))
			Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal("test-space-guid"))
			Expect(requestInfo.CallerIdentity.OrgGUID).To(Equal("test-org-guid"))
		})
	})

	Context("when cert contains only app and space GUIDs", func() {
		BeforeEach(func() {
			cert := buildTestCertWithIdentity("my-app", "my-space", "")
			certPEM := encodeCertToPEM(cert)
			xfccHeader := buildXFCCHeader(certPEM)
			request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
		})

		It("extracts app and space GUIDs", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).NotTo(BeNil())
			Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("my-app"))
			Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal("my-space"))
			Expect(requestInfo.CallerIdentity.OrgGUID).To(Equal(""))
		})
	})

	Context("when cert contains only app GUID", func() {
		BeforeEach(func() {
			cert := buildTestCertWithIdentity("my-app", "", "")
			certPEM := encodeCertToPEM(cert)
			xfccHeader := buildXFCCHeader(certPEM)
			request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
		})

		It("extracts only app GUID", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).NotTo(BeNil())
			Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("my-app"))
			Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal(""))
			Expect(requestInfo.CallerIdentity.OrgGUID).To(Equal(""))
		})
	})

	Context("when cert contains space and org but no app GUID", func() {
		BeforeEach(func() {
			cert := buildTestCertWithIdentity("", "my-space", "my-org")
			certPEM := encodeCertToPEM(cert)
			xfccHeader := buildXFCCHeader(certPEM)
			request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
		})

		It("does not set caller identity (app GUID required)", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).To(BeNil())
		})
	})
})

// Helper functions for generating test certificates

func generateTestCert(ou string) *x509.Certificate {
	return generateTestCertWithMultipleOUs([]string{ou})
}

func generateTestCertWithMultipleOUs(ous []string) *x509.Certificate {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:         "test-instance",
			OrganizationalUnit: ous,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	cert, err := x509.ParseCertificate(certDER)
	Expect(err).NotTo(HaveOccurred())

	return cert
}

func encodeCertToPEM(cert *x509.Certificate) string {
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	return string(certPEM)
}

func buildXFCCHeader(certPEM string) string {
	// XFCC header format: Cert="<PEM-with-newlines>"
	return "Cert=\"" + certPEM + "\""
}
