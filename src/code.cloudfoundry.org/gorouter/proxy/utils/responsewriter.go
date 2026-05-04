package utils

import (
	"bufio"
	"errors"
	"log/slog"
	"net"
	"net/http"
)

type ProxyResponseWriter interface {
	Header() http.Header
	Hijack() (net.Conn, *bufio.ReadWriter, error)
	Write(b []byte) (int, error)
	WriteHeader(s int)
	Done()
	Flush()
	Status() int
	SetStatus(status int)
	Size() int
	AddHeaderRewriter(HeaderRewriter)
	WriteError() error
}

const ConnectionCloseDuringStreamingErrMsg = "client-conn-closed-during-response-streaming"

type proxyResponseWriter struct {
	w      http.ResponseWriter
	status int
	size   int

	logger  *slog.Logger
	flusher http.Flusher
	done    bool

	writeErr error

	headerRewriters []HeaderRewriter
}

func NewProxyResponseWriter(w http.ResponseWriter, logger *slog.Logger) *proxyResponseWriter {
	proxyWriter := &proxyResponseWriter{
		w:       w,
		flusher: w.(http.Flusher),
		logger:  logger,
	}

	return proxyWriter
}

func (p *proxyResponseWriter) Header() http.Header {
	return p.w.Header()
}

func (p *proxyResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := p.w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer cannot hijack")
	}
	return hijacker.Hijack()
}

func (p *proxyResponseWriter) Write(b []byte) (int, error) {
	if p.done {
		return 0, nil
	}

	if p.status == 0 {
		p.WriteHeader(http.StatusOK)
	}
	size, err := p.w.Write(b)
	if err != nil && p.writeErr == nil {
		p.writeErr = err
		p.logger.Error(ConnectionCloseDuringStreamingErrMsg,
			slog.String("error", err.Error()),
			slog.Int("bytes_written", size),
			slog.Int("total_size", p.size),
			slog.Int("status", p.status))
	}
	p.size += size
	return size, err
}

func (p *proxyResponseWriter) WriteHeader(s int) {
	if p.done {
		return
	}

	// if Content-Type not in response, nil out to suppress Go's auto-detect
	if _, ok := p.w.Header()["Content-Type"]; !ok {
		p.w.Header()["Content-Type"] = nil
	}

	for _, headerRewriter := range p.headerRewriters {
		headerRewriter.RewriteHeader(p.w.Header())
	}

	p.w.WriteHeader(s)

	if p.status == 0 || (p.status >= 100 && p.status <= 199) {
		p.status = s
	}
}

func (p *proxyResponseWriter) Done() {
	p.done = true
}

func (p *proxyResponseWriter) Flush() {
	if p.flusher != nil {
		p.flusher.Flush()
	}
}

func (p *proxyResponseWriter) Status() int {
	return p.status
}

// SetStatus should be used when the ResponseWriter has been hijacked
// so WriteHeader is not valid but still needs to save a status code
func (p *proxyResponseWriter) SetStatus(status int) {
	p.status = status
}

func (p *proxyResponseWriter) Size() int {
	return p.size
}

// Satisfy http.ResponseController support (Go 1.20+)
func (p *proxyResponseWriter) Unwrap() http.ResponseWriter {
	return p.w
}

func (p *proxyResponseWriter) AddHeaderRewriter(r HeaderRewriter) {
	p.headerRewriters = append(p.headerRewriters, r)
}

func (p *proxyResponseWriter) WriteError() error {
	return p.writeErr
}
