package route

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"math/rand"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/proxy/fails"
	"code.cloudfoundry.org/routing-api/models"
)

type Counter struct {
	value int64
}

type PoolPutResult int

func (p PoolPutResult) String() string {
	switch p {
	case EndpointUnmodified:
		return "unmodified"
	case EndpointUpdated:
		return "updated"
	case EndpointAdded:
		return "added"
	case EndpointRefreshed:
		return "refreshed"
	default:
		panic("invalid PoolPutResult")
	}
}

const (
	EndpointUnmodified = PoolPutResult(iota)
	EndpointUpdated
	EndpointAdded
	EndpointRefreshed
)

func NewCounter(initial int64) *Counter {
	return &Counter{initial}
}

func (c *Counter) Increment() {
	atomic.AddInt64(&c.value, 1)
}
func (c *Counter) Decrement() {
	atomic.AddInt64(&c.value, -1)
}
func (c *Counter) Count() int64 {
	return atomic.LoadInt64(&c.value)
}

type Stats struct {
	NumberConnections *Counter
}

// RoutePolicyScopeAny, RoutePolicyScopeOrg, RoutePolicyScopeSpace are the valid values for RoutePolicyScope.
// They correspond to the route_policies_scope field in Cloud Controller.
const (
	RoutePolicyScopeAny   = "any"
	RoutePolicyScopeOrg   = "org"
	RoutePolicyScopeSpace = "space"
)

func NewStats() *Stats {
	return &Stats{
		NumberConnections: &Counter{},
	}
}

type ProxyRoundTripper interface {
	http.RoundTripper
	CancelRequest(*http.Request)
}

type RoutingProperties struct {
	RequestHeaders         *http.Header
	LocallyOptimistic      bool
	GlobalRoutingAlgorithm string
	AZ                     string
}

type HashRoutingProperties struct {
	Header        string
	BalanceFactor float64
}

func (hrp *HashRoutingProperties) Equal(hrp2 *HashRoutingProperties) bool {
	if hrp == nil && hrp2 == nil {
		return true
	}
	if hrp == nil || hrp2 == nil {
		return false
	}
	return hrp.Header == hrp2.Header && hrp.BalanceFactor == hrp2.BalanceFactor
}

type Endpoint struct {
	ApplicationId          string
	AvailabilityZone       string
	addr                   string
	Protocol               string
	Tags                   map[string]string
	ServerCertDomainSAN    string
	PrivateInstanceId      string
	StaleThreshold         time.Duration
	RouteServiceUrl        string
	PrivateInstanceIndex   string
	ModificationTag        models.ModificationTag
	Stats                  *Stats
	IsolationSegment       string
	useTls                 bool
	roundTripper           ProxyRoundTripper
	roundTripperMutex      sync.RWMutex
	UpdatedAt              time.Time
	RoundTripperInit       sync.Once
	LoadBalancingAlgorithm string
	HashHeaderName         string
	HashBalanceFactor      float64
	// RoutePolicyScope is the operator-level scope boundary: "any", "org", or "space".
	// Non-empty means access control is enforced for this endpoint's route.
	RoutePolicyScope string
	// RoutePolicies is the list of parsed sources (e.g. "cf:app:<guid>", "cf:space:<guid>",
	// "cf:org:<guid>", "cf:any"). Empty with a non-empty RoutePolicyScope means default-deny.
	RoutePolicies []string
}

func (e *Endpoint) RoundTripper() ProxyRoundTripper {
	e.roundTripperMutex.RLock()
	defer e.roundTripperMutex.RUnlock()

	return e.roundTripper
}

func (e *Endpoint) SetRoundTripper(tripper ProxyRoundTripper) {
	e.roundTripperMutex.Lock()
	defer e.roundTripperMutex.Unlock()

	e.roundTripper = tripper
}

func (e *Endpoint) SetRoundTripperIfNil(roundTripperCtor func() ProxyRoundTripper) {
	e.roundTripperMutex.Lock()
	defer e.roundTripperMutex.Unlock()

	if e.roundTripper == nil {
		e.roundTripper = roundTripperCtor()
	}
}

