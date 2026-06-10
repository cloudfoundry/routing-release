package handlers

import (
	"net/http"

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
