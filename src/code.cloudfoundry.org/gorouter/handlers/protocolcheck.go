package handlers

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/errorwriter"
)

type protocolCheck struct {
	logger      *slog.Logger
	errorWriter errorwriter.ErrorWriter
	enableHTTP2 bool
}

// NewProtocolCheck creates a handler responsible for checking the protocol of
// the request
func NewProtocolCheck(logger *slog.Logger, errorWriter errorwriter.ErrorWriter, enableHTTP2 bool) negroni.Handler {
	return &protocolCheck{
		logger:      logger,
		errorWriter: errorWriter,
		enableHTTP2: enableHTTP2,
	}
}

func (p *protocolCheck) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	logger := LoggerWithTraceInfo(p.logger, r)
	if !p.isProtocolSupported(r) {
		// must be hijacked, otherwise no response is sent back
		conn, buf, err := p.hijack(rw)
		if err != nil {
			p.errorWriter.WriteError(
				rw,
				http.StatusBadRequest,
				"Unsupported protocol",
				logger,
			)
			return
		}

		fmt.Fprintf(buf, "HTTP/1.0 400 Bad Request\r\n\r\n")
		// #nosec G104 - ignore errors when writing HTTP responses so we don't spam our logs during a DoS
		buf.Flush()
		// #nosec G104 - ignore errors when writing HTTP responses so we don't spam our logs during a DoS
		conn.Close()
		return
	}
	if !isRequestTargetSupported(r) {
		AddRouterErrorHeader(rw, "invalid_request_target")
		logger.Info("invalid-request-target")
		p.errorWriter.WriteError(rw, http.StatusBadRequest, "Invalid request target", logger)
		return
	}
	if r.URL.Scheme != "" && !strings.EqualFold(r.URL.Scheme, "http") && !strings.EqualFold(r.URL.Scheme, "https") {
		logger.Info("non-http-request-target", slog.String("scheme", r.URL.Scheme))
	}

	next(rw, r)
}

// isRequestTargetSupported accepts origin-form and absolute-form targets with any scheme if authority is valid.
// Non-http schemes were previously routed by host and path; routing and signing use only authority and path.
// Opaque targets have no authority to route or sign against.
// Empty host is rejected as required by RFC 9110.
// CONNECT and non-OPTIONS asterisk targets never route to an app; they previously returned 502 or 404,
// so the 400 isn't a breaking change.
// Do not relax the rejected forms without re-reviewing route service signature binding.
func isRequestTargetSupported(r *http.Request) bool {
	if r.URL.Opaque != "" {
		return false
	}

	if r.URL.Scheme == "" && r.URL.Host == "" && strings.HasPrefix(r.RequestURI, "/") {
		return true
	}

	if r.URL.Scheme == "" || r.URL.Host == "" {
		return false
	}
	authority, err := url.Parse((&url.URL{Scheme: "http", Host: r.URL.Host}).String())
	return err == nil && authority.Host == r.URL.Host
}

func (p *protocolCheck) hijack(rw http.ResponseWriter) (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer cannot hijack")
	}
	return hijacker.Hijack()
}

func (p *protocolCheck) isProtocolSupported(request *http.Request) bool {
	return (p.enableHTTP2 && request.ProtoMajor == 2) || (request.ProtoMajor == 1 && (request.ProtoMinor == 0 || request.ProtoMinor == 1))
}