func (e *Endpoint) Equal(e2 *Endpoint) bool {
	if e2 == nil {
		return false
	}

	return e.ApplicationId == e2.ApplicationId &&
		e.addr == e2.addr &&
		e.Protocol == e2.Protocol &&
		e.ServerCertDomainSAN == e2.ServerCertDomainSAN &&
		e.PrivateInstanceId == e2.PrivateInstanceId &&
		e.StaleThreshold == e2.StaleThreshold &&
		e.RouteServiceUrl == e2.RouteServiceUrl &&
		e.PrivateInstanceIndex == e2.PrivateInstanceIndex &&
		e.ModificationTag == e2.ModificationTag &&
		e.IsolationSegment == e2.IsolationSegment &&
		e.useTls == e2.useTls &&
		e.UpdatedAt.Equal(e2.UpdatedAt) &&
		e.LoadBalancingAlgorithm == e2.LoadBalancingAlgorithm &&
		e.HashHeaderName == e2.HashHeaderName &&
		e.HashBalanceFactor == e2.HashBalanceFactor &&
		maps.Equal(e.Tags, e2.Tags) &&
		e.RoutePolicyScope == e2.RoutePolicyScope &&
		slices.Equal(e.RoutePolicies, e2.RoutePolicies)

}

func (e *Endpoint) ProcessId() string {
	return e.Tags["process_id"]
}

//go:generate counterfeiter -o fakes/fake_endpoint_iterator.go . EndpointIterator
type EndpointIterator interface {
	// Next MUST either return the next endpoint available or nil. It MUST NOT return the same endpoint.
	// All available endpoints MUST have been used before any can be used again.
	// ProxyRoundTripper will not retry more often than endpoints available.
	Next(attempt int) *Endpoint
	EndpointFailed(err error)
	PreRequest(e *Endpoint)
	PostRequest(e *Endpoint)
}

type endpointElem struct {
	sync.RWMutex
	endpoint           *Endpoint
	index              int
	updated            time.Time
	failedAt           *time.Time
	maxConnsPerBackend int64
}

type EndpointPool struct {
	sync.Mutex
	endpoints []*endpointElem
	index     map[string]*endpointElem

	host        string
	contextPath string
	RouteSvcUrl string

	retryAfterFailure  time.Duration
	NextIdx            int
	maxConnsPerBackend int64

	random                 *rand.Rand
	logger                 *slog.Logger
	updatedAt              time.Time
	LoadBalancingAlgorithm string
	HashRoutingProperties  *HashRoutingProperties
	HashLookupTable        MaglevLookup

	// routePolicyScope is the operator-level scope boundary: "any", "org", or "space".
	// Stored at pool level because all endpoints on the same route share the same policies.
	routePolicyScope string
	// routePolicies is the list of parsed sources (e.g. "cf:app:<guid>", "cf:space:<guid>",
	// "cf:org:<guid>", "cf:any"). Empty with a non-empty routePolicyScope means default-deny.
	routePolicies []string
}

type EndpointOpts struct {
	AppId                   string
	AvailabilityZone        string
	Host                    string
	Port                    uint16
	Protocol                string
	ServerCertDomainSAN     string
	PrivateInstanceId       string
	PrivateInstanceIndex    string
	Tags                    map[string]string
	StaleThresholdInSeconds int
	RouteServiceUrl         string
	ModificationTag         models.ModificationTag
	IsolationSegment        string
	UseTLS                  bool
	UpdatedAt               time.Time
	LoadBalancingAlgorithm  string
	HashHeaderName          string
	HashBalanceFactor       float64
	// RoutePolicyScope is the operator-level scope: "any", "org", or "space".
	// Non-empty means enforcement is active for this route.
	RoutePolicyScope string
	// RoutePolicies are the parsed sources for this route.
	// Empty + non-empty RoutePolicyScope means default-deny.
	RoutePolicies []string
}

