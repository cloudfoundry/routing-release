package route

import (
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

	logger               *slog.Logger
	pool                 *EndpointPool
	lastEndpoint         *Endpoint
	lastLookupTableIndex uint64

	stickyEndpointID string
	mustBeSticky     bool

	HeaderValue string
}

// NewHashBased initializes an endpoint iterator that selects endpoints based on a hash of a header value.
// The global properties locallyOptimistic and localAvailabilityZone will be ignored when using Hash-Based Routing.
func NewHashBased(logger *slog.Logger, p *EndpointPool, initial string, mustBeSticky bool, headerValue string) EndpointIterator {
	return &HashBased{
		logger:           logger,
		pool:             p,
		lock:             &sync.Mutex{},
		stickyEndpointID: initial,
		mustBeSticky:     mustBeSticky,
		HeaderValue:      headerValue,
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

	endpoint := h.pool.FindStickyEndpoint(h.logger, h.stickyEndpointID, h.mustBeSticky)
	if endpoint != nil {
		h.lastEndpoint = endpoint
		return endpoint
	}

	if h.mustBeSticky {
		return nil
	}

	h.stickyEndpointID = "" // unset initial endpoint as it was not found in the pool

	// Check for empty pool
	if len(h.pool.endpoints) == 0 {
		h.logger.Warn("hash-based-routing-pool-empty", slog.String("host", h.pool.host))
		return nil
	}

	endpoint = h.getSingleEndpoint()
	if endpoint != nil {
		h.lastEndpoint = endpoint
		return endpoint
	}

	// Perform hash-based selection
	endpoint = h.selectHashBasedEndpoint(attempt)
	if endpoint != nil {
		h.lastEndpoint = endpoint
	}
	return endpoint
}

// selectHashBasedEndpoint performs hash-based endpoint selection using the lookup table.
func (h *HashBased) selectHashBasedEndpoint(attempt int) *Endpoint {
	if h.pool.HashLookupTable == nil {
		h.logger.Error("hash-based-routing-failed", slog.String("host", h.pool.host), log.ErrAttr(errors.New("lookup table is empty")))
		return nil
	}

	startIndex, err := h.getStartingIndex(attempt)
	if err != nil {
		h.logger.Error("hash-based-routing-failed", slog.String("host", h.pool.host), log.ErrAttr(err))
		return nil
	}

	return h.findEndpoint(startIndex, attempt)
}

// getStartingIndex determines the starting index in the lookup table based on the attempt number.
// For the initial attempt, it uses the hash of the header value.
// For retries, it uses the next index after the last lookup.
func (h *HashBased) getStartingIndex(attempt int) (uint64, error) {
	if attempt == 0 || h.lastLookupTableIndex == 0 {
		index, _, err := h.pool.HashLookupTable.GetInstanceForHashHeader(h.HeaderValue)
		return index, err
	}

	// On retries, start from the next index in the lookup table
	nextIndex := (h.lastLookupTableIndex + 1) % h.pool.HashLookupTable.GetLookupTableSize()
	return nextIndex, nil
}

func (h *HashBased) findEndpoint(index uint64, attempt int) *Endpoint {
	// Ensure we don't exceed the lookup table size
	lookupTableSize := h.pool.HashLookupTable.GetLookupTableSize()

	// Normalize index
	currentIndex := index % lookupTableSize
	// Keep track of endpoints already visited, to avoid visiting them twice
	visitedEndpoints := make(map[string]bool)

	numberOfEndpoints := len(h.pool.HashLookupTable.GetEndpointList())

	lastEndpointPrivateId := ""
	if attempt > 0 && h.lastEndpoint != nil {
		lastEndpointPrivateId = h.lastEndpoint.PrivateInstanceId
	}

	// abort when we have visited all available endpoints unsuccessfully
	for len(visitedEndpoints) < numberOfEndpoints {
		id := h.pool.HashLookupTable.GetEndpointId(currentIndex)

		if visitedEndpoints[id] || id == lastEndpointPrivateId {
			currentIndex = (currentIndex + 1) % lookupTableSize
			continue
		}
		visitedEndpoints[id] = true

		endpointElem := h.pool.findById(id)
		if endpointElem == nil {
			h.logger.Error("hash-based-routing-failed", slog.String("host", h.pool.host), log.ErrAttr(errors.New("endpoint not found in pool")), slog.String("endpoint-id", id))
			currentIndex = (currentIndex + 1) % lookupTableSize
			continue
		}

		if endpointElem.isOverloaded() {
			// If the selected endpoint has reached the limit of max request per backend, log the info about it and try the next one in the lookup table
			h.logger.Info("hash-based-routing-endpoint-overloaded", slog.String("host", h.pool.host), slog.String("endpoint-id", endpointElem.endpoint.PrivateInstanceId))
		} else if !h.IsImbalanced(endpointElem.endpoint) {
			h.lastLookupTableIndex = currentIndex
			return endpointElem.endpoint
		}

		currentIndex = (currentIndex + 1) % lookupTableSize
	}
	// All endpoints checked and overloaded or not found
	h.logger.Error("hash-based-routing-failed", slog.String("host", h.pool.host), log.ErrAttr(errors.New("all endpoints are overloaded")))
	return nil
}

func (h *HashBased) IsImbalanced(endpoint *Endpoint) bool {
	// endpoint cannot be imbalanced if balance factor is not set
	if h.pool.HashRoutingProperties.BalanceFactor <= 0 {
		return false
	}

	avgNumberOfInFlightRequests := h.CalculateAverageLoad()
	// Check if avgNumberOfInFlightRequests is 0 to avoid division by 0 in the next if-condition
	if avgNumberOfInFlightRequests == 0 {
		return false
	}

	currentInFlightRequestCount := endpoint.Stats.NumberConnections.Count()
	balanceFactor := h.pool.HashRoutingProperties.BalanceFactor

	if float64(currentInFlightRequestCount)/avgNumberOfInFlightRequests > balanceFactor {
		h.logger.Debug("hash-based-routing-endpoint-imbalanced", slog.String("host", h.pool.host), slog.String("endpoint-id", endpoint.PrivateInstanceId), slog.Int64("endpoint-connections", currentInFlightRequestCount), slog.Float64("average-load", avgNumberOfInFlightRequests))
		return true
	}
	return false
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

// CalculateAverageLoad computes the average number of in-flight requests across all endpoints in the pool.
func (h *HashBased) CalculateAverageLoad() float64 {
	if len(h.pool.endpoints) == 0 {
		return 0
	}

	var currentInFlightRequestCount int64
	for _, endpointElem := range h.pool.endpoints {
		currentInFlightRequestCount += endpointElem.endpoint.Stats.NumberConnections.Count()
	}

	return float64(currentInFlightRequestCount) / float64(len(h.pool.endpoints))
}

func (h *HashBased) getSingleEndpoint() *Endpoint {
	if len(h.pool.endpoints) == 1 {
		e := h.pool.endpoints[0]
		if e.isOverloaded() {
			return nil
		}

		return e.endpoint
	}
	return nil
}
