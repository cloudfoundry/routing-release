package postselection

import (
	"log/slog"

	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/route"
)

// PostSelectionHandler represents a single authorization check that runs after
// endpoint selection. Each handler inspects the selected endpoint and request
// context to make an authorization decision.
//
// Handlers are composable and run in sequence. The first handler to return an
// error stops the pipeline and causes the request to be rejected.
//
//go:generate counterfeiter -o fakes/fake_post_selection_handler.go . PostSelectionHandler
type PostSelectionHandler interface {
	// Check performs an authorization check against the selected endpoint.
	// Returns nil if authorized, or an AuthError if denied.
	Check(endpoint *route.Endpoint, reqInfo *handlers.RequestInfo) error
}

// PostSelectionPipeline runs a sequence of post-selection authorization handlers.
// This enables composable, layered authorization checks after the load balancer
// has selected a specific backend endpoint.
type PostSelectionPipeline struct {
	handlers []PostSelectionHandler
	logger   *slog.Logger
}

// NewPostSelectionPipeline creates a new authorization pipeline with the given handlers.
// Handlers are executed in the order provided.
func NewPostSelectionPipeline(logger *slog.Logger, handlers ...PostSelectionHandler) *PostSelectionPipeline {
	return &PostSelectionPipeline{
		handlers: handlers,
		logger:   logger,
	}
}

// Run executes all handlers in sequence. Returns nil if all handlers pass,
// or the first error encountered.
func (p *PostSelectionPipeline) Run(endpoint *route.Endpoint, reqInfo *handlers.RequestInfo) error {
	if p == nil || len(p.handlers) == 0 {
		return nil // No handlers configured, allow request
	}

	for _, handler := range p.handlers {
		if err := handler.Check(endpoint, reqInfo); err != nil {
			// First failure stops the pipeline
			return err
		}
	}

	return nil // All handlers passed
}