func NewEndpoint(opts *EndpointOpts) *Endpoint {
	endpoint := &Endpoint{
		ApplicationId:          opts.AppId,
		AvailabilityZone:       opts.AvailabilityZone,
		addr:                   fmt.Sprintf("%s:%d", opts.Host, opts.Port),
		Protocol:               opts.Protocol,
		Tags:                   opts.Tags,
		useTls:                 opts.UseTLS,
		ServerCertDomainSAN:    opts.ServerCertDomainSAN,
		PrivateInstanceId:      opts.PrivateInstanceId,
		PrivateInstanceIndex:   opts.PrivateInstanceIndex,
		StaleThreshold:         time.Duration(opts.StaleThresholdInSeconds) * time.Second,
		RouteServiceUrl:        opts.RouteServiceUrl,
		ModificationTag:        opts.ModificationTag,
		Stats:                  NewStats(),
		IsolationSegment:       opts.IsolationSegment,
		UpdatedAt:              opts.UpdatedAt,
		LoadBalancingAlgorithm: opts.LoadBalancingAlgorithm,
		RoutePolicyScope:       opts.RoutePolicyScope,
		RoutePolicies:          opts.RoutePolicies,
	}

	if opts.LoadBalancingAlgorithm == config.LOAD_BALANCE_HB && opts.HashHeaderName != "" { // BalanceFactor is optional
		endpoint.HashHeaderName = opts.HashHeaderName
		endpoint.HashBalanceFactor = opts.HashBalanceFactor
	}

	return endpoint
}

func (e *Endpoint) IsTLS() bool {
	return e.useTls
}

type PoolOpts struct {
	RetryAfterFailure      time.Duration
	Host                   string
	ContextPath            string
	MaxConnsPerBackend     int64
	Logger                 *slog.Logger
	LoadBalancingAlgorithm string
	HashHeader             string
	HashBalanceFactor      float64
}

func NewPool(opts *PoolOpts) *EndpointPool {
	pool := &EndpointPool{
		endpoints:              make([]*endpointElem, 0, 1),
		index:                  make(map[string]*endpointElem),
		retryAfterFailure:      opts.RetryAfterFailure,
		NextIdx:                -1,
		maxConnsPerBackend:     opts.MaxConnsPerBackend,
		host:                   opts.Host,
		contextPath:            opts.ContextPath,
		random:                 rand.New(rand.NewSource(time.Now().UnixNano())),
		logger:                 opts.Logger,
		updatedAt:              time.Now(),
		LoadBalancingAlgorithm: opts.LoadBalancingAlgorithm,
	}
	if pool.LoadBalancingAlgorithm == config.LOAD_BALANCE_HB {
		pool.HashLookupTable = NewMaglev(opts.Logger)
		pool.HashRoutingProperties = &HashRoutingProperties{
			Header:        opts.HashHeader,
			BalanceFactor: opts.HashBalanceFactor,
		}
	}
	return pool
}

func PoolsMatch(p1, p2 *EndpointPool) bool {
	return p1.Host() == p2.Host() && p1.ContextPath() == p2.ContextPath()
}

func (p *EndpointPool) Host() string {
	return p.host
}

func (p *EndpointPool) ContextPath() string {
	return p.contextPath
}

func (p *EndpointPool) MaxConnsPerBackend() int64 {
	return p.maxConnsPerBackend
}

func (p *EndpointPool) LastUpdated() time.Time {
	return p.updatedAt
}

func (p *EndpointPool) Update() {
	p.updatedAt = time.Now()
}

