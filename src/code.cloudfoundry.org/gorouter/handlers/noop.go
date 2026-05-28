package handlers

import (
	"net/http"

	"code.cloudfoundry.org/gorouter/route"
	"github.com/urfave/negroni/v3"
)

// noopNegroniHandler is a negroni handler that does nothing but call the next handler.
type noopNegroniHandler struct{}

func (h *noopNegroniHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next(w, r)
}

// NoopHandler is a negroni handler that does nothing but call the next handler.
// Use this when a handler should be conditionally disabled based on configuration.
var NoopHandler negroni.Handler = &noopNegroniHandler{}

// noopPostSelectionHandler is a PostSelectionHandler that always allows the request.
type noopPostSelectionHandler struct{}

func (h *noopPostSelectionHandler) Check(endpoint *route.Endpoint, reqInfo *RequestInfo) error {
	return nil
}

// NoopPostSelectionHandler is a PostSelectionHandler that does nothing.
// Use this when a post-selection handler should be conditionally disabled.
var NoopPostSelectionHandler PostSelectionHandler = &noopPostSelectionHandler{}
