package round_tripper_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"go.uber.org/zap"

	"code.cloudfoundry.org/gorouter/common/uuid"
	"code.cloudfoundry.org/gorouter/config"
	sharedfakes "code.cloudfoundry.org/gorouter/fakes"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/metrics/fakes"
	"code.cloudfoundry.org/gorouter/proxy/fails"
	errorClassifierFakes "code.cloudfoundry.org/gorouter/proxy/fails/fakes"
	"code.cloudfoundry.org/gorouter/proxy/round_tripper"
	roundtripperfakes "code.cloudfoundry.org/gorouter/proxy/round_tripper/fakes"
	"code.cloudfoundry.org/gorouter/proxy/utils"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/routeservice"
	"code.cloudfoundry.org/gorouter/test_util"
)

const StickyCookieKey = "JSESSIONID"
const AZ = "meow-zone"

type testBody struct {
	bytes.Buffer
	closeCount int
}

func (t *testBody) Close() error {
	t.closeCount++
	return nil
}

type RequestedRoundTripperType struct {
	IsRouteService bool
	IsHttp2        bool
}

type FakeRoundTripperFactory struct {
	ReturnValue                round_tripper.ProxyRoundTripper
	RequestedRoundTripperTypes []RequestedRoundTripperType
}

func (f *FakeRoundTripperFactory) New(expectedServerName string, isRouteService bool, isHttp2 bool) round_tripper.ProxyRoundTripper {
	f.RequestedRoundTripperTypes = append(f.RequestedRoundTripperTypes, RequestedRoundTripperType{
		IsRouteService: isRouteService,
		IsHttp2:        isHttp2,
	})
	return f.ReturnValue
}

func endpointFor(i int) *route.Endpoint {
	return route.NewEndpoint(&route.EndpointOpts{
		AppId:                fmt.Sprintf("appID%d", i),
		Host:                 fmt.Sprintf("%d.%d.%d.%d", i, i, i, i),
		Port:                 9090,
		PrivateInstanceId:    fmt.Sprintf("instanceID%d", i),
		PrivateInstanceIndex: fmt.Sprintf("%d", i),
		AvailabilityZone:     AZ,
	})
}