func (p *EndpointPool) Put(endpoint *Endpoint) PoolPutResult {
	p.Lock()
	defer p.Unlock()

	var equal bool
	e, found := p.index[endpoint.CanonicalAddr()]
	if found {
		// Only calculate equal once, it's expensive.
		equal = e.endpoint.Equal(endpoint)
	}

	switch {
	case found && equal:
		// This is the most common case. The endpoint has not changed but was simply re-announced
		// to ensure gorouter is still aware of it.
		e.updated = time.Now()
		p.Update()

		return EndpointRefreshed

	case found && !e.endpoint.ModificationTag.SucceededBy(&endpoint.ModificationTag):
		// This exists to protect against flapping when a route receives a change (e.g. a new
		// route-service URL) and messages for the old and new config are still floating around.
		return EndpointUnmodified

	case found && !equal:
		// The same endpoint was announced with different data, replace the old endpoint with the
		// new one.
		e.Lock()
		defer e.Unlock()

		oldEndpoint := e.endpoint
		e.endpoint = endpoint

		if oldEndpoint.PrivateInstanceId != endpoint.PrivateInstanceId {
			delete(p.index, oldEndpoint.PrivateInstanceId)
			p.index[endpoint.PrivateInstanceId] = e
		}

		if oldEndpoint.ServerCertDomainSAN == endpoint.ServerCertDomainSAN {
			endpoint.SetRoundTripper(oldEndpoint.RoundTripper())
		}

		p.RouteSvcUrl = e.endpoint.RouteServiceUrl
		p.setPoolLoadBalancingAlgorithm(e.endpoint)
		// Route policy fields are pool-level: all backends of a route carry the
		// same policies (enforced by CAPI at registration time), so last-writer-wins
		// here is safe and keeps the pool in sync with the latest registration.
		p.routePolicyScope = endpoint.RoutePolicyScope
		p.routePolicies = endpoint.RoutePolicies
		e.updated = time.Now()
		if p.LoadBalancingAlgorithm == config.LOAD_BALANCE_HB {
			p.HashLookupTable.Add(e.endpoint.PrivateInstanceId)
		}
		p.Update()

		return EndpointUpdated

	case !found:
		// New endpoint.
		e = &endpointElem{
			endpoint:           endpoint,
			index:              len(p.endpoints),
			updated:            time.Now(),
			maxConnsPerBackend: p.maxConnsPerBackend,
		}
		p.endpoints = append(p.endpoints, e)

		p.index[endpoint.CanonicalAddr()] = e
		p.index[endpoint.PrivateInstanceId] = e

		p.RouteSvcUrl = e.endpoint.RouteServiceUrl
		p.setPoolLoadBalancingAlgorithm(e.endpoint)
		// Route policy fields are pool-level: all backends of a route carry the
		// same policies (enforced by CAPI at registration time), so last-writer-wins
		// here is safe and keeps the pool in sync with the latest registration.
		p.routePolicyScope = endpoint.RoutePolicyScope
		p.routePolicies = endpoint.RoutePolicies
		if p.LoadBalancingAlgorithm == config.LOAD_BALANCE_HB {
			p.HashLookupTable.Add(e.endpoint.PrivateInstanceId)
		}
		p.Update()

		return EndpointAdded

	default:
		panic("quantum state discovered")
	}
}

func (p *EndpointPool) RouteServiceUrl() string {
	p.Lock()
	defer p.Unlock()
	return p.RouteSvcUrl
}

func (p *EndpointPool) PruneEndpoints() []*Endpoint {
	p.Lock()
	defer p.Unlock()

	last := len(p.endpoints)
	now := time.Now()

	prunedEndpoints := []*Endpoint{}

	for i := 0; i < last; {
		e := p.endpoints[i]

		if e.endpoint.useTls {
			i++
			continue
		}

		staleTime := now.Add(-e.endpoint.StaleThreshold)

		if e.updated.Before(staleTime) {
			p.removeEndpoint(e)
			prunedEndpoints = append(prunedEndpoints, e.endpoint)
			last--
		} else {
			i++
		}
	}

	return prunedEndpoints
}

// Returns true if the endpoint was removed from the EndpointPool, false otherwise.
func (p *EndpointPool) Remove(endpoint *Endpoint) bool {
	var e *endpointElem

	p.Lock()
	defer p.Unlock()
	l := len(p.endpoints)
	if l > 0 {
		e = p.index[endpoint.CanonicalAddr()]
		if e != nil && e.endpoint.modificationTagSameOrNewer(endpoint) {
			p.removeEndpoint(e)
			return true
		}
	}

	return false
}

func (p *EndpointPool) removeEndpoint(e *endpointElem) {
	i := e.index
	es := p.endpoints

	es = slices.Delete(es, i, i+1)
	for j := i; j < len(es); j++ {
		es[j].index = j
	}
	p.endpoints = es

	delete(p.index, e.endpoint.CanonicalAddr())
	delete(p.index, e.endpoint.PrivateInstanceId)
	p.Update()

	if p.LoadBalancingAlgorithm == config.LOAD_BALANCE_HB {
		p.HashLookupTable.Remove(e.endpoint.PrivateInstanceId)
	}

}

