package routeservice_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/common/secure"
	"code.cloudfoundry.org/gorouter/common/secure/fakes"
	"code.cloudfoundry.org/gorouter/routeservice"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("Route Service Config", func() {
	var (
		config           *routeservice.RouteServiceConfig
		crypto           secure.Crypto
		cryptoPrev       secure.Crypto
		cryptoKey        = "ABCDEFGHIJKLMNOP"
		logger           *test_util.TestLogger
		recommendHttps   bool
		strictValidation bool
	)

	BeforeEach(func() {
		var err error
		crypto, err = secure.NewAesGCM([]byte(cryptoKey))
		Expect(err).ToNot(HaveOccurred())
		logger = test_util.NewTestLogger("test")
		config = routeservice.NewRouteServiceConfig(logger.Logger, true, true, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
	})

	AfterEach(func() {
		crypto = nil
		cryptoPrev = nil
		config = nil
	})

	Describe("Request", func() {
		It("sets the signature to the forwarded URL exactly", func() {
			rawForwardedURL := "this is my url%0A"
			rsUrl := "https://example.com"

			args, err := config.CreateRequest(rsUrl, rawForwardedURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(args.ForwardedURL).To(Equal(rawForwardedURL))

			signatureContents, err := routeservice.SignatureContentsFromHeaders(args.Signature, args.Metadata, crypto)
			Expect(err).ToNot(HaveOccurred())

			Expect(signatureContents.ForwardedUrl).To(Equal(rawForwardedURL))
		})

		It("sets the requested time", func() {
			rawForwardedUrl := "test.app.com?query=sample"
			now := time.Now()
			rsUrl := "https://example.com"

			args, err := config.CreateRequest(rsUrl, rawForwardedUrl)
			Expect(err).NotTo(HaveOccurred())

			signatureContents, err := routeservice.SignatureContentsFromHeaders(args.Signature, args.Metadata, crypto)
			Expect(err).ToNot(HaveOccurred())

			Expect(signatureContents.RequestedTime).To(BeTemporally(">=", now))
		})

		It("signs the parsed route service URL", func() {
			args, err := config.CreateRequest("https://example.com/path?query=sample", "https://forwarded.example.com")
			Expect(err).NotTo(HaveOccurred())

			signatureContents, err := routeservice.SignatureContentsFromHeaders(args.Signature, args.Metadata, crypto)
			Expect(err).ToNot(HaveOccurred())
			Expect(signatureContents.RouteServiceUrl).To(Equal(args.ParsedUrl.String()))
		})

		It("returns an error for an unparseable route service URL", func() {
			_, err := config.CreateRequest("https://example.com/%zz", "https://forwarded.example.com")
			Expect(err).To(HaveOccurred())
		})

		Context("when encryption fails", func() {
			BeforeEach(func() {
				fakeCrypto := &fakes.FakeCrypto{}
				fakeCrypto.EncryptReturns([]byte{}, []byte{}, errors.New("test failed"))

				config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, 1*time.Hour, fakeCrypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns an error", func() {
				rawForwardedURL := "test.app.com"
				rsUrl := "https://example.com"

				args, err := config.CreateRequest(rsUrl, rawForwardedURL)
				Expect(err).To(HaveOccurred())

				Expect(args.Metadata).To(BeEmpty())
				Expect(args.Signature).To(BeEmpty())
			})
		})

		It("returns route service request information", func() {
			rsUrl := "https://example.com"
			forwardedUrl := "https://forwarded.example.com"
			args, err := config.CreateRequest(rsUrl, forwardedUrl)
			Expect(err).NotTo(HaveOccurred())

			rsURL, err := url.Parse(rsUrl)
			Expect(err).ToNot(HaveOccurred())

			Expect(args.ParsedUrl).To(Equal(rsURL))
			Expect(args.URLString).To(Equal(rsUrl))
			Expect(args.ForwardedURL).To(Equal(forwardedUrl))
		})
	})

	Describe("ValidateRequest", func() {
		var (
			requestFromRouteService routeservice.RequestReceivedFromRouteService
			requestUrl              string
			signatureContents       *routeservice.SignatureContents
		)

		BeforeEach(func() {
			var err error
			requestUrl = "http://some-forwarded-url.com"
			signatureContents = &routeservice.SignatureContents{
				RequestedTime:   time.Now(),
				ForwardedUrl:    requestUrl,
				RouteServiceUrl: requestUrl,
			}

			signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, signatureContents)
			Expect(err).ToNot(HaveOccurred())

			requestFromRouteService = routeservice.RequestReceivedFromRouteService{
				AppUrl:    requestUrl,
				Signature: signature,
				Metadata:  metadata,
			}
		})

		It("decrypts a valid signature and returns the decrypted signature", func() {
			validatedSig, err := config.ValidateRequest(requestFromRouteService)
			Expect(err).NotTo(HaveOccurred())
			Expect(validatedSig.ForwardedUrl).To(Equal(signatureContents.ForwardedUrl))
			Expect(validatedSig.RequestedTime.Equal(signatureContents.RequestedTime)).To(BeTrue())
		})

		It("decrypts an authenticated signature with an explicitly empty route service URL", func() {
			rawContents := []byte(`{"forwarded_url":"http://some-forwarded-url.com","requested_time":"` + time.Now().Format(time.RFC3339Nano) + `","route_service_url":""}`)
			ciphertext, nonce, err := crypto.Encrypt(rawContents)
			Expect(err).NotTo(HaveOccurred())
			metadata, err := json.Marshal(routeservice.Metadata{Nonce: nonce})
			Expect(err).NotTo(HaveOccurred())
			requestFromRouteService.Signature = base64.URLEncoding.EncodeToString(ciphertext)
			requestFromRouteService.Metadata = base64.URLEncoding.EncodeToString(metadata)

			validated, err := config.ValidateRequest(requestFromRouteService)
			Expect(err).NotTo(HaveOccurred())
			Expect(validated.RouteServiceUrl).To(BeEmpty())
		})

		Context("when the timestamp is expired", func() {
			BeforeEach(func() {
				signatureContents = &routeservice.SignatureContents{
					RequestedTime:   time.Now().Add(-10 * time.Hour),
					ForwardedUrl:    requestUrl,
					RouteServiceUrl: "https://route-service.example",
				}
				signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, signatureContents)
				Expect(err).ToNot(HaveOccurred())

				requestFromRouteService = routeservice.RequestReceivedFromRouteService{
					AppUrl:    requestUrl,
					Signature: signature,
					Metadata:  metadata,
				}
			})

			It("returns an route service request expired error", func() {
				_, err := config.ValidateRequest(requestFromRouteService)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(routeservice.ErrExpired))
				Expect(err.Error()).To(ContainSubstring("request expired"))
			})
		})

		Context("when the signature is invalid", func() {
			BeforeEach(func() {
				requestFromRouteService = routeservice.RequestReceivedFromRouteService{
					AppUrl:    requestUrl,
					Signature: "zKQt4bnxW30Kxky",
					Metadata:  "eyJpdiI6IjlBVn",
				}
			})

			It("returns an error", func() {
				_, err := config.ValidateRequest(requestFromRouteService)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the header does not match the current key", func() {
			BeforeEach(func() {
				var err error
				crypto, err = secure.NewAesGCM([]byte("QRSTUVWXYZ123456"))
				Expect(err).NotTo(HaveOccurred())
				config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			Context("when there is no previous key in the configuration", func() {
				It("rejects the signature", func() {
					_, err := config.ValidateRequest(requestFromRouteService)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("authentication failed"))
				})
			})

			Context("when the header key matches the previous key in the configuration", func() {
				BeforeEach(func() {
					var err error
					cryptoPrev, err = secure.NewAesGCM([]byte(cryptoKey))
					Expect(err).ToNot(HaveOccurred())
					config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
				})

				It("validates the signature", func() {
					_, err := config.ValidateRequest(requestFromRouteService)
					Expect(err).NotTo(HaveOccurred())
				})

				Context("when a request has an expired Route service signature header", func() {
					BeforeEach(func() {
						signatureContents = &routeservice.SignatureContents{
							RequestedTime: time.Now().Add(-10 * time.Hour),
							ForwardedUrl:  "some-forwarded-url",
						}
						signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, signatureContents)
						Expect(err).ToNot(HaveOccurred())

						requestFromRouteService = routeservice.RequestReceivedFromRouteService{
							AppUrl:    requestUrl,
							Signature: signature,
							Metadata:  metadata,
						}
					})

					It("returns an route service request expired error", func() {
						_, err := config.ValidateRequest(requestFromRouteService)
						Expect(err).To(HaveOccurred())
						Expect(err).To(BeAssignableToTypeOf(routeservice.ErrExpired))
					})
				})
			})

			Context("when the header key does not match the previous key in the configuration", func() {
				BeforeEach(func() {
					var err error
					cryptoPrev, err = secure.NewAesGCM([]byte("QRSTUVWXYZ123456"))
					Expect(err).ToNot(HaveOccurred())
					config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
				})

				It("rejects the signature", func() {
					_, err := config.ValidateRequest(requestFromRouteService)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("authentication failed"))
				})
			})
		})
	})

	Describe("RouteServiceEnabled", func() {
		Context("when rs recommendHttps is set to true", func() {
			BeforeEach(func() {
				recommendHttps = true
				config = routeservice.NewRouteServiceConfig(logger.Logger, true, true, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns the routeServiceEnabled to be true", func() {
				Expect(config.RouteServiceRecommendHttps()).To(BeTrue())
			})
		})

		Context("when rs recommendHttps is set to false", func() {
			BeforeEach(func() {
				recommendHttps = false
				config = routeservice.NewRouteServiceConfig(logger.Logger, true, true, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns the routeServiceEnabled to be false", func() {
				Expect(config.RouteServiceRecommendHttps()).To(BeFalse())
			})
		})
	})

	Describe("RouteServiceHairpinning", func() {
		Context("when routeServiceHairpinning is set to true", func() {
			BeforeEach(func() {
				recommendHttps = true
				config = routeservice.NewRouteServiceConfig(logger.Logger, true, true, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns the routeServiceEnabled to be true", func() {
				Expect(config.RouteServiceHairpinning()).To(BeTrue())
			})
		})

		Context("when routeServiceHairpinning is set to false", func() {
			BeforeEach(func() {
				recommendHttps = false
				config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns the routeServiceHairpinning to be false", func() {
				Expect(config.RouteServiceHairpinning()).To(BeFalse())
			})
		})
	})

	Describe("RouteServiceEnabled", func() {
		Context("when  RouteService is Enabled", func() {
			BeforeEach(func() {
				routeServiceEnabled := true
				config = routeservice.NewRouteServiceConfig(logger.Logger, routeServiceEnabled, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns the routeServiceEnabled to be true", func() {
				Expect(config.RouteServiceEnabled()).To(BeTrue())
			})
		})

		Context("when  RouteService is not Enabled", func() {
			BeforeEach(func() {
				routeServiceEnabled := false
				config = routeservice.NewRouteServiceConfig(logger.Logger, routeServiceEnabled, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns the routeServiceEnabled to be false", func() {
				Expect(config.RouteServiceEnabled()).To(BeFalse())
			})
		})
	})

	Describe("AllowUnboundSignatures", func() {
		It("returns the constructor value", func() {
			for _, allow := range []bool{true, false} {
				config := routeservice.NewRouteServiceConfig(logger.Logger, true, true, nil, time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, allow, true, nil)
				Expect(config.AllowUnboundSignatures()).To(Equal(allow))
			}
		})
	})

	Describe("EnableWebsockets", func() {
		Context("when EnableWebsockets is enabled", func() {
			BeforeEach(func() {
				config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, true, nil)
			})

			It("returns the EnableWebsockets to be true", func() {
				Expect(config.EnableWebsockets()).To(BeTrue())
			})
		})

		Context("when EnableWebsockets is not enabled", func() {
			BeforeEach(func() {
				config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, 1*time.Hour, crypto, cryptoPrev, recommendHttps, strictValidation, true, false, nil)
			})

			It("returns the EnableWebsockets to be false", func() {
				Expect(config.EnableWebsockets()).To(BeFalse())
			})
		})
	})
})
