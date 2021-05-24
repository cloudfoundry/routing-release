package handlers

import (
	"net/http"

	"code.cloudfoundry.org/routing-release/gorouter/common/uuid"
	"code.cloudfoundry.org/routing-release/gorouter/logger"
	"go.uber.org/zap"
	"github.com/urfave/negroni"
)

const (
	VcapRequestIdHeader = "X-Vcap-Request-Id"
)

type setVcapRequestIdHeader struct {
	logger logger.Logger
}

func NewVcapRequestIdHeader(logger logger.Logger) negroni.Handler {
	return &setVcapRequestIdHeader{
		logger: logger,
	}
}

func (s *setVcapRequestIdHeader) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// The X-Vcap-Request-Id must be set before the request is passed into the
	// dropsonde InstrumentedHandler

	guid, err := uuid.GenerateUUID()
	if err == nil {
		r.Header.Set(VcapRequestIdHeader, guid)
		s.logger.Debug("vcap-request-id-header-set", zap.String("VcapRequestIdHeader", guid))
	} else {
		s.logger.Error("failed-to-set-vcap-request-id-header", zap.Error(err))
	}

	next(rw, r)
}