func (p *EndpointPool) Endpoints(logger *slog.Logger, initial string, mustBeSticky bool, routingProps RoutingProperties) EndpointIterator {
	routingAlgorithm := p.LoadBalancingAlgorithm
	// Handle hash-based routing as special case
	if routingAlgorithm == config.LOAD_BALANCE_HB {
		// TODO: add VCAP-ID to logs after extracting handlers.VcapRequestIdHeader to new package "constants" (to avoid cyclic imports)
		headerValue := p.GetValidHashHeaderValue(routingProps.RequestHeaders, logger)
		if headerValue != "" {
			return NewHashBased(logger, p, initial, mustBeSticky, headerValue)
		}
		routingAlgorithm = routingProps.GlobalRoutingAlgorithm
	}

	switch routingAlgorithm {
	case config.LOAD_BALANCE_LC:
		logger.Debug("endpoint-iterator-with-least-connection-lb-algo")
		return NewLeastConnection(logger, p, initial, mustBeSticky, routingProps.LocallyOptimistic, routingProps.AZ)
	case config.LOAD_BALANCE_RR:
		logger.Debug("endpoint-iterator-with-round-robin-lb-algo")
		return NewRoundRobin(logger, p, initial, mustBeSticky, routingProps.LocallyOptimistic, routingProps.AZ)
	default:
		logger.Error("invalid-pool-load-balancing-algorithm",
			slog.String("poolLBAlgorithm", routingAlgorithm),
			slog.String("Host", p.host),
			slog.String("Path", p.contextPath))
		logger.Debug("endpoint-iterator-with-round-robin-lb-algo")
		return NewRoundRobin(logger, p, initial, mustBeSticky, routingProps.LocallyOptimistic, routingProps.AZ)
	}
}

func (p *EndpointPool) GetValidHashHeaderValue(header *http.Header, logger *slog.Logger) string {
	if p.HashRoutingProperties == nil || p.HashRoutingProperties.Header == "" {
		logger.Error("hash-routing-properties-missing", slog.String("host", p.Host()))
		return ""
	}

	hashHeader := header.Get(p.HashRoutingProperties.Header)
	if hashHeader == "" {
		logger.Info("hash-based-routing-header-value-not-found",
			slog.String("Host", p.host),
			slog.String("Path", p.contextPath),
		)
		return ""
	}
	return hashHeader
}

func (p *EndpointPool) NumEndpoints() int {
	p.Lock()
	defer p.Unlock()
	return len(p.endpoints)
}

func (p *EndpointPool) findById(id string) *endpointElem {
	p.Lock()
	defer p.Unlock()
	return p.index[id]
}

// FindStickyEndpoint attempts to find and return a sticky session endpoint.
// The function returns nil if the sticky session endpoint is not found, or overloaded.
func (p *EndpointPool) FindStickyEndpoint(logger *slog.Logger, stickyEndpointID string, mustBeSticky bool) *Endpoint {
	if stickyEndpointID == "" {
		return nil
	}

	e := p.findById(stickyEndpointID)

	// Handle overloaded endpoint
	if e != nil && e.isOverloaded() {
		if mustBeSticky {
			logger.Debug("endpoint-overloaded-but-request-must-be-sticky", e.endpoint.ToLogData()...)
			return nil
		}
		e = nil
	}

	// Handle missing endpoint
	if e == nil {
		if mustBeSticky {
			logger.Debug("endpoint-missing-but-request-must-be-sticky", slog.String("requested-endpoint", stickyEndpointID))
			return nil
		}
		logger.Debug("endpoint-missing-choosing-alternate", slog.String("requested-endpoint", stickyEndpointID))
		return nil
	}

	// Return found endpoint
	e.RLock()
	defer e.RUnlock()
	return e.endpoint
}

func (p *EndpointPool) IsEmpty() bool {
	p.Lock()
	l := len(p.endpoints)
	p.Unlock()

	return l == 0
}

// RoutePolicyScope returns the route policy scope for this pool.
// All endpoints in a pool share the same route policy scope since they represent
// instances of the same application route registered with the same options.
// Returns empty string if enforcement is not active.
func (p *EndpointPool) RoutePolicyScope() string {
	p.Lock()
	defer p.Unlock()

	return p.routePolicyScope
}

