package handlers_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/accesslog/fakes"
	"code.cloudfoundry.org/gorouter/handlers"
	metrics_fakes "code.cloudfoundry.org/gorouter/metrics/fakes"
	"code.cloudfoundry.org/gorouter/proxy/utils"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("AccessLog", func() {
	var (
		handler *negroni.Negroni

		resp http.ResponseWriter
		req  *http.Request

		logger            *test_util.TestLogger
		accessLogger      *fakes.FakeAccessLogger
		fakeReporter      *metrics_fakes.FakeMetricReporter
		extraHeadersToLog []string

		nextCalled bool

		reqChan chan *http.Request
	)
	testEndpoint := route.NewEndpoint(&route.EndpointOpts{
		Host: "host",
		Port: 1234,
	})
	testHeaders := http.Header{
		"Foo":               []string{"foobar"},
		"X-Forwarded-For":   []string{"1.2.3.4"},
		"X-Forwarded-Proto": []string{"https"},
	}

	nextHandler := negroni.HandlerFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
		_, err := io.ReadAll(req.Body)
		Expect(err).NotTo(HaveOccurred())

		rw.WriteHeader(http.StatusTeapot)
		rw.Write([]byte("I'm a little teapot, short and stout."))

		reqInfo, err := handlers.ContextRequestInfo(req)
		if err == nil {
			reqInfo.RouteEndpoint = testEndpoint
			timeNow := time.Now()
			reqInfo.ReceivedAt = timeNow.Add(-1 * time.Millisecond)
			reqInfo.AppRequestStartedAt = timeNow.Add(1 * time.Millisecond)
			reqInfo.AppRequestFinishedAt = timeNow.Add(2 * time.Millisecond)
		}

		if next != nil {
			next(rw, req)
		}

		reqChan <- req
		nextCalled = true
	})

	testProxyWriterHandler := func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
		proxyWriter := utils.NewProxyResponseWriter(rw, logger.Logger)
		next(proxyWriter, req)
	}

	BeforeEach(func() {
		body := bytes.NewBufferString("What are you?")
		req = test_util.NewRequest("GET", "example.com", "/", body)
		resp = httptest.NewRecorder()

		extraHeadersToLog = []string{}

		accessLogger = &fakes.FakeAccessLogger{}
		fakeReporter = new(metrics_fakes.FakeMetricReporter)

		logger = test_util.NewTestLogger("test")

		handler = negroni.New()
		handler.Use(handlers.NewRequestInfo())
		handler.Use(handlers.NewProxyWriter(logger.Logger))
		handler.Use(handlers.NewAccessLog(accessLogger, extraHeadersToLog, nil, logger.Logger))
		handler.Use(handlers.NewReporter(fakeReporter, logger.Logger))
		handler.Use(nextHandler)

		reqChan = make(chan *http.Request, 1)

		nextCalled = false
	})

	AfterEach(func() {
		Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
		close(reqChan)
	})

	It("logs the access log record after all subsequent handlers have run", func() {
		handler.ServeHTTP(resp, req)

		Expect(accessLogger.LogCallCount()).To(Equal(1))

		alr := accessLogger.LogArgsForCall(0)

		Expect(alr.ReceivedAt).ToNot(BeZero())
		Expect(alr.Request.Header).To(Equal(req.Header))
		Expect(alr.Request.Method).To(Equal(req.Method))
		Expect(alr.Request.URL).To(Equal(req.URL))
		Expect(alr.Request.RemoteAddr).To(Equal(req.RemoteAddr))
		Expect(alr.ExtraHeadersToLog).To(Equal(extraHeadersToLog))
		Expect(alr.RequestBytesReceived).To(Equal(13))
		Expect(alr.GorouterTime).ToNot(BeZero())
		Expect(alr.BodyBytesSent).To(Equal(37))
		Expect(alr.StatusCode).To(Equal(http.StatusTeapot))
		Expect(alr.RouteEndpoint).To(Equal(testEndpoint))
		Expect(alr.HeadersOverride).To(BeNil())
		Expect(alr.RouterError).To(BeEmpty())
	})

	Context("when duplicate extraFields are set", func() {
		BeforeEach(func() {
			handler = negroni.New()
			handler.Use(handlers.NewRequestInfo())
			handler.Use(handlers.NewProxyWriter(logger.Logger))
			handler.Use(handlers.NewAccessLog(accessLogger, extraHeadersToLog, []string{"local_address", "local_address"}, logger.Logger))
			handler.Use(nextHandler)
		})
		It("only logs them once", func() {
			handler.ServeHTTP(resp, req)
			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)

			Expect(alr.ExtraFields).To(Equal([]string{"local_address"}))
		})
	})

	Context("when there are backend request headers on the context", func() {
		BeforeEach(func() {
			extraHeadersHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				reqInfo, err := handlers.ContextRequestInfo(req)
				if err == nil {
					reqInfo.BackendReqHeaders = testHeaders
				}
			})

			handler.UseHandlerFunc(extraHeadersHandler)
		})
		It("uses those headers instead", func() {
			handler.ServeHTTP(resp, req)

			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)

			Expect(alr.Request.Header).To(Equal(req.Header))
			Expect(alr.Request.Method).To(Equal(req.Method))
			Expect(alr.Request.URL).To(Equal(req.URL))
			Expect(alr.Request.RemoteAddr).To(Equal(req.RemoteAddr))
			Expect(alr.HeadersOverride).To(Equal(testHeaders))
		})
	})

	Context("when request info is not set on the request context", func() {
		BeforeEach(func() {
			handler = negroni.New()
			handler.UseFunc(testProxyWriterHandler)
			handler.Use(handlers.NewAccessLog(accessLogger, extraHeadersToLog, nil, logger.Logger))
			handler.Use(nextHandler)
		})
		It("calls Panic on the logger", func() {
			Expect(func() { handler.ServeHTTP(resp, req) }).To(Panic())
		})
	})

	Context("when there is an X-Cf-RouterError header on the response", func() {
		BeforeEach(func() {
			resp.Header().Add("X-Cf-RouterError", "endpoint-failed")
		})

		It("logs the header and value", func() {
			handler.ServeHTTP(resp, req)
			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)

			Expect(alr.RouterError).To(Equal("endpoint-failed"))
		})
	})

	Context("when the client disconnects during response streaming", func() {
		BeforeEach(func() {
			resp = &failingResponseWriter{ResponseRecorder: httptest.NewRecorder()}
			handler = negroni.New()
			handler.Use(handlers.NewRequestInfo())
			handler.Use(handlers.NewProxyWriter(logger.Logger))
			handler.Use(handlers.NewAccessLog(accessLogger, extraHeadersToLog, nil, logger.Logger))
			handler.UseFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
				rw.WriteHeader(http.StatusOK)
				rw.Write([]byte("partial streaming data"))
				nextCalled = true
				reqChan <- req
				panic(http.ErrAbortHandler)
			})
		})

		It("writes the access log and sets RouterError to ConnectionCloseDuringStreamingErrMsg", func() {
			Expect(func() { handler.ServeHTTP(resp, req) }).To(Panic())
			Expect(accessLogger.LogCallCount()).To(Equal(1))
			alr := accessLogger.LogArgsForCall(0)
			Expect(alr.RouterError).To(Equal(utils.ConnectionCloseDuringStreamingErrMsg))
		})

		Context("when RouterError is already set on the response header", func() {
			BeforeEach(func() {
				resp.Header().Add("X-Cf-RouterError", "endpoint-failed")
			})

			It("writes the access log and keeps RouterError from response header", func() {
				Expect(func() { handler.ServeHTTP(resp, req) }).To(Panic())
				Expect(accessLogger.LogCallCount()).To(Equal(1))
				alr := accessLogger.LogArgsForCall(0)
				Expect(alr.RouterError).NotTo(Equal(utils.ConnectionCloseDuringStreamingErrMsg))
				Expect(alr.RouterError).To(Equal("endpoint-failed"))
			})
		})
	})

	Context("when CallerIdentity is set on the request context", func() {
		BeforeEach(func() {
			handler = negroni.New()
			handler.Use(handlers.NewRequestInfo())
			handler.Use(handlers.NewProxyWriter(logger.Logger))
			handler.Use(handlers.NewAccessLog(accessLogger, extraHeadersToLog, nil, logger.Logger))
			handler.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
				reqInfo, err := handlers.ContextRequestInfo(req)
				if err == nil {
					reqInfo.CallerIdentity = &handlers.CallerIdentity{
						AppGUID:   "app-guid-123",
						SpaceGUID: "space-guid-456",
						OrgGUID:   "org-guid-789",
					}
				}
				next(rw, req)
			}))
			handler.Use(nextHandler)
		})

		It("populates CallerCFApp, CallerCFSpace, CallerCFOrg on the access log record", func() {
			handler.ServeHTTP(resp, req)
			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)
			Expect(alr.CallerCFApp).To(Equal("app-guid-123"))
			Expect(alr.CallerCFSpace).To(Equal("space-guid-456"))
			Expect(alr.CallerCFOrg).To(Equal("org-guid-789"))
		})
	})

	Context("when CallerIdentity is nil on the request context", func() {
		It("leaves CallerCFApp, CallerCFSpace, CallerCFOrg empty", func() {
			handler.ServeHTTP(resp, req)
			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)
			Expect(alr.CallerCFApp).To(BeEmpty())
			Expect(alr.CallerCFSpace).To(BeEmpty())
			Expect(alr.CallerCFOrg).To(BeEmpty())
		})
	})

	Context("when AuthResult is set on the request context", func() {
		BeforeEach(func() {
			handler = negroni.New()
			handler.Use(handlers.NewRequestInfo())
			handler.Use(handlers.NewProxyWriter(logger.Logger))
			handler.Use(handlers.NewAccessLog(accessLogger, extraHeadersToLog, nil, logger.Logger))
			handler.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
				reqInfo, err := handlers.ContextRequestInfo(req)
				if err == nil {
					reqInfo.AuthResult = &handlers.AuthResult{
						Outcome: "allowed",
						Rule:    "route:cf:app:app-guid-123",
					}
				}
				next(rw, req)
			}))
			handler.Use(nextHandler)
		})

		It("populates RoutePolicy on the access log record", func() {
			handler.ServeHTTP(resp, req)
			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)
			Expect(alr.RoutePolicy).To(Equal("route:cf:app:app-guid-123"))
		})
	})

	Context("when AuthResult is nil on the request context", func() {
		It("leaves RoutePolicy empty", func() {
			handler.ServeHTTP(resp, req)
			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)
			Expect(alr.RoutePolicy).To(BeEmpty())
		})
	})

	Context("when TlsSNI is set on the request context", func() {
		BeforeEach(func() {
			handler = negroni.New()
			handler.Use(handlers.NewRequestInfo())
			handler.Use(handlers.NewProxyWriter(logger.Logger))
			handler.Use(handlers.NewAccessLog(accessLogger, extraHeadersToLog, nil, logger.Logger))
			handler.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
				reqInfo, err := handlers.ContextRequestInfo(req)
				if err == nil {
					reqInfo.TlsSNI = "backend.apps.internal"
				}
				next(rw, req)
			}))
			handler.Use(nextHandler)
		})

		It("populates TlsSNI on the access log record", func() {
			handler.ServeHTTP(resp, req)
			Expect(accessLogger.LogCallCount()).To(Equal(1))

			alr := accessLogger.LogArgsForCall(0)
			Expect(alr.TlsSNI).To(Equal("backend.apps.internal"))
		})
	})

})

type failingResponseWriter struct {
	*httptest.ResponseRecorder
}

func (f *failingResponseWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}
