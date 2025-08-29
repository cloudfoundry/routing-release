package route

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	log "code.cloudfoundry.org/gorouter/logger"
)

// HashBased load balancing algorithm distributes requests based on a hash of a specific header value.
// The sticky session cookie has precedence over hash-based routing and the request should be routed to the instance stored in the cookie.
// If requests do not contain the hash-related header set configured for the hash-based route option, use the default load-balancing algorithm.
type HashBased struct {
	lock *sync.Mutex

	logger       *slog.Logger
	pool         *EndpointPool
	lastEndpoint *Endpoint

	stickyEndpointID string
	mustBeSticky     bool

	HeaderValue string
}

// NewHashBased initializes an endpoint iterator that selects endpoints based on a hash of a header value.
// The global properties locallyOptimistic and localAvailabilityZone will be ignored when using Hash-Based Routing.
func NewHashBased(logger *slog.Logger, p *EndpointPool, initial string, mustBeSticky bool, locallyOptimistic bool, localAvailabilityZone string) EndpointIterator {
	return &HashBased{
		logger:           logger,
		pool:             p,
		lock:             &sync.Mutex{},
		stickyEndpointID: initial,
		mustBeSticky:     mustBeSticky,
	}
}

// Next selects the next endpoint based on the hash of the header value.
// If a sticky session endpoint is available and not overloaded, it will be returned.
// If the request must be sticky and the sticky endpoint is unavailable or overloaded, nil will be returned.
// If no sticky session is present, the endpoint will be selected based on the hash of the header value.
// It returns the same endpoint for the same header value consistently.
// If the hash lookup fails or the endpoint is not found, nil will be returned.
func (h *HashBased) Next(attempt int) *Endpoint {
	h.lock.Lock()
	defer h.lock.Unlock()

	e := h.findEndpointIfStickySession()
	if e == nil && h.mustBeSticky {
		return nil
	}

	if e != nil {
		h.lastEndpoint = e
		return e
	}

	if h.pool.HashLookupTable == nil {
		h.logger.Error("hash-based-routing-failed", slog.String("host", h.pool.host), log.ErrAttr(errors.New("Lookup table is empty")))
		return nil
	}

	id, err := h.pool.HashLookupTable.Get(h.HeaderValue)

	if err != nil {
		h.logger.Error(
			"hash-based-routing-failed",
			slog.String("host", h.pool.host),
			log.ErrAttr(err),
		)
		return nil
	}

	h.logger.Debug(
		"hash-based-routing",
		slog.String("hash header value", h.HeaderValue),
		slog.String("endpoint-id", id),
	)

	endpointElem := h.pool.findById(id)
	if endpointElem == nil {
		h.logger.Error("hash-based-routing-failed", slog.String("host", h.pool.host), log.ErrAttr(errors.New("Endpoint not found in pool")), slog.String("endpoint-id", id))
		return nil
	}

	return endpointElem.endpoint
}

// findEndpointIfStickySession checks if there is a sticky session endpoint and returns it if available.
// If the sticky session endpoint is overloaded, returns nil.
func (h *HashBased) findEndpointIfStickySession() *Endpoint {
	var e *endpointElem
	if h.stickyEndpointID != "" {
		e = h.pool.findById(h.stickyEndpointID)
		if e != nil && e.isOverloaded() {
			if h.mustBeSticky {
				if h.logger.Enabled(context.Background(), slog.LevelDebug) {
					h.logger.Debug("endpoint-overloaded-but-request-must-be-sticky", e.endpoint.ToLogData()...)
				}
				return nil
			}
			e = nil
		}

		if e == nil && h.mustBeSticky {
			h.logger.Debug("endpoint-missing-but-request-must-be-sticky", slog.String("requested-endpoint", h.stickyEndpointID))
			return nil
		}

		if !h.mustBeSticky {
			h.logger.Debug("endpoint-missing-choosing-alternate", slog.String("requested-endpoint", h.stickyEndpointID))
			h.stickyEndpointID = ""
		}
	}

	if e != nil {
		e.RLock()
		defer e.RUnlock()
		return e.endpoint
	}
	return nil
}

// EndpointFailed notifies the endpoint pool that the last selected endpoint has failed.
func (h *HashBased) EndpointFailed(err error) {
	if h.lastEndpoint != nil {
		h.pool.EndpointFailed(h.lastEndpoint, err)
	}
}

// PreRequest increments the in-flight request count for the selected endpoint from current Gorouter.
func (h *HashBased) PreRequest(e *Endpoint) {
	e.Stats.NumberConnections.Increment()
}

// PostRequest decrements the in-flight request count for the selected endpoint from current Gorouter.
func (h *HashBased) PostRequest(e *Endpoint) {
	e.Stats.NumberConnections.Decrement()
}