// RoutePolicies returns the route policies for this pool.
// All endpoints in a pool share the same route policies.
// Returns nil if no policies are configured.
func (p *EndpointPool) RoutePolicies() []string {
	p.Lock()
	defer p.Unlock()

	return p.routePolicies
}

// ApplicationId returns the ApplicationId from the first endpoint in the pool.
// All endpoints in a pool should have the same ApplicationId.
func (p *EndpointPool) ApplicationId() string {
	p.Lock()
	defer p.Unlock()

	if len(p.endpoints) == 0 {
		return ""
	}

	return p.endpoints[0].endpoint.ApplicationId
}

func (p *EndpointPool) NextIndex() int {
	if p.NextIdx == -1 {
		p.NextIdx = p.random.Intn(len(p.endpoints))
	}

	next := p.NextIdx
	p.NextIdx++

	if p.NextIdx >= len(p.endpoints) {
		p.NextIdx = 0
	}

	return next
}

func (p *EndpointPool) IsOverloaded() bool {
	if p.IsEmpty() {
		return false
	}

	p.Lock()
	defer p.Unlock()
	if p.maxConnsPerBackend == 0 {
		return false
	}

	if p.maxConnsPerBackend > 0 {
		for _, e := range p.endpoints {
			if e.endpoint.Stats.NumberConnections.Count() < p.maxConnsPerBackend {
				return false
			}
		}
	}

	return true
}

func (p *EndpointPool) MarkUpdated(t time.Time) {
	p.Lock()
	defer p.Unlock()
	for _, e := range p.endpoints {
		e.updated = t
	}
}

func (p *EndpointPool) EndpointFailed(endpoint *Endpoint, err error) {
	p.Lock()
	defer p.Unlock()
	e := p.index[endpoint.CanonicalAddr()]
	if e == nil {
		return
	}
	logger := p.logger.With(slog.Group("route-endpoint", endpoint.ToLogData()...))
	if e.endpoint.useTls && fails.PrunableClassifiers.Classify(err) {
		logger.Error("prune-failed-endpoint")
		p.removeEndpoint(e)

		return
	}

	if fails.FailableClassifiers.Classify(err) {
		logger.Error("endpoint-marked-as-ineligible")
		e.failed()
		return
	}

}

func (p *EndpointPool) Each(f func(endpoint *Endpoint)) {
	p.Lock()
	for _, e := range p.endpoints {
		f(e.endpoint)
	}
	p.Unlock()
}

func (p *EndpointPool) MarshalJSON() ([]byte, error) {
	p.Lock()
	endpoints := make([]*Endpoint, 0, len(p.endpoints))
	for _, e := range p.endpoints {
		endpoints = append(endpoints, e.endpoint)
	}
	p.Unlock()

	return json.Marshal(endpoints)
}

// setPoolLoadBalancingAlgorithm overwrites the load balancing algorithm of a pool by that of a specified endpoint, if that is valid.
func (p *EndpointPool) setPoolLoadBalancingAlgorithm(endpoint *Endpoint) {
	if endpoint.LoadBalancingAlgorithm == "" {
		return
	}
	if endpoint.LoadBalancingAlgorithm != p.LoadBalancingAlgorithm {
		if config.IsLoadBalancingAlgorithmValid(endpoint.LoadBalancingAlgorithm) {
			previousAlgorithm := p.LoadBalancingAlgorithm
			p.LoadBalancingAlgorithm = endpoint.LoadBalancingAlgorithm
			p.logger.Debug("setting-pool-load-balancing-algorithm-to-that-of-an-endpoint",
				slog.String("endpointLBAlgorithm", endpoint.LoadBalancingAlgorithm),
				slog.String("poolLBAlgorithm", p.LoadBalancingAlgorithm))

			// Clean up hash-based routing state when switching away from HB
			if previousAlgorithm == config.LOAD_BALANCE_HB && p.LoadBalancingAlgorithm != config.LOAD_BALANCE_HB {
				p.HashLookupTable = nil
				p.HashRoutingProperties = nil
			}
		} else {
			p.logger.Error("invalid-endpoint-load-balancing-algorithm-provided-keeping-pool-lb-algo",
				slog.String("endpointLBAlgorithm", endpoint.LoadBalancingAlgorithm),
				slog.String("poolLBAlgorithm", p.LoadBalancingAlgorithm))
		}
	}
	p.prepareHashBasedRouting(endpoint)
}