var _ = Describe("ProxyRoundTripper", func() {
	Context("RoundTrip", func() {
		var (
			proxyRoundTripper      round_tripper.ProxyRoundTripper
			routePool              *route.EndpointPool
			transport              *roundtripperfakes.FakeProxyRoundTripper
			logger                 *test_util.TestLogger
			req                    *http.Request
			reqBody                *testBody
			resp                   *httptest.ResponseRecorder
			combinedReporter       *fakes.FakeMetricReporter
			roundTripperFactory    *FakeRoundTripperFactory
			routeServicesTransport *sharedfakes.RoundTripper
			retriableClassifier    *errorClassifierFakes.Classifier
			errorHandler           *roundtripperfakes.ErrorHandler
			cfg                    *config.Config

			reqInfo *handlers.RequestInfo

			numEndpoints int
			endpoint     *route.Endpoint

			dialError = &net.OpError{
				Err: errors.New("error"),
				Op:  "dial",
			}
		)

		BeforeEach(func() {
			logger = test_util.NewTestLogger("test")
			routePool = route.NewPool(&route.PoolOpts{
				Logger:                 logger.Logger,
				RetryAfterFailure:      1 * time.Second,
				Host:                   "myapp.com",
				ContextPath:            "",
				MaxConnsPerBackend:     0,
				LoadBalancingAlgorithm: config.LOAD_BALANCE_RR,
			})
			numEndpoints = 1
			resp = httptest.NewRecorder()
			proxyWriter := utils.NewProxyResponseWriter(resp, logger.Logger)
			reqBody = new(testBody)
			req = test_util.NewRequest("GET", "myapp.com", "/", reqBody)
			req.URL.Scheme = "http"

			handlers.NewRequestInfo().ServeHTTP(nil, req, func(_ http.ResponseWriter, transformedReq *http.Request) {
				req = transformedReq
			})

			var err error
			reqInfo, err = handlers.ContextRequestInfo(req)
			Expect(err).ToNot(HaveOccurred())

			reqInfo.RoutePool = routePool
			reqInfo.ProxyResponseWriter = proxyWriter

			transport = new(roundtripperfakes.FakeProxyRoundTripper)
			combinedReporter = new(fakes.FakeMetricReporter)
			errorHandler = &roundtripperfakes.ErrorHandler{}
			roundTripperFactory = &FakeRoundTripperFactory{ReturnValue: transport}
			retriableClassifier = &errorClassifierFakes.Classifier{}
			retriableClassifier.ClassifyReturns(false)
			routeServicesTransport = &sharedfakes.RoundTripper{}

			cfg, err = config.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())
			cfg.EndpointTimeout = 0 * time.Millisecond
			cfg.Backends.MaxAttempts = 3
			cfg.RouteServiceConfig.MaxAttempts = 3
			cfg.StickySessionsForAuthNegotiate = true
		})

		JustBeforeEach(func() {
			for i := 1; i <= numEndpoints; i++ {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:                fmt.Sprintf("appID%d", i),
					Host:                 fmt.Sprintf("%d.%d.%d.%d", i, i, i, i),
					Port:                 9090,
					PrivateInstanceId:    fmt.Sprintf("instanceID%d", i),
					PrivateInstanceIndex: fmt.Sprintf("%d", i),
					AvailabilityZone:     AZ,
				})

				added := routePool.Put(endpoint)
				Expect(added).To(Equal(route.EndpointAdded))
			}

			proxyRoundTripper = round_tripper.NewProxyRoundTripper(
				roundTripperFactory,
				retriableClassifier,
				logger.Logger,
				combinedReporter,
				errorHandler,
				routeServicesTransport,
				cfg,
			)
		})

		Context("RoundTrip", func() {
			Context("when RequestInfo is not set on the request context", func() {
				BeforeEach(func() {
					req = test_util.NewRequest("GET", "myapp.com", "/", nil)
				})

				It("returns an error", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err.Error()).To(ContainSubstring("RequestInfo not set on context"))
				})
			})

			Context("when route pool is not set on the request context", func() {
				BeforeEach(func() {
					reqInfo.RoutePool = nil
				})

				It("returns an error", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err.Error()).To(ContainSubstring("RoutePool not set on context"))
				})
			})

			Context("when ProxyResponseWriter is not set on the request context", func() {
				BeforeEach(func() {
					reqInfo.ProxyResponseWriter = nil
				})

				It("returns an error", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err.Error()).To(ContainSubstring("ProxyResponseWriter not set on context"))
				})
			})

			Context("HTTP headers", func() {
				BeforeEach(func() {
					transport.RoundTripReturns(resp.Result(), nil)
				})

				It("sends X-cf headers", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(transport.RoundTripCallCount()).To(Equal(1))
					outreq := transport.RoundTripArgsForCall(0)
					Expect(outreq.Header.Get("X-CF-ApplicationID")).To(Equal("appID1"))
					Expect(outreq.Header.Get("X-CF-InstanceID")).To(Equal("instanceID1"))
					Expect(outreq.Header.Get("X-CF-InstanceIndex")).To(Equal("1"))
				})
			})

			Context("when some backends fail", func() {
				BeforeEach(func() {
					numEndpoints = 3
					transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
						switch transport.RoundTripCallCount() {
						case 1:
							return nil, &net.OpError{Op: "dial", Err: errors.New("something")}
						case 2:
							return nil, &net.OpError{Op: "dial", Err: errors.New("something")}
						case 3:
							return &http.Response{StatusCode: http.StatusTeapot}, nil
						default:
							return nil, nil
						}
					}

					retriableClassifier.ClassifyReturns(true)
				})

				It("retries until success", func() {
					res, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(transport.RoundTripCallCount()).To(Equal(3))
					Expect(retriableClassifier.ClassifyCallCount()).To(Equal(2))

					Expect(reqInfo.RoundTripSuccessful).To(BeTrue())
					Expect(res.StatusCode).To(Equal(http.StatusTeapot))
				})

				It("captures each routing request to the backend", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).NotTo(HaveOccurred())

					Expect(combinedReporter.CaptureRoutingRequestCallCount()).To(Equal(3))
					// Test if each endpoint has been used
					routePool.Each(func(endpoint *route.Endpoint) {
						found := false
						for i := 0; i < 3; i++ {
							if combinedReporter.CaptureRoutingRequestArgsForCall(i) == endpoint {
								found = true
							}
						}
						Expect(found).To(BeTrue())
					})
				})

				It("logs the error and removes offending backend", func() {
					res, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).NotTo(HaveOccurred())
					routingProps := route.RoutingProperties{
						LocallyOptimistic:      false,
						GlobalRoutingAlgorithm: cfg.LoadBalance,
						AZ:                     AZ,
						RequestHeaders:         &req.Header,
					}

					iter := routePool.Endpoints(logger.Logger, "", false, routingProps)
					ep1 := iter.Next(0)
					ep2 := iter.Next(1)
					Expect(ep1.PrivateInstanceId).To(Equal(ep2.PrivateInstanceId))

					errorLogs := logger.Lines(zap.ErrorLevel)
					Expect(len(errorLogs)).To(BeNumerically(">=", 2))
					count := 0
					for i := 0; i < len(errorLogs); i++ {
						if strings.Contains(errorLogs[i], "backend-endpoint-failed") {
							count++
						}
						Expect(errorLogs[i]).To(ContainSubstring(AZ))
					}
					Expect(count).To(Equal(2))
					Expect(res.StatusCode).To(Equal(http.StatusTeapot))
				})

				It("logs the attempt number", func() {
					res, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(res.StatusCode).To(Equal(http.StatusTeapot))

					errorLogs := logger.Lines(zap.ErrorLevel)
					Expect(len(errorLogs)).To(BeNumerically(">=", 3))
					count := 0
					for i := 0; i < len(errorLogs); i++ {
						if strings.Contains(errorLogs[i], fmt.Sprintf("\"attempt\":%d", count+1)) {
							count++
						}
					}
					Expect(count).To(Equal(2))
				})

				It("does not call the error handler", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(errorHandler.HandleErrorCallCount()).To(Equal(0))
				})

				It("does not log anything about route services", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("with 5 backends, 4 of them failing", func() {
				BeforeEach(func() {
					numEndpoints = 5
					transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
						switch transport.RoundTripCallCount() {
						case 1:
							return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
						case 2:
							return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
						case 3:
							return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
						case 4:
							return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
						case 5:
							return &http.Response{StatusCode: http.StatusTeapot}, nil
						default:
							return nil, nil
						}
					}

					retriableClassifier.ClassifyReturns(true)
				})

				Context("when MaxAttempts is set to 4", func() {
					BeforeEach(func() {
						cfg.Backends.MaxAttempts = 4
					})

					It("stops after 4 tries, returning an error", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(MatchError(ContainSubstring("connection refused")))
						Expect(transport.RoundTripCallCount()).To(Equal(4))
						Expect(retriableClassifier.ClassifyCallCount()).To(Equal(4))
						Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
					})
				})

				Context("when MaxAttempts is set to 10", func() {
					BeforeEach(func() {
						cfg.Backends.MaxAttempts = 10
					})

					It("retries until success", func() {
						res, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(transport.RoundTripCallCount()).To(Equal(5))
						Expect(retriableClassifier.ClassifyCallCount()).To(Equal(4))
						Expect(reqInfo.RoundTripSuccessful).To(BeTrue())
						Expect(res.StatusCode).To(Equal(http.StatusTeapot))
					})
				})
			})

			Context("with 5 backends, all of them failing", func() {
				BeforeEach(func() {
					numEndpoints = 5
					transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
						return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
					}

					retriableClassifier.ClassifyReturns(true)
				})

				Context("when MaxAttempts is set to 4", func() {
					BeforeEach(func() {
						cfg.Backends.MaxAttempts = 4
					})

					It("stops after 4 tries, returning an error", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(MatchError(ContainSubstring("connection refused")))
						Expect(transport.RoundTripCallCount()).To(Equal(4))
						Expect(retriableClassifier.ClassifyCallCount()).To(Equal(4))
						Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
					})
				})

				Context("when MaxAttempts is set to 10", func() {
					BeforeEach(func() {
						cfg.Backends.MaxAttempts = 10
					})

					Context("when no new endpoints were added", func() {
						It("stops after 5 tries when all backends have been tried, returning an error", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).To(MatchError(ContainSubstring("connection refused")))
							Expect(transport.RoundTripCallCount()).To(Equal(5))
							Expect(retriableClassifier.ClassifyCallCount()).To(Equal(5))
							Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
						})
					})

					Context("when no new endpoints were added but some were updated", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
								if transport.RoundTripCallCount() == 1 {
									endpoint := endpointFor(4)
									updated := routePool.Put(endpoint)
									Expect(updated).To(Equal(route.EndpointRefreshed))

									endpoint = endpointFor(5)
									updated = routePool.Put(endpoint)
									Expect(updated).To(Equal(route.EndpointRefreshed))
								}

								return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
							}
						})

						It("stops after 5 tries when all backends have been tried, returning an error", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).To(MatchError(ContainSubstring("connection refused")))
							Expect(transport.RoundTripCallCount()).To(Equal(5))
							Expect(retriableClassifier.ClassifyCallCount()).To(Equal(5))
							Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
						})
					})

					Context("when 2 new endpoints are added after first failure", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
								if transport.RoundTripCallCount() == 1 {
									for i := 6; i <= 7; i++ {
										endpoint := endpointFor(i)
										added := routePool.Put(endpoint)
										Expect(added).To(Equal(route.EndpointAdded))
									}
								}

								return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
							}
						})

						It("retries for new endpoints only", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).To(MatchError(ContainSubstring("connection refused")))
							Expect(transport.RoundTripCallCount()).To(Equal(7))
							Expect(retriableClassifier.ClassifyCallCount()).To(Equal(7))
							Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
						})
					})

					Context("when 1 new endpoint is added and 1 is removed", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
								if transport.RoundTripCallCount() == 2 {
									added := routePool.Put(endpointFor(6))
									Expect(added).To(Equal(route.EndpointAdded))

									removed := routePool.Remove(endpointFor(2))
									Expect(removed).To(BeTrue())
								}

								if transport.RoundTripCallCount() < 5 {
									return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
								} else {
									return &http.Response{StatusCode: http.StatusTeapot}, nil
								}
							}
						})

						It("retries for new endpoints", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).NotTo(HaveOccurred())
							Expect(transport.RoundTripCallCount()).To(Equal(5))
							Expect(retriableClassifier.ClassifyCallCount()).To(Equal(4))
							Expect(reqInfo.RoundTripSuccessful).To(BeTrue())
						})
					})

					Context("when 1 new endpoint is added and 1 is removed on last attempt", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
								if transport.RoundTripCallCount() == 5 {
									added := routePool.Put(endpointFor(6))
									Expect(added).To(Equal(route.EndpointAdded))

									removed := routePool.Remove(endpointFor(2))
									Expect(removed).To(BeTrue())
								}

								if transport.RoundTripCallCount() < 6 {
									return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
								} else {
									return &http.Response{StatusCode: http.StatusTeapot}, nil
								}
							}
						})

						It("retries for new endpoints", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).NotTo(HaveOccurred())
							Expect(transport.RoundTripCallCount()).To(Equal(6))
							Expect(retriableClassifier.ClassifyCallCount()).To(Equal(5))
							Expect(reqInfo.RoundTripSuccessful).To(BeTrue())

							req := transport.RoundTripArgsForCall(5)
							Expect(req.URL.Host).To(Equal("6.6.6.6:9090"))
						})
					})
				})

				Context("when MaxAttempts is set to 0 (illegal value)", func() {
					BeforeEach(func() {
						cfg.Backends.MaxAttempts = 0
					})

					It("still tries once, returning an error", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(MatchError(ContainSubstring("connection refused")))
						Expect(transport.RoundTripCallCount()).To(Equal(1))
						Expect(retriableClassifier.ClassifyCallCount()).To(Equal(1))
						Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
					})
				})

				Context("when MaxAttempts is set to < 0 (illegal value)", func() {
					BeforeEach(func() {
						cfg.Backends.MaxAttempts = -1
					})

					It("still tries once, returning an error", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(MatchError(ContainSubstring("connection refused")))
						Expect(transport.RoundTripCallCount()).To(Equal(1))
						Expect(retriableClassifier.ClassifyCallCount()).To(Equal(1))
						Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
					})
				})
			})

			Context("when backend is unavailable due to non-retriable error", func() {
				BeforeEach(func() {
					badResponse := &http.Response{
						Header: make(map[string][]string),
					}
					badResponse.Header.Add(handlers.VcapRequestIdHeader, "some-request-id")
					transport.RoundTripReturns(badResponse, &net.OpError{Op: "remote error", Err: errors.New("tls: handshake failure")})
					retriableClassifier.ClassifyReturns(false)
				})

				It("does not retry", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(MatchError(ContainSubstring("tls: handshake failure")))
					Expect(transport.RoundTripCallCount()).To(Equal(1))

					Expect(reqInfo.RouteEndpoint).To(Equal(endpoint))
					Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
				})

				It("captures each routing request to the backend", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(MatchError(ContainSubstring("tls: handshake failure")))

					Expect(combinedReporter.CaptureRoutingRequestCallCount()).To(Equal(1))
					Expect(combinedReporter.CaptureRoutingRequestArgsForCall(0)).To(Equal(endpoint))
				})

				It("calls the error handler", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(HaveOccurred())
					Expect(errorHandler.HandleErrorCallCount()).To(Equal(1))
					_, err = errorHandler.HandleErrorArgsForCall(0)
					Expect(err).To(MatchError(ContainSubstring("tls: handshake failure")))
				})

				It("does not log anything about route services", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(MatchError(ContainSubstring("tls: handshake failure")))

					Eventually(logger).ShouldNot(gbytes.Say(`route-service`))
				})

				It("does log the error and reports the endpoint failure", func() {
					endpoint = route.NewEndpoint(&route.EndpointOpts{
						AppId:                "appId2",
						Host:                 "2.2.2.2",
						Port:                 8080,
						PrivateInstanceId:    "instanceId2",
						PrivateInstanceIndex: "2",
					})

					routingProps := route.RoutingProperties{
						LocallyOptimistic:      false,
						GlobalRoutingAlgorithm: cfg.LoadBalance,
						AZ:                     AZ,
						RequestHeaders:         &req.Header,
					}

					added := routePool.Put(endpoint)
					Expect(added).To(Equal(route.EndpointAdded))

					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(MatchError(ContainSubstring("tls: handshake failure")))

					iter := routePool.Endpoints(logger.Logger, "", false, routingProps)
					ep1 := iter.Next(0)
					ep2 := iter.Next(1)
					Expect(ep1).To(Equal(ep2))

					logOutput := logger.Buffer()
					Eventually(logOutput).Should(gbytes.Say(`backend-endpoint-failed`))
					Eventually(logOutput).Should(gbytes.Say(`vcap_request_id`))
				})
			})

			Context("with two endpoints, one of them failing", func() {
				BeforeEach(func() {
					numEndpoints = 2
				})

				DescribeTable("when the backend fails with an empty response error (io.EOF)",
					func(reqBody io.ReadCloser, getBodyIsNil bool, reqMethod string, headers map[string]string, classify fails.ClassifierFunc, expectRetry bool) {
						badResponse := &http.Response{
							Header: make(map[string][]string),
						}
						badResponse.Header.Add(handlers.VcapRequestIdHeader, "some-request-id")

						// The first request fails with io.EOF, the second (if retried) succeeds
						transport.RoundTripStub = func(*http.Request) (*http.Response, error) {
							switch transport.RoundTripCallCount() {
							case 1:
								return nil, io.EOF
							case 2:
								return &http.Response{StatusCode: http.StatusTeapot}, nil
							default:
								return nil, nil
							}
						}

						retriableClassifier.ClassifyStub = classify
						req.Method = reqMethod
						req.Body = reqBody
						if !getBodyIsNil {
							req.GetBody = func() (io.ReadCloser, error) {
								return new(testBody), nil
							}
						}
						for key, value := range headers {
							req.Header.Add(key, value)
						}

						res, err := proxyRoundTripper.RoundTrip(req)

						if expectRetry {
							Expect(err).NotTo(HaveOccurred())
							Expect(transport.RoundTripCallCount()).To(Equal(2))
							Expect(retriableClassifier.ClassifyCallCount()).To(Equal(1))
							Expect(res.StatusCode).To(Equal(http.StatusTeapot))
						} else {
							Expect(errors.Is(err, io.EOF)).To(BeTrue())
							Expect(transport.RoundTripCallCount()).To(Equal(1))
							Expect(retriableClassifier.ClassifyCallCount()).To(Equal(1))
						}
					},

					Entry("POST, body is empty: does not retry", nil, true, "POST", nil, fails.IdempotentRequestEOF, false),
					Entry("POST, body is not empty and GetBody is non-nil: does not retry", reqBody, false, "POST", nil, fails.IdempotentRequestEOF, false),
					Entry("POST, body is not empty: does not retry", reqBody, true, "POST", nil, fails.IdempotentRequestEOF, false),
					Entry("POST, body is http.NoBody: does not retry", http.NoBody, true, "POST", nil, fails.IdempotentRequestEOF, false),

					Entry("POST, body is empty, X-Idempotency-Key header: attempts retry", nil, true, "POST", map[string]string{"X-Idempotency-Key": "abc123"}, fails.IncompleteRequest, true),
					Entry("POST, body is not empty and GetBody is non-nil, X-Idempotency-Key header: attempts retry", reqBody, false, "POST", map[string]string{"X-Idempotency-Key": "abc123"}, fails.IncompleteRequest, true),
					Entry("POST, body is not empty, X-Idempotency-Key header: does not retry", reqBody, true, "POST", map[string]string{"X-Idempotency-Key": "abc123"}, fails.IdempotentRequestEOF, false),
					Entry("POST, body is http.NoBody, X-Idempotency-Key header: does not retry", http.NoBody, true, "POST", map[string]string{"X-Idempotency-Key": "abc123"}, fails.IdempotentRequestEOF, false),

					Entry("POST, body is empty, Idempotency-Key header: attempts retry", nil, true, "POST", map[string]string{"Idempotency-Key": "abc123"}, fails.IncompleteRequest, true),
					Entry("POST, body is not empty and GetBody is non-nil, Idempotency-Key header: attempts retry", reqBody, false, "POST", map[string]string{"Idempotency-Key": "abc123"}, fails.IncompleteRequest, true),
					Entry("POST, body is not empty, Idempotency-Key header: does not retry", reqBody, true, "POST", map[string]string{"Idempotency-Key": "abc123"}, fails.IdempotentRequestEOF, false),
					Entry("POST, body is http.NoBody, Idempotency-Key header: does not retry", http.NoBody, true, "POST", map[string]string{"Idempotency-Key": "abc123"}, fails.IdempotentRequestEOF, false),

					Entry("GET, body is empty: attempts retry", nil, true, "GET", nil, fails.IncompleteRequest, true),
					Entry("GET, body is not empty and GetBody is non-nil: attempts retry", reqBody, false, "GET", nil, fails.IncompleteRequest, true),
					Entry("GET, body is not empty: does not retry", reqBody, true, "GET", nil, fails.IdempotentRequestEOF, false),
					Entry("GET, body is http.NoBody: does not retry", http.NoBody, true, "GET", nil, fails.IdempotentRequestEOF, false),

					Entry("TRACE, body is empty: attempts retry", nil, true, "TRACE", nil, fails.IncompleteRequest, true),
					Entry("TRACE, body is not empty: does not retry", reqBody, true, "TRACE", nil, fails.IdempotentRequestEOF, false),
					Entry("TRACE, body is http.NoBody: does not retry", http.NoBody, true, "TRACE", nil, fails.IdempotentRequestEOF, false),
					Entry("TRACE, body is not empty and GetBody is non-nil: attempts retry", reqBody, false, "TRACE", nil, fails.IncompleteRequest, true),

					Entry("HEAD, body is empty: attempts retry", nil, true, "HEAD", nil, fails.IncompleteRequest, true),
					Entry("HEAD, body is not empty: does not retry", reqBody, true, "HEAD", nil, fails.IdempotentRequestEOF, false),
					Entry("HEAD, body is http.NoBody: does not retry", http.NoBody, true, "HEAD", nil, fails.IdempotentRequestEOF, false),
					Entry("HEAD, body is not empty and GetBody is non-nil: attempts retry", reqBody, false, "HEAD", nil, fails.IncompleteRequest, true),

					Entry("OPTIONS, body is empty: attempts retry", nil, true, "OPTIONS", nil, fails.IncompleteRequest, true),
					Entry("OPTIONS, body is not empty and GetBody is non-nil: attempts retry", reqBody, false, "OPTIONS", nil, fails.IncompleteRequest, true),
					Entry("OPTIONS, body is not empty: does not retry", reqBody, true, "OPTIONS", nil, fails.IdempotentRequestEOF, false),
					Entry("OPTIONS, body is http.NoBody: does not retry", http.NoBody, true, "OPTIONS", nil, fails.IdempotentRequestEOF, false),

					Entry("<empty method>, body is empty: attempts retry", nil, true, "", nil, fails.IncompleteRequest, true),
					Entry("<empty method>, body is not empty and GetBody is non-nil: attempts retry", reqBody, false, "", nil, fails.IncompleteRequest, true),
					Entry("<empty method>, body is not empty: does not retry", reqBody, true, "", nil, fails.IdempotentRequestEOF, false),
					Entry("<empty method>, body is http.NoBody: does not retry", http.NoBody, true, "", nil, fails.IdempotentRequestEOF, false),
				)
			})

			Context("when there are no more endpoints available", func() {
				JustBeforeEach(func() {
					removed := routePool.Remove(endpoint)
					Expect(removed).To(BeTrue())
				})

				It("returns a 502 Bad Gateway response", func() {
					backendRes, err := proxyRoundTripper.RoundTrip(req)
					Expect(backendRes).To(BeNil())
					Expect(err).To(Equal(round_tripper.NoEndpointsAvailable))

					Expect(reqInfo.RouteEndpoint).To(BeNil())
					Expect(reqInfo.RoundTripSuccessful).To(BeFalse())
				})

				It("calls the error handler", func() {
					proxyRoundTripper.RoundTrip(req)
					Expect(errorHandler.HandleErrorCallCount()).To(Equal(1))
					_, err := errorHandler.HandleErrorArgsForCall(0)
					Expect(err).To(Equal(round_tripper.NoEndpointsAvailable))
				})

				It("logs a message with `select-endpoint-failed`", func() {
					proxyRoundTripper.RoundTrip(req)
					Eventually(logger).Should(gbytes.Say(`select-endpoint-failed`))
					Eventually(logger).Should(gbytes.Say(`myapp.com`))
				})

				It("does not capture any routing requests to the backend", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(Equal(round_tripper.NoEndpointsAvailable))

					Expect(combinedReporter.CaptureRoutingRequestCallCount()).To(Equal(0))
				})

				It("does not log anything about route services", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(Equal(round_tripper.NoEndpointsAvailable))

					Eventually(logger).ShouldNot(gbytes.Say(`route-service`))
				})

				It("does not report the endpoint failure", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).To(MatchError(round_tripper.NoEndpointsAvailable))

					Eventually(logger).ShouldNot(gbytes.Say(`backend-endpoint-failed`))
				})
			})

			Context("when the request succeeds", func() {
				BeforeEach(func() {
					transport.RoundTripReturns(
						&http.Response{StatusCode: http.StatusTeapot}, nil,
					)
				})

				It("returns the exact response received from the backend", func() {
					resp, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusTeapot))
				})

				It("does not log an error or report the endpoint failure", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())

					Eventually(logger).ShouldNot(gbytes.Say(`backend-endpoint-failed`))
				})

				It("does not log anything about route services", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())

					Eventually(logger).ShouldNot(gbytes.Say(`route-service`))
				})

			})

			Context("when there are a mixture of tls and non-tls backends", func() {
				BeforeEach(func() {
					tlsEndpoint := route.NewEndpoint(&route.EndpointOpts{
						Host:   "2.2.2.2",
						Port:   20222,
						UseTLS: true,
					})
					Expect(routePool.Put(tlsEndpoint)).To(Equal(route.EndpointAdded))

					nonTLSEndpoint := route.NewEndpoint(&route.EndpointOpts{
						Host:   "3.3.3.3",
						Port:   30333,
						UseTLS: false,
					})
					Expect(routePool.Put(nonTLSEndpoint)).To(Equal(route.EndpointAdded))
				})

				Context("when retrying different backends", func() {
					var (
						recordedRequests map[string]string
						mutex            sync.Mutex
					)

					BeforeEach(func() {
						recordedRequests = map[string]string{}
						transport.RoundTripStub = func(r *http.Request) (*http.Response, error) {
							mutex.Lock()
							defer mutex.Unlock()
							recordedRequests[r.URL.Host] = r.URL.Scheme
							return nil, errors.New("potato")
						}
						retriableClassifier.ClassifyReturns(true)
					})

					It("uses the correct url scheme (protocol) for each backend", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(HaveOccurred())
						Expect(transport.RoundTripCallCount()).To(Equal(3))
						Expect(retriableClassifier.ClassifyCallCount()).To(Equal(3))

						Expect(recordedRequests).To(Equal(map[string]string{
							"1.1.1.1:9090":  "http",
							"2.2.2.2:20222": "https",
							"3.3.3.3:30333": "http",
						}))
					})
				})
			})

			Context("when backend is registered with a tls port", func() {
				JustBeforeEach(func() {
					var oldEndpoints []*route.Endpoint
					routePool.Each(func(endpoint *route.Endpoint) {
						oldEndpoints = append(oldEndpoints, endpoint)
					})

					for _, ep := range oldEndpoints {
						routePool.Remove(ep)
					}

					Expect(routePool.IsEmpty()).To(BeTrue())
					endpoint = route.NewEndpoint(&route.EndpointOpts{
						Host:   "1.1.1.1",
						Port:   9090,
						UseTLS: true,
					})

					added := routePool.Put(endpoint)
					Expect(added).To(Equal(route.EndpointAdded))
					transport.RoundTripReturns(
						&http.Response{StatusCode: http.StatusTeapot}, nil,
					)
				})

				It("should set request URL scheme to https", func() {
					resp, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(transport.RoundTripCallCount()).To(Equal(1))
					transformedReq := transport.RoundTripArgsForCall(0)
					Expect(transformedReq.URL.Scheme).To(Equal("https"))
					Expect(resp.StatusCode).To(Equal(http.StatusTeapot))
				})

				Context("when the backend is registered with a non-tls port", func() {
					JustBeforeEach(func() {
						endpoint = route.NewEndpoint(&route.EndpointOpts{
							Host: "1.1.1.1",
							Port: 9090,
						})

						added := routePool.Put(endpoint)
						Expect(added).To(Equal(route.EndpointUpdated))
						transport.RoundTripReturns(
							&http.Response{StatusCode: http.StatusTeapot}, nil,
						)
					})

					It("should set request URL scheme to http", func() {
						resp, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).ToNot(HaveOccurred())
						Expect(transport.RoundTripCallCount()).To(Equal(1))
						transformedReq := transport.RoundTripArgsForCall(0)
						Expect(transformedReq.URL.Scheme).To(Equal("http"))
						Expect(resp.StatusCode).To(Equal(http.StatusTeapot))
					})
				})
			})

			Context("transport re-use", func() {
				It("re-uses transports for the same endpoint", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
						{IsRouteService: false, IsHttp2: false},
					}))

					_, err = proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
						{IsRouteService: false, IsHttp2: false},
					}))
				})

				It("does not re-use transports between endpoints", func() {
					endpoint = route.NewEndpoint(&route.EndpointOpts{
						Host: "1.1.1.1", Port: 9091, UseTLS: true, PrivateInstanceId: "instanceId-2",
					})
					added := routePool.Put(endpoint)
					Expect(added).To(Equal(route.EndpointAdded))

					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
						{IsRouteService: false, IsHttp2: false},
					}))

					_, err = proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
						{IsRouteService: false, IsHttp2: false},
						{IsRouteService: false, IsHttp2: false},
					}))

					_, err = proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
						{IsRouteService: false, IsHttp2: false},
						{IsRouteService: false, IsHttp2: false},
					}))
				})
			})

			Context("using HTTP/2", func() {
				Context("when HTTP/2 is enabled", func() {
					BeforeEach(func() {
						cfg.EnableHTTP2 = true
					})
					It("uses HTTP/2 when endpoint's Protocol is set to http2", func() {
						endpoint.Protocol = "http2"
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).ToNot(HaveOccurred())
						Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
							{IsRouteService: false, IsHttp2: true},
						}))
					})

					It("does not use HTTP/2 when endpoint's Protocol is not set to http2", func() {
						endpoint.Protocol = ""
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).ToNot(HaveOccurred())
						Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
							{IsRouteService: false, IsHttp2: false},
						}))
					})
				})

				Context("when HTTP/2 is disabled", func() {
					BeforeEach(func() {
						cfg.EnableHTTP2 = false
					})

					It("does not use HTTP/2, regardless of the endpoint's protocol", func() {
						endpoint.Protocol = "http2"
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).ToNot(HaveOccurred())
						Expect(roundTripperFactory.RequestedRoundTripperTypes).To(Equal([]RequestedRoundTripperType{
							{IsRouteService: false, IsHttp2: false},
						}))
					})
				})
			})

			Context("when the request context contains a Route Service URL", func() {
				var routeServiceURL *url.URL
				BeforeEach(func() {
					var err error
					routeServiceURL, err = url.Parse("https://foo.com")
					Expect(err).ToNot(HaveOccurred())
					reqInfo.RouteServiceURL = routeServiceURL
					transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
						Expect(req.Host).To(Equal(routeServiceURL.Host))
						Expect(req.URL).To(Equal(routeServiceURL))
						return nil, nil
					}
				})

				It("makes requests to the route service", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
				})

				It("does not capture the routing request in metrics", func() {
					_, err := proxyRoundTripper.RoundTrip(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(combinedReporter.CaptureRoutingRequestCallCount()).To(Equal(0))
				})

				Context("when the route service returns a non-2xx status code", func() {
					BeforeEach(func() {
						transport.RoundTripReturns(
							&http.Response{StatusCode: http.StatusTeapot}, nil,
						)

					})

					It("logs the response error", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).ToNot(HaveOccurred())
						Eventually(logger).Should(gbytes.Say(`response.*status-code":418`))
					})
				})

				Context("when the route service is an internal route service", func() {
					BeforeEach(func() {
						reqInfo.ShouldRouteToInternalRouteService = true
						transport.RoundTripStub = nil
						transport.RoundTripReturns(nil, nil)
					})

					It("uses the route services round tripper to make the request", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(BeNil())
						Expect(transport.RoundTripCallCount()).To(Equal(0))
						Expect(routeServicesTransport.RoundTripCallCount()).To(Equal(1))

						outReq := routeServicesTransport.RoundTripArgsForCall(0)
						Expect(outReq.Host).To(Equal(routeServiceURL.Host))
					})
				})

				Context("when the route service request fails", func() {
					BeforeEach(func() {
						transport.RoundTripReturns(
							nil, dialError,
						)
						retriableClassifier.ClassifyReturns(true)
					})

					It("calls the error handler", func() {
						proxyRoundTripper.RoundTrip(req)
						Expect(errorHandler.HandleErrorCallCount()).To(Equal(1))

						_, err := errorHandler.HandleErrorArgsForCall(0)
						Expect(err).To(Equal(dialError))
					})

					It("logs the failure", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(MatchError(dialError))

						Eventually(logger).ShouldNot(gbytes.Say(`backend-endpoint-failed`))
						for i := 0; i < 3; i++ {
							Eventually(logger).Should(gbytes.Say(`route-service-connection-failed`))
							Eventually(logger).Should(gbytes.Say(`foo.com`))
						}
					})

					Context("when MaxAttempts is set to 5", func() {
						BeforeEach(func() {
							cfg.RouteServiceConfig.MaxAttempts = 5
						})

						It("tries for 5 times before giving up", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).To(MatchError(dialError))
							Expect(transport.RoundTripCallCount()).To(Equal(5))
						})
					})

					Context("when route service is unavailable due to non-retriable error", func() {
						BeforeEach(func() {
							transport.RoundTripReturns(nil, errors.New("banana"))
							retriableClassifier.ClassifyReturns(false)
						})

						It("does not retry and returns status bad gateway", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).To(MatchError(errors.New("banana")))
							Expect(transport.RoundTripCallCount()).To(Equal(1))
						})

						It("calls the error handler", func() {
							proxyRoundTripper.RoundTrip(req)
							Expect(errorHandler.HandleErrorCallCount()).To(Equal(1))
							_, err := errorHandler.HandleErrorArgsForCall(0)
							Expect(err).To(MatchError("banana"))
						})

						It("logs the error", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).To(MatchError("banana"))
							Eventually(logger).Should(gbytes.Say(`route-service-connection-failed`))
							Eventually(logger).Should(gbytes.Say(`foo.com`))
						})
					})
				})
			})

			Context("when using sticky sessions", func() {
				var (
					sessionCookie     *http.Cookie
					endpoint1         *route.Endpoint
					endpoint2         *route.Endpoint
					defaultExpiryDate = time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC)

					// options for transport.RoundTripStub
					responseContainsNoCookies                     func(req *http.Request) (*http.Response, error)
					responseContainsJSESSIONID                    func(req *http.Request) (*http.Response, error)
					responseContainsJSESSIONIDWithExtraProperties func(req *http.Request) (*http.Response, error)
					responseContainsPartitionedJSESSIONID         func(req *http.Request) (*http.Response, error)
					responseContainsVCAPID                        func(req *http.Request) (*http.Response, error)
					responseContainsJSESSIONIDAndVCAPID           func(req *http.Request) (*http.Response, error)
				)

				setJSESSIONID := func(req *http.Request, resp *http.Response, setExtraProperties bool, partitioned bool) (response *http.Response) {

					//Attach the same JSESSIONID on to the response if it exists on the request
					if len(req.Cookies()) > 0 && !setExtraProperties && !partitioned {
						resp.Header.Add(round_tripper.CookieHeader, req.Cookies()[0].String())
						return resp
					}

					if setExtraProperties {
						sessionCookie.SameSite = http.SameSiteStrictMode
						sessionCookie.Expires = defaultExpiryDate
						sessionCookie.Secure = true
						sessionCookie.HttpOnly = true
					}

					if partitioned {
						sessionCookie.Partitioned = true
					}

					sessionCookie.Value, _ = uuid.GenerateUUID()
					resp.Header.Add(round_tripper.CookieHeader, sessionCookie.String())
					return resp
				}

				setVCAPID := func(resp *http.Response) (response *http.Response) {
					vcapCookie := http.Cookie{
						Name:  round_tripper.VcapCookieId,
						Value: "vcap-id-property-already-on-the-response",
					}

					if c := vcapCookie.String(); c != "" {
						resp.Header.Add(round_tripper.CookieHeader, c)
					}

					return resp
				}

				setAuthorizationNegotiateHeader := func(resp *http.Response) (response *http.Response) {
					resp.Header.Add("WWW-Authenticate", "Negotiate SOME-TOKEN")
					return resp
				}

				responseContainsNoCookies = func(req *http.Request) (*http.Response, error) {
					resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
					return resp, nil
				}

				responseContainsJSESSIONID = func(req *http.Request) (*http.Response, error) {
					resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
					setJSESSIONID(req, resp, false, false)
					return resp, nil
				}

				responseContainsVCAPID = func(req *http.Request) (*http.Response, error) {
					resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
					setVCAPID(resp)
					return resp, nil
				}

				responseContainsJSESSIONIDAndVCAPID = func(req *http.Request) (*http.Response, error) {
					resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
					setJSESSIONID(req, resp, false, false)
					setVCAPID(resp)
					return resp, nil
				}

				// Non-partitioned JSESSIONID with extra properties (Secure, HttpOnly, SameSite, Expires)
				responseContainsJSESSIONIDWithExtraProperties = func(req *http.Request) (*http.Response, error) {
					resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
					setJSESSIONID(req, resp, true, false)
					return resp, nil
				}

				// Partitioned JSESSIONID with all properties
				responseContainsPartitionedJSESSIONID = func(req *http.Request) (*http.Response, error) {
					resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
					setJSESSIONID(req, resp, true, true)
					return resp, nil
				}

				JustBeforeEach(func() {
					sessionCookie = &http.Cookie{
						Name: StickyCookieKey, //JSESSIONID
					}

					endpoint1 = route.NewEndpoint(&route.EndpointOpts{
						Host: "1.1.1.1", Port: 9091, PrivateInstanceId: "id-1",
					})
					endpoint2 = route.NewEndpoint(&route.EndpointOpts{
						Host: "1.1.1.1", Port: 9092, PrivateInstanceId: "id-2",
					})

					added := routePool.Put(endpoint1)
					Expect(added).To(Equal(route.EndpointAdded))
					added = routePool.Put(endpoint2)
					Expect(added).To(Equal(route.EndpointAdded))
					removed := routePool.Remove(endpoint)
					Expect(removed).To(BeTrue())
				})

				expectVcapIdCookie := func(cookie *http.Cookie, expectedInstanceIds ...string) {
					ExpectWithOffset(1, cookie.Name).To(Equal(round_tripper.VcapCookieId))
					if len(expectedInstanceIds) > 0 {
						ExpectWithOffset(1, cookie.Value).To(BeElementOf(expectedInstanceIds))
					}
				}

				expectMetaCookie := func(cookie *http.Cookie, checkFn func(value string)) {
					ExpectWithOffset(1, cookie.Name).To(Equal(round_tripper.VcapMetaCookieId))
					if checkFn != nil {
						checkFn(cookie.Value)
					}
				}

				Describe("isSessionCookie", func() {
					It("matches an exact cookie name", func() {
						Expect(round_tripper.IsSessionCookie("JSESSIONID", cfg.StickySessionCookieNames)).To(BeTrue())
					})

					It("matches a __Host- prefixed cookie name", func() {
						Expect(round_tripper.IsSessionCookie("__Host-JSESSIONID", cfg.StickySessionCookieNames)).To(BeTrue())
					})

					It("does not match an unknown cookie name", func() {
						Expect(round_tripper.IsSessionCookie("UNKNOWN", cfg.StickySessionCookieNames)).To(BeFalse())
					})

					It("does not match a __Host- prefixed unknown cookie name", func() {
						Expect(round_tripper.IsSessionCookie("__Host-UNKNOWN", cfg.StickySessionCookieNames)).To(BeFalse())
					})

					It("does not match partial prefix like __Host without dash", func() {
						Expect(round_tripper.IsSessionCookie("__HostJSESSIONID", cfg.StickySessionCookieNames)).To(BeFalse())
					})

					It("does not match other casings of the __Host- prefix", func() {
						Expect(round_tripper.IsSessionCookie("__HOST-JSESSIONID", cfg.StickySessionCookieNames)).To(BeFalse())
						Expect(round_tripper.IsSessionCookie("__host-JSESSIONID", cfg.StickySessionCookieNames)).To(BeFalse())
						Expect(round_tripper.IsSessionCookie("__HoSt-JSESSIONID", cfg.StickySessionCookieNames)).To(BeFalse())
					})
				})

				Context("Early Return: when the backend already sets VCAP_ID on the response", func() {
					// Gorouter must never overwrite a __VCAP_ID__ cookie that the backend sets itself.
					// This is an early-return guard in setupStickySession that applies regardless of
					// which scenario (Auth Negotiate, New Session, Stale Session) would otherwise trigger.

					Context("when only VCAP_ID is set on the response", func() {
						BeforeEach(func() {
							transport.RoundTripStub = responseContainsVCAPID
						})

						It("does not overwrite it", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(1))
							Expect(cookies[0].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[0].Value).To(Equal("vcap-id-property-already-on-the-response"))
						})
					})

					Context("when both JSESSIONID and VCAP_ID are set on the response", func() {
						BeforeEach(func() {
							transport.RoundTripStub = responseContainsJSESSIONIDAndVCAPID
						})

						It("does not add a second VCAP_ID or META cookie", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(2))
							Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
							Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[1].Value).To(Equal("vcap-id-property-already-on-the-response"))
						})
					})
				})

				Context("Auth Negotiation Scenario", func() {
					Context("when VCAP_ID cookie and 'Authorization: Negotiate ...' header are on the request", func() {
						BeforeEach(func() {
							req.AddCookie(&http.Cookie{
								Name:  round_tripper.VcapCookieId,
								Value: "id-2",
							})
							req.Header.Add("Authorization", "Negotiate SOME-TOKEN")
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								Expect(req.URL.Host).To(Equal("1.1.1.1:9092"))
								resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
								return resp, nil
							}
						})

						It("routes the request to the sticky endpoint and does not set VCAP_ID (endpoint unchanged)", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())
							// Backend selection is verified in the transport stub above
							Expect(resp.Cookies()).To(HaveLen(0))
						})
					})

					Context("when VCAP_ID is already set on the request and the response contains 'WWW-Authenticate: Negotiate'", func() {
						// When a client already has a valid sticky session (VCAP_ID points to a live endpoint),
						// and the backend responds with WWW-Authenticate: Negotiate (e.g., a Kerberos handshake
						// round-trip), gorouter must NOT re-issue the VCAP_ID cookie. The endpoint has not
						// changed, so there is nothing to update.
						BeforeEach(func() {
							req.AddCookie(&http.Cookie{
								Name:  round_tripper.VcapCookieId,
								Value: "id-2",
							})
							// Authorization: Negotiate on the request is what enables sticky routing via __VCAP_ID__.
							req.Header.Add("Authorization", "Negotiate SOME-TOKEN")
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								// The request is correctly routed to endpoint2 (id-2) — no endpoint change.
								Expect(req.URL.Host).To(Equal("1.1.1.1:9092"))
								resp := &http.Response{StatusCode: http.StatusUnauthorized, Header: make(map[string][]string)}
								resp.Header.Add("WWW-Authenticate", "Negotiate SOME-TOKEN")
								return resp, nil
							}
						})

						It("does not set VCAP_ID on the response (endpoint unchanged, no cookie update needed)", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							Expect(resp.Cookies()).To(HaveLen(0))
						})
					})

					Context("when a JSESSIONID sticky session is stale and the new backend responds with 'WWW-Authenticate: Negotiate'", func() {
						// A client has an active JSESSIONID sticky session (so originalEndpointId is set)
						// but the pinned endpoint is gone. The new backend responds with WWW-Authenticate:
						// Negotiate. Gorouter must re-issue VCAP_ID with the fixed Negotiate defaults
						// (MaxAge=60, SameSite=Strict) — not attempt to restore attributes from
						// __VCAP_ID_META__ (which encodes JSESSIONID attributes, not Negotiate defaults).
						JustBeforeEach(func() {
							// First request: establish a JSESSIONID session so JSESSIONID + VCAP_ID + META are on req.
							transport.RoundTripStub = responseContainsJSESSIONID
							firstResp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())
							for _, c := range firstResp.Cookies() {
								req.AddCookie(c)
							}
							// Swap out the endpoint so endpointChanged=true on the next request.
							routePool.Remove(endpoint1)
							routePool.Remove(endpoint2)
							newEndpoint := route.NewEndpoint(&route.EndpointOpts{PrivateInstanceId: "id-new-backend"})
							routePool.Put(newEndpoint)
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								resp := &http.Response{StatusCode: http.StatusUnauthorized, Header: make(map[string][]string)}
								resp.Header.Add("WWW-Authenticate", "Negotiate SOME-TOKEN")
								return resp, nil
							}
						})

						It("sets VCAP_ID with Negotiate defaults (MaxAge=60, SameSite=Strict) on the new backend", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(1))
							Expect(cookies[0].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[0].Value).To(Equal("id-new-backend"))
							Expect(cookies[0].MaxAge).To(Equal(round_tripper.AuthNegotiateHeaderCookieMaxAgeInSeconds))
							Expect(cookies[0].SameSite).To(Equal(http.SameSiteStrictMode))
							Expect(cookies[0].Expires).To(Equal(time.Time{}))
						})
					})

					Context("when there is an 'WWW-Authenticate: Negotiate ...' header set on the response", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
								setAuthorizationNegotiateHeader(resp)
								return resp, nil
							}
						})

						It("will select an endpoint and set VCAP_ID to the privateInstanceId", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(1))
							Expect(cookies[0].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[0].Value).To(SatisfyAny(
								Equal("id-1"),
								Equal("id-2")))
							Expect(cookies[0].MaxAge).To(Equal(60))
							Expect(cookies[0].Expires).To(Equal(time.Time{}))
							Expect(cookies[0].Secure).To(Equal(cfg.SecureCookies))
							Expect(cookies[0].SameSite).To(Equal(http.SameSiteStrictMode))
						})

						Context("when there is also JSESSIONID cookie with extra properties set", func() {
							BeforeEach(func() {
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
									setAuthorizationNegotiateHeader(resp)
									setJSESSIONID(req, resp, true, false)
									return resp, nil
								}
							})

							It("sets the auth negotiate default properties on VCAP_ID", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								Expect(cookies).To(HaveLen(2))
								Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
								Expect(sessionCookie.String()).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))

								Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(cookies[1].Value).To(SatisfyAny(
									Equal("id-1"),
									Equal("id-2")))
								Expect(cookies[1].Raw).To(ContainSubstring("Max-Age=60; HttpOnly; SameSite=Strict"))
							})

							Context("when config requires secure cookies", func() {
								BeforeEach(func() {
									cfg.SecureCookies = true
								})

								It("sets the auth negotiate default properties with Secure on VCAP_ID", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									cookies := resp.Cookies()
									Expect(cookies).To(HaveLen(2))
									Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
									Expect(sessionCookie.String()).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))

									Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(cookies[1].Value).To(SatisfyAny(
										Equal("id-1"),
										Equal("id-2")))
									Expect(cookies[1].Raw).To(ContainSubstring("Max-Age=60; HttpOnly; Secure; SameSite=Strict"))
								})
							})
						})
						Context("when sticky sessions for 'Authorization: Negotiate' is disabled", func() {
							BeforeEach(func() {
								cfg.StickySessionsForAuthNegotiate = false
							})
							It("does not set VCAP_ID cookie", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								Expect(cookies).To(HaveLen(0))
							})
							Context("when there is also a JSESSIONID cookie on the response", func() {
								// Even though Negotiate sticky sessions are disabled, the JSESSIONID code path
								// is independent and still applies: VCAP_ID is set from the JSESSIONID attributes.
								BeforeEach(func() {
									transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
										resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
										setAuthorizationNegotiateHeader(resp)
										setJSESSIONID(req, resp, true, false)
										return resp, nil
									}
								})
								It("sets VCAP_ID via the JSESSIONID path (Negotiate path is skipped)", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									cookies := resp.Cookies()
									Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META
									Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
									Expect(sessionCookie.String()).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))
									Expect(sessionCookie.String()).ToNot(ContainSubstring("Partitioned"))

									expectVcapIdCookie(cookies[1], "id-1", "id-2")
									Expect(cookies[1].Raw).ToNot(ContainSubstring("Partitioned"))
								})
							})
						})
					})

				})

				Context("New Session Scenario", func() {

					Context("When the response contains a JSESSIONID cookie", func() {
						BeforeEach(func() {
							transport.RoundTripStub = responseContainsJSESSIONID
						})

						It("selects an endpoint and sets VCAP_ID and VCAP_ID_META", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META
							Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
							expectVcapIdCookie(cookies[1], "id-1", "id-2")
							expectMetaCookie(cookies[2], nil)
						})
					})

					Context("when JSESSIONID is non-partitioned with Secure, SameSite=Strict, and an Expires date", func() {
						BeforeEach(func() {
							transport.RoundTripStub = responseContainsJSESSIONIDWithExtraProperties
						})

						It("creates non-partitioned VCAP_ID cookies with matching attributes", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META (all non-partitioned)
							Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
							Expect(sessionCookie.String()).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))
							Expect(sessionCookie.String()).ToNot(ContainSubstring("Partitioned"))

							expectVcapIdCookie(cookies[1], "id-1", "id-2")
							Expect(cookies[1].Raw).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))
							Expect(cookies[1].Partitioned).To(BeFalse())
							expectMetaCookie(cookies[2], nil)
							Expect(cookies[2].Partitioned).To(BeFalse())
						})

						It("encodes secure, samesite=strict, and expires= in VCAP_ID_META", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META
							expectMetaCookie(cookies[2], func(value string) {
								Expect(value).To(ContainSubstring("secure"))
								Expect(value).To(ContainSubstring("samesite=strict"))
								// expires= holds the raw Unix timestamp from the original Expires header
								Expect(value).To(ContainSubstring("expires="))
								Expect(value).ToNot(ContainSubstring("maxage="))
								params, _ := url.ParseQuery(value)
								v, err := strconv.ParseInt(params.Get("expires"), 10, 64)
								Expect(err).ToNot(HaveOccurred())
								// Expires=Wed, 01 Jan 2020 01:00:00 GMT → Unix timestamp 1577840400
								Expect(v).To(Equal(int64(1577840400)))
							})
						})
					})

					Context("when JSESSIONID is partitioned", func() {
						BeforeEach(func() {
							transport.RoundTripStub = responseContainsPartitionedJSESSIONID
						})

						It("creates partitioned VCAP_ID and VCAP_ID_META cookies with partitioned in meta", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META (all partitioned)

							// JSESSIONID is partitioned
							Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
							Expect(sessionCookie.String()).To(ContainSubstring("Partitioned"))

							// VCAP_ID is partitioned
							expectVcapIdCookie(cookies[1], "id-1", "id-2")
							Expect(cookies[1].Partitioned).To(BeTrue())

							// VCAP_ID_META carries partitioned and is itself partitioned
							expectMetaCookie(cookies[2], func(value string) {
								Expect(value).To(ContainSubstring("partitioned"))
							})
							Expect(cookies[2].Partitioned).To(BeTrue())
						})
					})

					Context("when JSESSIONID has MaxAge=-1 (delete cookie)", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

								// Create JSESSIONID with MaxAge=-1, which translates to "Max-Age=0" in the HTTP header (delete cookie immediately)
								deleteCookie := &http.Cookie{
									Name:     StickyCookieKey,
									Value:    "session-to-delete",
									MaxAge:   -1,
									Secure:   true,
									HttpOnly: true,
									SameSite: http.SameSiteStrictMode,
								}
								resp.Header.Add(round_tripper.CookieHeader, deleteCookie.String())
								return resp, nil
							}
						})

						It("copies MaxAge=-1 to VCAP_ID and stores maxage=-1 in VCAP_ID_META", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

							// Verify JSESSIONID has MaxAge=-1
							Expect(cookies[0].Name).To(Equal(StickyCookieKey))
							Expect(cookies[0].MaxAge).To(Equal(-1))

							// Verify VCAP_ID also has MaxAge=-1
							Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[1].MaxAge).To(Equal(-1))
							Expect(cookies[1].Value).To(SatisfyAny(
								Equal("id-1"),
								Equal("id-2")))

							// Verify VCAP_ID_META stores maxage=-1 so the delete is preserved on refresh
							expectMetaCookie(cookies[2], func(value string) {
								params, _ := url.ParseQuery(value)
								Expect(params.Get("maxage")).To(Equal("-1"))
								Expect(value).ToNot(ContainSubstring("expires="))
							})
						})
					})

					Context("when JSESSIONID has a positive MaxAge", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
								resp.Header.Add(round_tripper.CookieHeader, (&http.Cookie{
									Name: StickyCookieKey, Value: "session-value", MaxAge: 3600,
								}).String())
								return resp, nil
							}
						})

						It("copies MaxAge to VCAP_ID and stores absolute epoch in VCAP_ID_META maxage=", func() {
							before := time.Now().Unix()
							resp, err := proxyRoundTripper.RoundTrip(req)
							after := time.Now().Unix()
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

							Expect(cookies[0].Name).To(Equal(StickyCookieKey))
							Expect(cookies[0].MaxAge).To(Equal(3600))

							Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[1].MaxAge).To(Equal(3600))
							Expect(cookies[1].Value).To(SatisfyAny(Equal("id-1"), Equal("id-2")))

							// VCAP_ID_META stores the absolute expiry epoch so stale refresh can compute remaining time
							expectMetaCookie(cookies[2], func(value string) {
								Expect(value).To(ContainSubstring("maxage="))
								Expect(value).ToNot(ContainSubstring("expires="))
								params, _ := url.ParseQuery(value)
								v, err := strconv.ParseInt(params.Get("maxage"), 10, 64)
								Expect(err).ToNot(HaveOccurred())
								Expect(v).To(BeNumerically(">=", before+3600))
								Expect(v).To(BeNumerically("<=", after+3600))
							})
						})
					})

					Context("when JSESSIONID has MaxAge=0 (session cookie, Max-Age not set)", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

								// Create JSESSIONID with MaxAge=0 (Max-Age attribute is not set in HTTP header, cookie is a session cookie)
								sessionCookie := &http.Cookie{
									Name:     StickyCookieKey,
									Value:    "session-value",
									MaxAge:   0,
									Secure:   true,
									HttpOnly: true,
									SameSite: http.SameSiteStrictMode,
								}
								resp.Header.Add(round_tripper.CookieHeader, sessionCookie.String())
								return resp, nil
							}
						})

						It("sets VCAP_ID as a session cookie and VCAP_ID_META with no expiry flags", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

							// JSESSIONID has no Max-Age (zero value, not set)
							Expect(cookies[0].Name).To(Equal(StickyCookieKey))
							Expect(cookies[0].MaxAge).To(Equal(0))

							// VCAP_ID is also a session cookie — Max-Age is absent, not copied
							Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[1].MaxAge).To(Equal(0))
							Expect(cookies[1].Value).To(SatisfyAny(
								Equal("id-1"),
								Equal("id-2")))
						})

					})

					Context("when Secure Cookies are enforced in Gorouter", func() {
						BeforeEach(func() {
							cfg.SecureCookies = true
							transport.RoundTripStub = responseContainsJSESSIONIDWithExtraProperties
						})

						Context("when JSESSIONID is Secure", func() {
							BeforeEach(func() {
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
									sessionCookie.Value, _ = uuid.GenerateUUID()
									sessionCookie.Secure = true
									resp.Header.Add(round_tripper.CookieHeader, sessionCookie.String())
									return resp, nil
								}
							})

							It("sets Secure on VCAP_ID and VCAP_ID_META, and encodes secure in VCAP_ID_META", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

								Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(cookies[1].Secure).To(BeTrue())
								Expect(cookies[2].Name).To(Equal(round_tripper.VcapMetaCookieId))
								Expect(cookies[2].Secure).To(BeTrue())
								expectMetaCookie(cookies[2], func(value string) {
									Expect(value).To(ContainSubstring("secure"))
								})
							})
						})

						Context("when JSESSIONID is not Secure", func() {
							BeforeEach(func() {
								transport.RoundTripStub = responseContainsJSESSIONID
							})

							It("enforces Secure on VCAP_ID and VCAP_ID_META, and encodes secure in VCAP_ID_META", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

								Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(cookies[1].Secure).To(BeTrue())
								Expect(cookies[2].Name).To(Equal(round_tripper.VcapMetaCookieId))
								Expect(cookies[2].Secure).To(BeTrue())
								expectMetaCookie(cookies[2], func(value string) {
									Expect(value).To(ContainSubstring("secure"))
								})
							})
						})
					})

					Context("when there are multiple JSESSIONIDs on the response (CHIPS migration)", func() {
						Context("when one JSESSIONID is partitioned and one is non-partitioned with MaxAge=-1 (delete)", func() {
							BeforeEach(func() {
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

									partitionedCookie := &http.Cookie{
										Name:        StickyCookieKey,
										Value:       "new-session-value",
										Secure:      true,
										HttpOnly:    true,
										SameSite:    http.SameSiteNoneMode,
										Partitioned: true,
									}
									resp.Header.Add(round_tripper.CookieHeader, partitionedCookie.String())

									deleteCookie := &http.Cookie{
										Name:     StickyCookieKey,
										Value:    "old-session-value",
										MaxAge:   -1,
										Secure:   true,
										HttpOnly: true,
									}
									resp.Header.Add(round_tripper.CookieHeader, deleteCookie.String())

									return resp, nil
								}
							})

							It("creates a VCAP_ID + META pair for each JSESSIONID", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								// 2x JSESSIONID + 2x VCAP_ID + 2x VCAP_ID_META = 6
								Expect(cookies).To(HaveLen(6))

								// First JSESSIONID (partitioned)
								Expect(cookies[0].Name).To(Equal(StickyCookieKey))
								Expect(cookies[0].Partitioned).To(BeTrue())

								// Second JSESSIONID (delete)
								Expect(cookies[1].Name).To(Equal(StickyCookieKey))
								Expect(cookies[1].MaxAge).To(Equal(-1))

								// First VCAP_ID — partitioned, matching the first JSESSIONID
								expectVcapIdCookie(cookies[2], "id-1", "id-2")
								Expect(cookies[2].Partitioned).To(BeTrue())
								Expect(cookies[2].SameSite).To(Equal(http.SameSiteNoneMode))

								// First VCAP_ID_META — partitioned
								expectMetaCookie(cookies[3], func(value string) {
									Expect(value).To(ContainSubstring("partitioned"))
								})
								Expect(cookies[3].Partitioned).To(BeTrue())

								// Second VCAP_ID — non-partitioned, MaxAge=-1 (delete)
								expectVcapIdCookie(cookies[4], "id-1", "id-2")
								Expect(cookies[4].Partitioned).To(BeFalse())
								Expect(cookies[4].MaxAge).To(Equal(-1))

								// Second VCAP_ID_META — non-partitioned, MaxAge=-1
								expectMetaCookie(cookies[5], func(value string) {
									Expect(value).To(ContainSubstring("maxage=-1"))
								})
								Expect(cookies[5].Partitioned).To(BeFalse())
							})
						})

						Context("when two JSESSIONIDs have different SameSite values", func() {
							BeforeEach(func() {
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

									cookie1 := &http.Cookie{
										Name:     StickyCookieKey,
										Value:    "session-strict",
										SameSite: http.SameSiteStrictMode,
										HttpOnly: true,
									}
									resp.Header.Add(round_tripper.CookieHeader, cookie1.String())

									cookie2 := &http.Cookie{
										Name:     StickyCookieKey,
										Value:    "session-lax",
										SameSite: http.SameSiteLaxMode,
										HttpOnly: true,
									}
									resp.Header.Add(round_tripper.CookieHeader, cookie2.String())

									return resp, nil
								}
							})

							It("each VCAP_ID inherits the correct SameSite from its corresponding JSESSIONID", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								// 2x JSESSIONID + 2x VCAP_ID + 2x VCAP_ID_META = 6
								Expect(cookies).To(HaveLen(6))

								// First VCAP_ID inherits SameSite=Strict
								expectVcapIdCookie(cookies[2], "id-1", "id-2")
								Expect(cookies[2].SameSite).To(Equal(http.SameSiteStrictMode))

								// Second VCAP_ID inherits SameSite=Lax
								expectVcapIdCookie(cookies[4], "id-1", "id-2")
								Expect(cookies[4].SameSite).To(Equal(http.SameSiteLaxMode))
							})
						})

						Context("when SecureCookies is enforced in Gorouter", func() {
							BeforeEach(func() {
								cfg.SecureCookies = true
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

									cookie1 := &http.Cookie{
										Name:        StickyCookieKey,
										Value:       "new-session",
										Partitioned: true,
										HttpOnly:    true,
									}
									resp.Header.Add(round_tripper.CookieHeader, cookie1.String())

									cookie2 := &http.Cookie{
										Name:     StickyCookieKey,
										Value:    "old-session",
										MaxAge:   -1,
										HttpOnly: true,
									}
									resp.Header.Add(round_tripper.CookieHeader, cookie2.String())

									return resp, nil
								}
							})

							It("sets Secure on all VCAP_ID and VCAP_ID_META cookies", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								Expect(cookies).To(HaveLen(6))

								// Both VCAP_ID cookies must be Secure
								Expect(cookies[2].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(cookies[2].Secure).To(BeTrue())
								Expect(cookies[4].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(cookies[4].Secure).To(BeTrue())

								// Both VCAP_ID_META cookies must be Secure
								Expect(cookies[3].Name).To(Equal(round_tripper.VcapMetaCookieId))
								Expect(cookies[3].Secure).To(BeTrue())
								Expect(cookies[5].Name).To(Equal(round_tripper.VcapMetaCookieId))
								Expect(cookies[5].Secure).To(BeTrue())
							})
						})
					})

					Context("when a __Host- prefixed session cookie is on the response", func() {
						Context("when the response contains __Host-JSESSIONID", func() {
							BeforeEach(func() {
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

									hostCookie := &http.Cookie{
										Name:     "__Host-" + StickyCookieKey,
										Value:    "host-session-value",
										Secure:   true,
										HttpOnly: true,
										SameSite: http.SameSiteStrictMode,
									}
									resp.Header.Add(round_tripper.CookieHeader, hostCookie.String())

									return resp, nil
								}
							})

							It("recognizes the cookie and adds VCAP_ID and VCAP_ID_META", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								// __Host-JSESSIONID + VCAP_ID + VCAP_ID_META = 3
								Expect(cookies).To(HaveLen(3))

								Expect(cookies[0].Name).To(Equal("__Host-" + StickyCookieKey))

								expectVcapIdCookie(cookies[1], "id-1", "id-2")
								Expect(cookies[1].Secure).To(BeTrue())
								Expect(cookies[1].SameSite).To(Equal(http.SameSiteStrictMode))

								expectMetaCookie(cookies[2], func(value string) {
									Expect(value).To(ContainSubstring("secure"))
								})
							})
						})

						Context("when the response contains both JSESSIONID and __Host-JSESSIONID", func() {
							BeforeEach(func() {
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

									plainCookie := &http.Cookie{
										Name:     StickyCookieKey,
										Value:    "plain-session",
										HttpOnly: true,
									}
									resp.Header.Add(round_tripper.CookieHeader, plainCookie.String())

									hostCookie := &http.Cookie{
										Name:     "__Host-" + StickyCookieKey,
										Value:    "host-session",
										Secure:   true,
										HttpOnly: true,
										SameSite: http.SameSiteNoneMode,
									}
									resp.Header.Add(round_tripper.CookieHeader, hostCookie.String())

									return resp, nil
								}
							})

							It("creates a VCAP_ID + META pair for each session cookie", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								// 2x session cookie + 2x VCAP_ID + 2x VCAP_ID_META = 6
								Expect(cookies).To(HaveLen(6))

								Expect(cookies[0].Name).To(Equal(StickyCookieKey))
								Expect(cookies[1].Name).To(Equal("__Host-" + StickyCookieKey))

								// First VCAP_ID — from plain JSESSIONID (not Secure)
								expectVcapIdCookie(cookies[2], "id-1", "id-2")
								Expect(cookies[2].Secure).To(BeFalse())

								// First META
								expectMetaCookie(cookies[3], nil)

								// Second VCAP_ID — from __Host-JSESSIONID (Secure, SameSite=None)
								expectVcapIdCookie(cookies[4], "id-1", "id-2")
								Expect(cookies[4].Secure).To(BeTrue())
								Expect(cookies[4].SameSite).To(Equal(http.SameSiteNoneMode))

								// Second META
								expectMetaCookie(cookies[5], func(value string) {
									Expect(value).To(ContainSubstring("secure"))
								})
							})
						})

						Context("when SecureCookies is enforced with __Host-JSESSIONID", func() {
							BeforeEach(func() {
								cfg.SecureCookies = true
								transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
									resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}

									hostCookie := &http.Cookie{
										Name:     "__Host-" + StickyCookieKey,
										Value:    "host-session",
										HttpOnly: true,
									}
									resp.Header.Add(round_tripper.CookieHeader, hostCookie.String())

									return resp, nil
								}
							})

							It("sets Secure on VCAP_ID and VCAP_ID_META", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								cookies := resp.Cookies()
								Expect(cookies).To(HaveLen(3))

								Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(cookies[1].Secure).To(BeTrue())
								Expect(cookies[2].Name).To(Equal(round_tripper.VcapMetaCookieId))
								Expect(cookies[2].Secure).To(BeTrue())
							})
						})
					})

					Context("when there is a JSESSIONID and a VCAP_ID on the response", func() {
						BeforeEach(func() {
							transport.RoundTripStub = responseContainsJSESSIONIDAndVCAPID
						})

						It("leaves VCAP_ID alone and does not overwrite it", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(2))
							Expect(cookies[0].Raw).To(Equal(sessionCookie.String()))
							Expect(cookies[1].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[1].Value).To(Equal("vcap-id-property-already-on-the-response"))
						})
					})

					Context("when there is only a VCAP_ID set on the response", func() {
						BeforeEach(func() {
							transport.RoundTripStub = responseContainsVCAPID
						})

						It("leaves VCAP_ID alone and does not overwrite it", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(1))
							Expect(cookies[0].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(cookies[0].Value).To(Equal("vcap-id-property-already-on-the-response"))
						})
					})

				})

				Context("Existing Session Scenario", func() {
					Context("when the request contains a __Host- prefixed session cookie", func() {
						JustBeforeEach(func() {
							// First request: app responds with __Host-JSESSIONID
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
								hostCookie := &http.Cookie{
									Name:     "__Host-" + StickyCookieKey,
									Value:    "host-session-value",
									Secure:   true,
									HttpOnly: true,
								}
								resp.Header.Add(round_tripper.CookieHeader, hostCookie.String())
								return resp, nil
							}
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							firstCookies := resp.Cookies()
							Expect(firstCookies).To(HaveLen(3)) // __Host-JSESSIONID + VCAP_ID + VCAP_ID_META
							for _, cookie := range firstCookies {
								req.AddCookie(cookie)
							}
						})

						It("recognizes the __Host- prefixed cookie for sticky routing and refreshes VCAP_ID", func() {
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
								hostCookie := &http.Cookie{
									Name:     "__Host-" + StickyCookieKey,
									Value:    "host-session-value-refreshed",
									Secure:   true,
									HttpOnly: true,
								}
								resp.Header.Add(round_tripper.CookieHeader, hostCookie.String())
								return resp, nil
							}

							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							cookies := resp.Cookies()
							Expect(cookies).To(HaveLen(3)) // __Host-JSESSIONID + VCAP_ID + VCAP_ID_META

							Expect(cookies[0].Name).To(Equal("__Host-" + StickyCookieKey))
							expectVcapIdCookie(cookies[1])
							expectMetaCookie(cookies[2], nil)
						})
					})

					Context("when the sticky endpoint still exists (no stale session)", func() {
						var firstCookies []*http.Cookie
						JustBeforeEach(func() {
							transport.RoundTripStub = responseContainsJSESSIONID
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							firstCookies = resp.Cookies()
							Expect(firstCookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META
							for _, cookie := range firstCookies {
								req.AddCookie(cookie)
							}
						})

						Context("when there is a JSESSIONID set on the response", func() {
							JustBeforeEach(func() {
								transport.RoundTripStub = responseContainsJSESSIONID
							})

							It("selects the previous backend and refreshes VCAP_ID and VCAP_ID_META", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								newCookies := resp.Cookies()
								Expect(newCookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

								// JSESSIONID is the session cookie set by the backend
								Expect(newCookies[0].Name).To(Equal(StickyCookieKey))

								// VCAP_ID still points to the same backend instance (previous backend selected)
								expectVcapIdCookie(newCookies[1], firstCookies[1].Value)

								// VCAP_ID_META is refreshed alongside VCAP_ID
								expectMetaCookie(newCookies[2], nil)
							})

							Context("when JSESSIONID on the response is non-partitioned with extra properties", func() {
								JustBeforeEach(func() {
									transport.RoundTripStub = responseContainsJSESSIONIDWithExtraProperties
								})

								It("sets VCAP_ID and VCAP_ID_META on the response with the same new cookie attributes", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META
									Expect(newCookies[0].Raw).To(Equal(sessionCookie.String()))
									Expect(newCookies[0].Partitioned).To(BeFalse())

									Expect(newCookies[1].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[1].Value).To(Equal(firstCookies[1].Value)) // still pointing to the same app
									Expect(sessionCookie.String()).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))
									Expect(newCookies[1].Raw).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))
									Expect(newCookies[1].Partitioned).To(BeFalse())

									expectMetaCookie(newCookies[2], nil)
									Expect(newCookies[2].Raw).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))
									Expect(newCookies[2].Partitioned).To(BeFalse())
								})
							})

							Context("when JSESSIONID on the response is partitioned", func() {
								JustBeforeEach(func() {
									transport.RoundTripStub = responseContainsPartitionedJSESSIONID
								})

								It("sets VCAP_ID and VCAP_ID_META with the same new cookie attributes", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

									// JSESSIONID is partitioned
									Expect(newCookies[0].Raw).To(Equal(sessionCookie.String()))
									Expect(newCookies[0].Partitioned).To(BeTrue())

									// VCAP_ID is partitioned and points to same instance
									Expect(newCookies[1].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[1].Value).To(Equal(firstCookies[1].Value)) // still pointing to the same app
									Expect(newCookies[1].Partitioned).To(BeTrue())

									// VCAP_ID_META carries partitioned and is itself partitioned
									expectMetaCookie(newCookies[2], func(value string) {
										Expect(value).To(ContainSubstring("partitioned"))
									})
									Expect(newCookies[2].Partitioned).To(BeTrue())
								})
							})

						})

						Context("when no cookies are set on the response", func() {
							JustBeforeEach(func() {
								transport.RoundTripStub = responseContainsNoCookies
							})

							It("does not set cookies on the response", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								Expect(resp.Cookies()).To(HaveLen(0))
							})
						})

					})

					Context("when the sticky endpoint no longer exists (stale session)", func() {

						// initialStub is the RoundTrip stub used for the first (setup) request.
						// Sub-contexts that need direct cookie injection set it to nil and add
						// cookies themselves in an inner JustBeforeEach.
						var initialStub func(*http.Request) (*http.Response, error)

						BeforeEach(func() {
							initialStub = responseContainsJSESSIONID // default: plain non-partitioned session cookie
						})

						JustBeforeEach(func() {
							if initialStub != nil {
								transport.RoundTripStub = initialStub
								firstResp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())
								for _, c := range firstResp.Cookies() {
									req.AddCookie(c)
								}
							}
							routePool.Remove(endpoint1)
							routePool.Remove(endpoint2)
							newEndpoint := route.NewEndpoint(&route.EndpointOpts{PrivateInstanceId: "id-new-backend"})
							routePool.Put(newEndpoint)
							transport.RoundTripStub = responseContainsNoCookies
						})

						Context("when the new backend sets a JSESSIONID on the response", func() {
							JustBeforeEach(func() {
								transport.RoundTripStub = responseContainsJSESSIONIDWithExtraProperties
							})

							It("selects the new backend, updates VCAP_ID with JSESSIONID attributes, and updates VCAP_ID_META", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								newCookies := resp.Cookies()
								Expect(newCookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

								Expect(newCookies[0].Raw).To(Equal(sessionCookie.String()))

								Expect(newCookies[1].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(newCookies[1].Value).To(Equal("id-new-backend"))
								Expect(newCookies[1].Raw).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))

								expectMetaCookie(newCookies[2], func(value string) {
									Expect(value).To(ContainSubstring("secure"))
									Expect(value).To(ContainSubstring("samesite=strict"))
									params, _ := url.ParseQuery(value)
									v, err := strconv.ParseInt(params.Get("expires"), 10, 64)
									Expect(err).ToNot(HaveOccurred())
									// Expires=Wed, 01 Jan 2020 01:00:00 GMT → Unix timestamp 1577840400
									Expect(v).To(Equal(int64(1577840400)))
								})
								Expect(newCookies[2].Raw).To(ContainSubstring("Expires=Wed, 01 Jan 2020 01:00:00 GMT; HttpOnly; Secure; SameSite=Strict"))
								Expect(newCookies[2].Partitioned).To(BeFalse())
							})
						})

						Context("when the new backend does not set JSESSIONID on the response", func() {
							Context("when the session is partitioned (client has partitioned JSESSIONID + VCAP_ID + VCAP_ID_META)", func() {
								JustBeforeEach(func() {
									// Simulate a client that previously received a partitioned session:
									// replace the non-partitioned META cookie (set by the outer JustBeforeEach)
									// with a partitioned one. Rebuild the Cookie header so there is exactly one META.
									var kept []*http.Cookie
									for _, c := range req.Cookies() {
										if c.Name != round_tripper.VcapMetaCookieId {
											kept = append(kept, c)
										}
									}
									req.Header.Del("Cookie")
									for _, c := range kept {
										req.AddCookie(c)
									}
									req.AddCookie(&http.Cookie{
										Name:  round_tripper.VcapMetaCookieId,
										Value: "partitioned",
									})
								})

								It("updates VCAP_ID as partitioned and re-sets VCAP_ID_META, without JSESSIONID in the response", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1)) // VCAP_ID

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Value).To(Equal("id-new-backend"))
									Expect(newCookies[0].Partitioned).To(BeTrue())
								})
							})

							Context("when VCAP_ID_META has Secure=true and SameSite=Strict", func() {
								BeforeEach(func() {
									initialStub = responseContainsJSESSIONIDWithExtraProperties
								})

								It("restores Expires verbatim and preserved attributes on refreshed VCAP_ID", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1)) // VCAP_ID

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Value).To(Equal("id-new-backend"))
									Expect(newCookies[0].Secure).To(BeTrue())
									Expect(newCookies[0].SameSite).To(Equal(http.SameSiteStrictMode))
									// Expires is restored verbatim from the stored timestamp
									Expect(newCookies[0].Expires).To(Equal(time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC)))
									Expect(newCookies[0].MaxAge).To(Equal(0))
								})
							})

							Context("when VCAP_ID_META is a session cookie (no expiry)", func() {
								// initialStub defaults to responseContainsJSESSIONID — no override needed

								It("refreshes VCAP_ID as a session cookie (no MaxAge, no Expires)", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1)) // VCAP_ID

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].MaxAge).To(Equal(0))
									Expect(newCookies[0].Expires).To(Equal(time.Time{}))
								})
							})

							Context("when VCAP_ID_META has a positive MaxAge", func() {
								BeforeEach(func() {
									initialStub = func(req *http.Request) (*http.Response, error) {
										resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
										resp.Header.Add(round_tripper.CookieHeader, (&http.Cookie{
											Name: StickyCookieKey, Value: "s", MaxAge: 3600,
										}).String())
										return resp, nil
									}
								})

								It("restores VCAP_ID with remaining MaxAge (no Expires)", func() {
									before := time.Now().Unix()
									resp, err := proxyRoundTripper.RoundTrip(req)
									after := time.Now().Unix()
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1)) // VCAP_ID

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Expires).To(Equal(time.Time{}))
									Expect(newCookies[0].MaxAge).To(BeNumerically(">=", int(before+3600-after)))
									Expect(newCookies[0].MaxAge).To(BeNumerically("<=", 3600))
								})
							})

							Context("when VCAP_ID_META has an already-elapsed MaxAge", func() {
								BeforeEach(func() {
									initialStub = nil // skip first request; inject cookies directly below
								})

								JustBeforeEach(func() {
									// Construct META cookie directly with a maxage epoch in the past,
									// bypassing the first-request path to avoid timing-sensitive sleeps.
									pastEpoch := time.Now().Unix() - 10
									req.AddCookie(&http.Cookie{Name: StickyCookieKey, Value: "s"})
									req.AddCookie(&http.Cookie{Name: round_tripper.VcapCookieId, Value: endpoint1.PrivateInstanceId})
									req.AddCookie(&http.Cookie{
										Name:  round_tripper.VcapMetaCookieId,
										Value: "maxage=" + strconv.FormatInt(pastEpoch, 10),
									})
								})

								It("sets MaxAge=-1 on refreshed VCAP_ID (session has expired)", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1)) // VCAP_ID

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].MaxAge).To(Equal(-1))
									Expect(newCookies[0].Expires).To(Equal(time.Time{}))
								})
							})

							Context("when VCAP_ID_META has both MaxAge and Expires", func() {
								BeforeEach(func() {
									initialStub = func(req *http.Request) (*http.Response, error) {
										resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
										resp.Header.Add(round_tripper.CookieHeader, (&http.Cookie{
											Name:    StickyCookieKey,
											Value:   "s",
											MaxAge:  3600,
											Expires: time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC),
										}).String())
										return resp, nil
									}
								})

								It("restores both remaining MaxAge and Expires verbatim on refreshed VCAP_ID", func() {
									before := time.Now().Unix()
									resp, err := proxyRoundTripper.RoundTrip(req)
									after := time.Now().Unix()
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1)) // VCAP_ID

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].MaxAge).To(BeNumerically(">=", int(before+3600-after)))
									Expect(newCookies[0].MaxAge).To(BeNumerically("<=", 3600))
									Expect(newCookies[0].Expires).To(Equal(time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC)))
								})
							})

							Context("when VCAP_ID_META is absent (legacy client without meta cookie)", func() {
								BeforeEach(func() {
									initialStub = nil // skip first request; inject cookies directly below
								})

								JustBeforeEach(func() {
									req.AddCookie(&http.Cookie{Name: StickyCookieKey, Value: "s"})
									req.AddCookie(&http.Cookie{Name: round_tripper.VcapCookieId, Value: endpoint1.PrivateInstanceId})
								})

								It("updates VCAP_ID with defaults (no attribute preservation possible)", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1)) // VCAP_ID only — no META to read or write

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Value).To(Equal("id-new-backend"))
									Expect(newCookies[0].Partitioned).To(BeFalse())
								})

								It("logs an info message that VCAP_ID_META was not found", func() {
									_, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())
									Eventually(logger).Should(gbytes.Say(`"log_level":1.*vcap-id-meta-cookie-not-found`))
								})
							})

							Context("when VCAP_ID_META has an unparseable value", func() {
								BeforeEach(func() {
									initialStub = nil // skip first request; inject cookies directly below
								})

								JustBeforeEach(func() {
									req.AddCookie(&http.Cookie{Name: StickyCookieKey, Value: "s"})
									req.AddCookie(&http.Cookie{Name: round_tripper.VcapCookieId, Value: endpoint1.PrivateInstanceId})
									req.AddCookie(&http.Cookie{
										Name:  round_tripper.VcapMetaCookieId,
										Value: "partitioned=%ZZ", // invalid percent-encoding causes url.ParseQuery to return an error
									})
								})

								It("logs an error that VCAP_ID_META could not be parsed", func() {
									_, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())
									Eventually(logger).Should(gbytes.Say(`"log_level":3.*vcap-id-meta-cookie-parse-error`))
								})

								It("falls back to default cookie attributes on the refreshed VCAP_ID", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1))

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Value).To(Equal("id-new-backend"))
									Expect(newCookies[0].Secure).To(BeFalse())
									Expect(newCookies[0].SameSite).To(BeZero())
									Expect(newCookies[0].Partitioned).To(BeFalse())
									Expect(newCookies[0].MaxAge).To(Equal(0))
									Expect(newCookies[0].Expires).To(Equal(time.Time{}))
								})
							})

							Context("when VCAP_ID_META has an unparseable maxage", func() {
								BeforeEach(func() {
									initialStub = nil
								})

								JustBeforeEach(func() {
									req.AddCookie(&http.Cookie{Name: StickyCookieKey, Value: "s"})
									req.AddCookie(&http.Cookie{Name: round_tripper.VcapCookieId, Value: endpoint1.PrivateInstanceId})
									req.AddCookie(&http.Cookie{
										Name:  round_tripper.VcapMetaCookieId,
										Value: "secure&samesite=strict&partitioned&maxage=notanumber&expires=1577840400",
									})
								})

								It("logs an error that VCAP_ID_META could not be parsed", func() {
									_, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())
									Eventually(logger).Should(gbytes.Say(`"log_level":3.*vcap-id-meta-cookie-parse-error.*maxage`))
								})

								It("preserves parseable attributes but falls back to default MaxAge", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1))

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Value).To(Equal("id-new-backend"))
									Expect(newCookies[0].Secure).To(BeTrue())
									Expect(newCookies[0].SameSite).To(Equal(http.SameSiteStrictMode))
									Expect(newCookies[0].Partitioned).To(BeTrue())
									Expect(newCookies[0].MaxAge).To(Equal(0))
									Expect(newCookies[0].Expires).To(Equal(time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC)))
								})
							})

							Context("when VCAP_ID_META has an unparseable expires", func() {
								BeforeEach(func() {
									initialStub = nil
								})

								JustBeforeEach(func() {
									req.AddCookie(&http.Cookie{Name: StickyCookieKey, Value: "s"})
									req.AddCookie(&http.Cookie{Name: round_tripper.VcapCookieId, Value: endpoint1.PrivateInstanceId})
									req.AddCookie(&http.Cookie{
										Name:  round_tripper.VcapMetaCookieId,
										Value: "secure&samesite=lax&expires=notanumber",
									})
								})

								It("logs an error that VCAP_ID_META could not be parsed", func() {
									_, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())
									Eventually(logger).Should(gbytes.Say(`"log_level":3.*vcap-id-meta-cookie-parse-error.*expires`))
								})

								It("preserves parseable attributes but falls back to default Expires", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1))

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Value).To(Equal("id-new-backend"))
									Expect(newCookies[0].Secure).To(BeTrue())
									Expect(newCookies[0].SameSite).To(Equal(http.SameSiteLaxMode))
									Expect(newCookies[0].Partitioned).To(BeFalse())
									Expect(newCookies[0].MaxAge).To(Equal(0))
									Expect(newCookies[0].Expires).To(Equal(time.Time{}))
								})
							})

							Context("when VCAP_ID_META has an unknown samesite value", func() {
								BeforeEach(func() {
									initialStub = nil
								})

								JustBeforeEach(func() {
									req.AddCookie(&http.Cookie{Name: StickyCookieKey, Value: "s"})
									req.AddCookie(&http.Cookie{Name: round_tripper.VcapCookieId, Value: endpoint1.PrivateInstanceId})
									req.AddCookie(&http.Cookie{
										Name:  round_tripper.VcapMetaCookieId,
										Value: "secure&samesite=bogus",
									})
								})

								It("logs an error that VCAP_ID_META could not be parsed", func() {
									_, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())
									Eventually(logger).Should(gbytes.Say(`"log_level":3.*vcap-id-meta-cookie-parse-error.*samesite`))
								})

								It("preserves parseable attributes but falls back to default SameSite", func() {
									resp, err := proxyRoundTripper.RoundTrip(req)
									Expect(err).ToNot(HaveOccurred())

									newCookies := resp.Cookies()
									Expect(newCookies).To(HaveLen(1))

									Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
									Expect(newCookies[0].Value).To(Equal("id-new-backend"))
									Expect(newCookies[0].Secure).To(BeTrue())
									Expect(newCookies[0].SameSite).To(BeZero())
								})
							})
						})

						Context("when Secure Cookies are enforced in Gorouter", func() {
							BeforeEach(func() {
								cfg.SecureCookies = true
							})

							It("enforces Secure on the refreshed VCAP_ID even though JSESSIONID was not Secure", func() {
								resp, err := proxyRoundTripper.RoundTrip(req)
								Expect(err).ToNot(HaveOccurred())

								newCookies := resp.Cookies()
								Expect(newCookies).To(HaveLen(1)) // VCAP_ID only

								Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
								Expect(newCookies[0].Value).To(Equal("id-new-backend"))
								Expect(newCookies[0].Secure).To(BeTrue())
							})
						})

					})

				})

				Context("when route service headers are present on the request", func() {
					JustBeforeEach(func() {
						// Simulate an existing session: first request populates JSESSIONID + VCAP_ID on req
						transport.RoundTripStub = responseContainsJSESSIONID
						firstResp, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).ToNot(HaveOccurred())
						for _, c := range firstResp.Cookies() {
							req.AddCookie(c)
						}
						// Swap endpoint so the request hits a new backend
						routePool.Remove(endpoint1)
						routePool.Remove(endpoint2)
						newEndpoint := route.NewEndpoint(&route.EndpointOpts{PrivateInstanceId: "id-5"})
						routePool.Put(newEndpoint)
						// Set route service headers — gorouter must not set VCAP_ID for route service requests
						req.Header.Set(routeservice.HeaderKeySignature, "foo")
						req.Header.Set(routeservice.HeaderKeyForwardedURL, "bar")
					})

					Context("when the backend sets a JSESSIONID on the response", func() {
						JustBeforeEach(func() {
							transport.RoundTripStub = responseContainsJSESSIONID
						})

						It("does not set VCAP_ID", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							newCookies := resp.Cookies()
							Expect(newCookies).To(HaveLen(1))
							Expect(newCookies[0].Raw).To(Equal(sessionCookie.String()))
						})
					})

					Context("when the backend sets no cookies on the response", func() {
						JustBeforeEach(func() {
							transport.RoundTripStub = responseContainsNoCookies
						})

						It("does not set VCAP_ID", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							Expect(resp.Cookies()).To(HaveLen(0))
						})
					})

					Context("when the backend sets a VCAP_ID on the response", func() {
						JustBeforeEach(func() {
							transport.RoundTripStub = responseContainsVCAPID
						})

						It("leaves VCAP_ID alone and does not overwrite it", func() {
							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							newCookies := resp.Cookies()
							Expect(newCookies).To(HaveLen(1))
							Expect(newCookies[0].Name).To(Equal(round_tripper.VcapCookieId))
							Expect(newCookies[0].Value).To(Equal("vcap-id-property-already-on-the-response"))
						})
					})
				})
			})

			Context("when load-balancing strategy is set to hash-based routing", func() {
				JustBeforeEach(func() {
					for i := 1; i <= 3; i++ {
						endpoint = route.NewEndpoint(&route.EndpointOpts{
							AppId:                  fmt.Sprintf("appID%d", i),
							Host:                   fmt.Sprintf("%d.%d.%d.%d", i, i, i, i),
							Port:                   9090,
							PrivateInstanceId:      fmt.Sprintf("instanceID%d", i),
							PrivateInstanceIndex:   fmt.Sprintf("%d", i),
							AvailabilityZone:       AZ,
							LoadBalancingAlgorithm: config.LOAD_BALANCE_HB,
							HashHeaderName:         "X-Hash",
						})

						_ = routePool.Put(endpoint)
						Expect(routePool.HashLookupTable).ToNot(BeNil())

					}
				})

				It("routes requests with same hash header value to the same endpoint", func() {
					req.Header.Set("X-Hash", "value")
					reqInfo, err := handlers.ContextRequestInfo(req)
					Expect(err).ToNot(HaveOccurred())
					reqInfo.RoutePool = routePool

					var selectedEndpoints []*route.Endpoint

					// Make multiple requests with the same hash value
					for i := 0; i < 5; i++ {
						_, err = proxyRoundTripper.RoundTrip(req)
						Expect(err).NotTo(HaveOccurred())
						selectedEndpoints = append(selectedEndpoints, reqInfo.RouteEndpoint)
					}

					// All requests should go to the same endpoint
					firstEndpoint := selectedEndpoints[0]
					for _, ep := range selectedEndpoints[1:] {
						Expect(ep.PrivateInstanceId).To(Equal(firstEndpoint.PrivateInstanceId))
					}
				})

				It("routes requests with different hash header values to potentially different endpoints", func() {
					reqInfo, err := handlers.ContextRequestInfo(req)
					Expect(err).ToNot(HaveOccurred())
					reqInfo.RoutePool = routePool

					endpointDistribution := make(map[string]int)

					// Make requests with different hash values
					for i := 0; i < 10; i++ {
						req.Header.Set("X-Hash", fmt.Sprintf("value-%d", i))
						_, err = proxyRoundTripper.RoundTrip(req)
						Expect(err).NotTo(HaveOccurred())
						endpointDistribution[reqInfo.RouteEndpoint.PrivateInstanceId]++
					}

					// Should distribute across multiple endpoints (not all to one)
					Expect(len(endpointDistribution)).To(BeNumerically(">", 1))
				})

				It("falls back to default load balancing algorithm when hash header is missing", func() {
					reqInfo, err := handlers.ContextRequestInfo(req)
					Expect(err).ToNot(HaveOccurred())

					reqInfo.RoutePool = routePool

					_, err = proxyRoundTripper.RoundTrip(req)
					Expect(err).NotTo(HaveOccurred())

					infoLogs := logger.Lines(zap.InfoLevel)
					count := 0
					for i := 0; i < len(infoLogs); i++ {
						if strings.Contains(infoLogs[i], "hash-based-routing-header-value-not-found") {
							count++
						}
					}
					Expect(count).To(Equal(1))
					// Verify it still selects an endpoint
					Expect(reqInfo.RouteEndpoint).ToNot(BeNil())
				})

				Context("when sticky session cookies (JSESSIONID and VCAP_ID) are on the request", func() {
					var (
						sessionCookie *http.Cookie
						cookies       []*http.Cookie
					)

					JustBeforeEach(func() {
						sessionCookie = &http.Cookie{
							Name: StickyCookieKey, //JSESSIONID
						}
						transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
							resp := &http.Response{StatusCode: http.StatusTeapot, Header: make(map[string][]string)}
							//Attach the same JSESSIONID on to the response if it exists on the request

							if len(req.Cookies()) > 0 {
								for _, cookie := range req.Cookies() {
									if cookie.Name == StickyCookieKey {
										resp.Header.Add(round_tripper.CookieHeader, cookie.String())
										return resp, nil
									}
								}
							}

							sessionCookie.Value, _ = uuid.GenerateUUID()
							resp.Header.Add(round_tripper.CookieHeader, sessionCookie.String())
							return resp, nil
						}
						resp, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).ToNot(HaveOccurred())

						cookies = resp.Cookies()
						Expect(cookies).To(HaveLen(3)) // JSESSIONID + VCAP_ID + VCAP_ID_META

					})

					It("will always route to the instance specified with the __VCAP_ID__ cookie", func() {

						// Generate 20 random values for the hash header, so chance that all go to instanceID1
						// by accident is 0.33^20
						for i := 0; i < 20; i++ {
							randomStr := make([]byte, 8)
							for j := range randomStr {
								randomStr[j] = byte('a' + rand.Intn(26))
							}

							req.Header.Set("X-Hash", string(randomStr))
							reqInfo, err := handlers.ContextRequestInfo(req)
							req.AddCookie(&http.Cookie{Name: round_tripper.VcapCookieId, Value: "instanceID1"})
							req.AddCookie(&http.Cookie{Name: StickyCookieKey, Value: "abc"})

							Expect(err).ToNot(HaveOccurred())
							reqInfo.RoutePool = routePool

							resp, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).ToNot(HaveOccurred())

							new_cookies := resp.Cookies()
							Expect(new_cookies).To(HaveLen(3))

							for _, cookie := range new_cookies {
								Expect(cookie.Name).To(SatisfyAny(
									Equal(StickyCookieKey),
									Equal(round_tripper.VcapCookieId),
									Equal(round_tripper.VcapMetaCookieId),
								))
								if cookie.Name == StickyCookieKey {
									Expect(cookie.Value).To(Equal("abc"))
								} else if cookie.Name == round_tripper.VcapCookieId {
									Expect(cookie.Value).To(Equal("instanceID1"))
								}
							}

						}
					})
				})
			})

			Context("when endpoint timeout is not 0", func() {
				var reqCh chan *http.Request
				BeforeEach(func() {
					cfg.EndpointTimeout = 10 * time.Millisecond
					reqCh = make(chan *http.Request, 1)

					transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
						reqCh <- req
						return &http.Response{}, nil
					}
				})

				It("sets a timeout on the request context", func() {
					proxyRoundTripper.RoundTrip(req)
					var request *http.Request
					Eventually(reqCh).Should(Receive(&request))

					_, deadlineSet := request.Context().Deadline()
					Expect(deadlineSet).To(BeTrue())
					Eventually(func() string {
						err := request.Context().Err()
						if err != nil {
							return err.Error()
						}
						return ""
					}).Should(ContainSubstring("deadline exceeded"))
				})

				Context("when the round trip errors the deadline is cancelled", func() {
					BeforeEach(func() {
						transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
							reqCh <- req
							return &http.Response{}, errors.New("boom!")
						}
					})

					It("sets a timeout on the request context", func() {
						_, err := proxyRoundTripper.RoundTrip(req)
						Expect(err).To(HaveOccurred())
						var request *http.Request
						Eventually(reqCh).Should(Receive(&request))

						err = request.Context().Err()
						Expect(err).NotTo(BeNil())
						Expect(err.Error()).To(ContainSubstring("canceled"))
					})
				})

				Context("when route service url is not nil", func() {
					var routeServiceURL *url.URL
					BeforeEach(func() {
						var err error
						routeServiceURL, err = url.Parse("https://foo.com")
						Expect(err).ToNot(HaveOccurred())
						reqInfo.RouteServiceURL = routeServiceURL
						transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
							reqCh <- req
							Expect(req.Host).To(Equal(routeServiceURL.Host))
							Expect(req.URL).To(Equal(routeServiceURL))
							return nil, nil
						}
					})

					It("sets a timeout on the request context", func() {
						proxyRoundTripper.RoundTrip(req)
						var request *http.Request
						Eventually(reqCh).Should(Receive(&request))

						_, deadlineSet := request.Context().Deadline()
						Expect(deadlineSet).To(BeTrue())
						Eventually(func() string {
							err := request.Context().Err()
							if err != nil {
								return err.Error()
							}
							return ""
						}).Should(ContainSubstring("deadline exceeded"))
					})

					Context("when the round trip errors the deadline is cancelled", func() {
						BeforeEach(func() {
							transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
								reqCh <- req
								Expect(req.Host).To(Equal(routeServiceURL.Host))
								Expect(req.URL).To(Equal(routeServiceURL))
								return &http.Response{}, errors.New("boom!")
							}
						})

						It("sets a timeout on the request context", func() {
							_, err := proxyRoundTripper.RoundTrip(req)
							Expect(err).To(HaveOccurred())
							var request *http.Request
							Eventually(reqCh).Should(Receive(&request))

							err = request.Context().Err()
							Expect(err).NotTo(BeNil())
							Expect(err.Error()).To(ContainSubstring("canceled"))
						})
					})

				})
				Context("when http/1 endpoint timeout is not 0", func() {
					BeforeEach(func() {
						cfg.Http1EndpointTimeout = 20 * time.Millisecond
						transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
							reqCh <- req
							return &http.Response{}, nil
						}
					})
					It("sets a http/1 timeout on the request context", func() {
						before := time.Now()
						proxyRoundTripper.RoundTrip(req)
						var request *http.Request
						Eventually(reqCh).Should(Receive(&request))

						deadLine, deadlineSet := request.Context().Deadline()
						Expect(deadlineSet).To(BeTrue())
						Expect(deadLine).To(BeTemporally("~", before.Add(20*time.Millisecond), 20*time.Millisecond))
						Eventually(func() string {
							err := request.Context().Err()
							if err != nil {
								return err.Error()
							}
							return ""
						}).Should(ContainSubstring("deadline exceeded"))
					})
				})
				Context("when http/2 endpoint timeout is not 0", func() {
					BeforeEach(func() {
						cfg.Http2EndpointTimeout = 15 * time.Millisecond
						transport.RoundTripStub = func(req *http.Request) (*http.Response, error) {
							reqCh <- req
							return &http.Response{}, nil
						}
					})
					It("sets a http/2 timeout on the request context", func() {
						before := time.Now()
						proxyRoundTripper.RoundTrip(req)
						var request *http.Request
						Eventually(reqCh).Should(Receive(&request))

						deadLine, deadlineSet := request.Context().Deadline()
						Expect(deadlineSet).To(BeTrue())
						Expect(deadLine).To(BeTemporally("~", before.Add(15*time.Millisecond), 15*time.Millisecond))
						Eventually(func() string {
							err := request.Context().Err()
							if err != nil {
								return err.Error()
							}
							return ""
						}).Should(ContainSubstring("deadline exceeded"))
					})
				})
			})
			Context("CancelRequest", func() {
				It("can cancel requests", func() {
					reqInfo.RouteEndpoint = endpoint
					proxyRoundTripper.CancelRequest(req)
					Expect(transport.CancelRequestCallCount()).To(Equal(1))
					Expect(transport.CancelRequestArgsForCall(0)).To(Equal(req))
				})
			})
			Context("when response headers are limited in count", func() {
				// Note: we can only test the header count as the limit on header bytes is
				// implemented in the http.Transport which we fake for these tests.
				BeforeEach(func() {
					cfg.MaxResponseHeaders = 20
				})
				It("returns an error when the response exceeds it", func() {
					transport.RoundTripStub = func(r *http.Request) (*http.Response, error) {
						header := http.Header{}
						for i := 0; i < 21; i++ {
							header[fmt.Sprintf("header-%d", i)] = []string{"foobar"}
						}

						return &http.Response{
							StatusCode: http.StatusTeapot,
							Header:     header,
						}, nil
					}

					_, err := proxyRoundTripper.RoundTrip(req)

					Expect(err).To(HaveOccurred())
					Expect(err).To(Equal(round_tripper.TooManyResponseHeaders))
				})
				It("doesn't return an error when the response does not exceed it", func() {
					transport.RoundTripStub = func(r *http.Request) (*http.Response, error) {
						header := http.Header{}
						for i := 0; i < 10; i++ {
							header[fmt.Sprintf("header-%d", i)] = []string{"foobar"}
						}

						return &http.Response{
							StatusCode: http.StatusTeapot,
							Header:     header,
						}, nil
					}

					res, err := proxyRoundTripper.RoundTrip(req)

					Expect(err).NotTo(HaveOccurred())
					Expect(res.StatusCode).To(Equal(http.StatusTeapot))
				})
			})
		})
	})
})
