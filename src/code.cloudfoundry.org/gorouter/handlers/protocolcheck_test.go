package handlers_test

import (
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/urfave/negroni/v3"
	"golang.org/x/net/http2"

	"code.cloudfoundry.org/gorouter/errorwriter"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("Protocolcheck", func() {
	var (
		logger *test_util.TestLogger
		ew     = errorwriter.NewPlaintextErrorWriter()

		nextCalled  bool
		server      *ghttp.Server
		n           *negroni.Negroni
		enableHTTP2 bool
		prevHandler negroni.Handler
	)

	BeforeEach(func() {
		logger = test_util.NewTestLogger("protocolcheck")
		nextCalled = false
		prevHandler = &PrevHandler{}
	})

	JustBeforeEach(func() {
		n = negroni.New()
		n.UseFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
			next(rw, req)
		})
		n.Use(prevHandler)
		n.Use(handlers.NewProtocolCheck(logger.Logger, ew, enableHTTP2))
		n.UseHandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		server = ghttp.NewUnstartedServer()
		server.AppendHandlers(n.ServeHTTP)
		server.Start()
	})

	AfterEach(func() {
		server.Close()
	})

	Context("http2", func() {
		Context("when http2 is enabled", func() {
			BeforeEach(func() {
				enableHTTP2 = true
			})

			It("handles HTTP/2 requests and rejects CONNECT request targets", func() {
				http2Server := httptest.NewUnstartedServer(n)
				http2Server.EnableHTTP2 = true
				http2Server.StartTLS()
				defer http2Server.Close()

				client := &http.Client{Transport: &http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
				defer client.CloseIdleConnections()

				response, err := client.Get(http2Server.URL)
				Expect(err).NotTo(HaveOccurred())
				response.Body.Close()
				Expect(response.ProtoMajor).To(Equal(2))
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(nextCalled).To(BeTrue())

				nextCalled = false
				connectReq := httptest.NewRequest(http.MethodConnect, "http://example.com:443", nil)
				connectReq.ProtoMajor = 2
				connectReq.RequestURI = "example.com:443"
				connectReq.URL.Scheme = ""
				connectReq.URL.Host = "example.com:443"
				connectReq.URL.Path = ""
				connectResp := httptest.NewRecorder()
				n.ServeHTTP(connectResp, connectReq)

				Expect(connectResp.Code).To(Equal(http.StatusBadRequest))
				Expect(connectResp.Header().Get("X-Cf-Routererror")).To(Equal("invalid_request_target"))
				Expect(nextCalled).To(BeFalse())
			})
		})

		Context("when http2 is disabled", func() {
			BeforeEach(func() {
				enableHTTP2 = false
			})

			It("returns a 400 with a helpful error ", func() {
				conn, err := net.Dial("tcp", server.Addr())
				Expect(err).ToNot(HaveOccurred())
				defer conn.Close()
				respReader := bufio.NewReader(conn)

				conn.Write([]byte("PRI * HTTP/2.0\r\nHost: example.com\r\n\r\n"))
				resp, err := http.ReadResponse(respReader, nil)
				Expect(err).ToNot(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(400))
				Expect(nextCalled).To(BeFalse())
			})
		})
	})

	Context("http 1.1", func() {
		It("passes the request through", func() {
			conn, err := net.Dial("tcp", server.Addr())
			Expect(err).ToNot(HaveOccurred())
			defer conn.Close()
			respReader := bufio.NewReader(conn)

			conn.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
			resp, err := http.ReadResponse(respReader, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(200))
			Expect(nextCalled).To(BeTrue())
		})
	})

	Context("http 1.0", func() {
		It("passes the request through", func() {
			conn, err := net.Dial("tcp", server.Addr())
			Expect(err).ToNot(HaveOccurred())
			defer conn.Close()
			respReader := bufio.NewReader(conn)

			conn.Write([]byte("GET / HTTP/1.0\r\nHost: example.com\r\n\r\n"))
			resp, err := http.ReadResponse(respReader, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(200))
			Expect(nextCalled).To(BeTrue())
		})
	})

	Context("request-target forms", func() {
		It("rejects unsupported forms and passes supported forms", func() {
			cases := []struct {
				name       string
				request    string
				statusCode int
				passed     bool
			}{
				{name: "opaque", request: "GET x:pass@other.example/ HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusBadRequest},
				{name: "ws opaque", request: "GET ws:opaque HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusBadRequest},
				{name: "ws no authority", request: "GET ws:/p HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusBadRequest},
				{name: "ws empty host", request: "GET ws:///p HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusBadRequest},
				{name: "authority", request: "CONNECT host:443 HTTP/1.1\r\nHost: host:443\r\nConnection: close\r\n\r\n", statusCode: http.StatusBadRequest},
				{name: "userinfo", request: "GET http://u:p@host/ HTTP/1.1\r\nHost: host\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws absolute", request: "GET ws://h/p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "wss absolute", request: "GET wss://h/p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ftp absolute", request: "GET ftp://h/p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "custom scheme", request: "GET custom+v1://h/p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws userinfo", request: "GET ws://u:p@h/p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws no path", request: "GET ws://h HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws query only", request: "GET ws://h?x=1 HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws port", request: "GET ws://h:8080/p HTTP/1.1\r\nHost: h:8080\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws empty port", request: "GET ws://h:/p HTTP/1.1\r\nHost: h:\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws ipv6", request: "GET ws://[::1]:80/p HTTP/1.1\r\nHost: [::1]:80\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "http empty port", request: "GET http://h:/p HTTP/1.1\r\nHost: h:\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "http percent-escaped host", request: "GET http://a%25.example.com/ HTTP/1.1\r\nHost: a.example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws percent-escaped host", request: "GET ws://a%25.example.com/p HTTP/1.1\r\nHost: a.example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "ws multi-colon authority", request: "GET ws://h:80:90/p HTTP/1.1\r\nHost: h:80:90\r\nConnection: close\r\n\r\n", statusCode: http.StatusBadRequest},
				{name: "asterisk", request: "GET * HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusBadRequest},
				{name: "origin", request: "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "http absolute", request: "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "https absolute", request: "GET https://example.com/ HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK, passed: true},
				{name: "options asterisk", request: "OPTIONS * HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n", statusCode: http.StatusOK},
			}

			for _, testCase := range cases {
				server.Close()
				server = ghttp.NewUnstartedServer()
				server.AppendHandlers(n.ServeHTTP)
				server.Start()
				nextCalled = false
				conn, err := net.Dial("tcp", server.Addr())
				Expect(err).ToNot(HaveOccurred(), testCase.name)
				respReader := bufio.NewReader(conn)

				_, err = conn.Write([]byte(testCase.request))
				Expect(err).ToNot(HaveOccurred(), testCase.name)
				resp, err := http.ReadResponse(respReader, nil)
				Expect(err).ToNot(HaveOccurred(), testCase.name)
				conn.Close()

				Expect(resp.StatusCode).To(Equal(testCase.statusCode), testCase.name)
				if testCase.statusCode == http.StatusBadRequest {
					Expect(resp.Header.Get("X-Cf-Routererror")).To(Equal("invalid_request_target"), testCase.name)
					Expect(nextCalled).To(BeFalse(), testCase.name)
				} else if testCase.passed {
					Expect(nextCalled).To(BeTrue(), testCase.name)
				} else {
					Expect(resp.Header.Get("X-Cf-Routererror")).ToNot(Equal("invalid_request_target"), testCase.name)
					Expect(nextCalled).To(BeFalse(), testCase.name)
				}
			}
		})
	})

	It("logs scheme for non-http request target", func() {
		for _, target := range []struct {
			request string
			logged  bool
		}{
			{request: "GET ws://h/p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n", logged: true},
			{request: "GET http://h/p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n"},
			{request: "GET /p HTTP/1.1\r\nHost: h\r\nConnection: close\r\n\r\n"},
		} {
			previousLines := len(logger.TestSink.Lines())
			server.Close()
			server = ghttp.NewUnstartedServer()
			server.AppendHandlers(n.ServeHTTP)
			server.Start()
			conn, err := net.Dial("tcp", server.Addr())
			Expect(err).NotTo(HaveOccurred())
			_, err = conn.Write([]byte(target.request))
			Expect(err).NotTo(HaveOccurred())
			_, err = http.ReadResponse(bufio.NewReader(conn), nil)
			Expect(err).NotTo(HaveOccurred())
			conn.Close()
			if target.logged {
				Expect(logger.TestSink.Lines()[previousLines:]).To(ContainElement(And(
					ContainSubstring(`"message":"non-http-request-target"`),
					ContainSubstring(`"scheme":"ws"`),
				)))
			} else {
				Expect(logger.TestSink.Lines()[previousLines:]).NotTo(ContainElement(ContainSubstring("non-http-request-target")))
			}
		}
	})

	Context("unsupported versions of http", func() {
		It("returns a 400 bad request", func() {
			conn, err := net.Dial("tcp", server.Addr())
			Expect(err).ToNot(HaveOccurred())
			respReader := bufio.NewReader(conn)

			conn.Write([]byte("GET / HTTP/1.5\r\nHost: example.com\r\n\r\n"))
			resp, err := http.ReadResponse(respReader, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			Expect(nextCalled).To(BeFalse())
		})
	})
})