func (p *EndpointPool) prepareHashBasedRouting(endpoint *Endpoint) {
	if p.LoadBalancingAlgorithm != config.LOAD_BALANCE_HB {
		return
	}
	if p.HashLookupTable == nil {
		logger := p.logger.With(slog.String("host", p.Host()))
		p.HashLookupTable = NewMaglev(logger)
	}

	newProps := &HashRoutingProperties{
		Header:        endpoint.HashHeaderName,
		BalanceFactor: endpoint.HashBalanceFactor,
	}

	if p.HashRoutingProperties == nil || !p.HashRoutingProperties.Equal(newProps) {
		p.HashRoutingProperties = newProps
	}
}

func (e *endpointElem) failed() {
	t := time.Now()
	e.failedAt = &t
}

func (e *endpointElem) isOverloaded() bool {
	if e.maxConnsPerBackend == 0 {
		return false
	}

	return e.endpoint.Stats.NumberConnections.Count() >= e.maxConnsPerBackend
}

func (e *Endpoint) MarshalJSON() ([]byte, error) {
	var jsonObj struct {
		Address                string            `json:"address"`
		AvailabilityZone       string            `json:"availability_zone"`
		Protocol               string            `json:"protocol"`
		TLS                    bool              `json:"tls"`
		TTL                    int               `json:"ttl"`
		RouteServiceUrl        string            `json:"route_service_url,omitempty"`
		Tags                   map[string]string `json:"tags"`
		IsolationSegment       string            `json:"isolation_segment,omitempty"`
		PrivateInstanceId      string            `json:"private_instance_id,omitempty"`
		ServerCertDomainSAN    string            `json:"server_cert_domain_san,omitempty"`
		LoadBalancingAlgorithm string            `json:"load_balancing_algorithm,omitempty"`
		HashHeader             string            `json:"hash_header,omitempty"`
		HashBalance            *float64          `json:"hash_balance,omitempty"` // omitempty on a float64 field will omit the field when the value is 0.0, to keep 0 use pointer of float64
		RoutePolicyScope       string            `json:"route_policy_scope,omitempty"`
		RoutePolicies          []string          `json:"route_policies,omitempty"`
	}

	jsonObj.Address = e.addr
	jsonObj.AvailabilityZone = e.AvailabilityZone
	jsonObj.Protocol = e.Protocol
	jsonObj.TLS = e.IsTLS()
	jsonObj.RouteServiceUrl = e.RouteServiceUrl
	jsonObj.TTL = int(e.StaleThreshold.Seconds())
	jsonObj.Tags = e.Tags
	jsonObj.IsolationSegment = e.IsolationSegment
	jsonObj.PrivateInstanceId = e.PrivateInstanceId
	jsonObj.ServerCertDomainSAN = e.ServerCertDomainSAN
	jsonObj.LoadBalancingAlgorithm = e.LoadBalancingAlgorithm
	jsonObj.HashHeader = e.HashHeaderName
	jsonObj.RoutePolicyScope = e.RoutePolicyScope
	jsonObj.RoutePolicies = e.RoutePolicies

	// marshal balance factor only if load balancing algorithm is hash-based
	if e.LoadBalancingAlgorithm == config.LOAD_BALANCE_HB {
		jsonObj.HashBalance = &e.HashBalanceFactor
	}
	return json.Marshal(jsonObj)
}

func (e *Endpoint) CanonicalAddr() string {
	return e.addr
}

func (e *Endpoint) Component() string {
	return e.Tags["component"]
}

func (e *Endpoint) ToLogData() []any {
	return []any{
		slog.String("ApplicationId", e.ApplicationId),
		slog.String("Addr", e.addr),
		slog.Any("Tags", e.Tags),
		slog.String("RouteServiceUrl", e.RouteServiceUrl),
		slog.String("AZ", e.AvailabilityZone),
	}
}

func (e *Endpoint) modificationTagSameOrNewer(other *Endpoint) bool {
	return e.ModificationTag == other.ModificationTag || e.ModificationTag.SucceededBy(&other.ModificationTag)
}
