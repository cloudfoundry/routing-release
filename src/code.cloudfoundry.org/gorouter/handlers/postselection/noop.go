package postselection

import (
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/route"
)

// noopPostSelectionHandler is a PostSelectionHandler that always allows the request.
type noopPostSelectionHandler struct{}

func (h *noopPostSelectionHandler) Check(endpoint *route.Endpoint, reqInfo *handlers.RequestInfo) error {
	return nil
}

// NoopPostSelectionHandler is a PostSelectionHandler that does nothing.
// Use this when a post-selection handler should be conditionally disabled.
var NoopPostSelectionHandler PostSelectionHandler = &noopPostSelectionHandler{}
