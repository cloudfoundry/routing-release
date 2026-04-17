package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	gouuid "github.com/nu7hatch/gouuid"
	"github.com/openzipkin/zipkin-go/idgenerator"
	"github.com/openzipkin/zipkin-go/model"
	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/common/uuid"
	"code.cloudfoundry.org/gorouter/proxy/utils"
	"code.cloudfoundry.org/gorouter/route"
)

type key string

const RequestInfoCtxKey key = "RequestInfo"

// TLSConnStateKey is the context key type for TLSConnState.
// Exported so router.go can retrieve the pointer during the TLS handshake.
type TLSConnStateKey struct{}

// TLSConnState captures per-connection TLS handshake state.
// It is stored in a connection-scoped context via http.Server.ConnContext
// (set by router.go) and retrieved per-request in authorization handlers.
type TLSConnState struct {
	// SNI is the Server Name Indication value from the TLS ClientHello.
	SNI string
	// MtlsDomain is the matched mTLS domain name (empty if none matched).
	MtlsDomain string
	// ClientCertRequired is true when GoRouter required and validated a client
	// certificate during the TLS handshake for this connection.
	ClientCertRequired bool
}

// SetTLSConnState stores the TLSConnState in a context (for use in ConnContext).
func SetTLSConnState(ctx context.Context, state *TLSConnState) context.Context {
	return context.WithValue(ctx, TLSConnStateKey{}, state)
}

// GetTLSConnectionState retrieves the TLSConnState from the request context.
// Returns a zero-value TLSConnState (not nil) if none was set (e.g. plain HTTP).
func GetTLSConnectionState(r *http.Request) TLSConnState {
	if v := r.Context().Value(TLSConnStateKey{}); v != nil {
		if state, ok := v.(*TLSConnState); ok && state != nil {
			return *state
		}
	}
	return TLSConnState{}
}

type TraceInfo struct {
	TraceID string
	SpanID  string
	UUID    string
}

// RequestInfo stores all metadata about the request and is used to pass
// information between handlers. The timing information is ordered by time of
// occurrence.
type RequestInfo struct {
	// ReceivedAt records the time at which this request was received by
	// gorouter as recorded in the RequestInfo middleware.
	ReceivedAt time.Time
	// AppRequestStartedAt records the time at which gorouter starts sending
	// the request to the backend.
	AppRequestStartedAt time.Time
	// LastFailedAttemptFinishedAt is the end of the last failed request,
	// if any. If there was at least one failed attempt this will be set, if
	// there was no successful attempt the RequestFailed flag will be set.
	LastFailedAttemptFinishedAt time.Time

	// These times document at which timestamps the individual phases of the
	// request started / finished if there was a successful attempt.
	DnsStartedAt           time.Time
	DnsFinishedAt          time.Time
	DialStartedAt          time.Time
	DialFinishedAt         time.Time
	TlsHandshakeStartedAt  time.Time
	TlsHandshakeFinishedAt time.Time

	// AppRequestFinishedAt records the time at which either a response was
	// received or the last performed attempt failed and no further attempts
	// could be made.
	AppRequestFinishedAt time.Time

	// FinishedAt is recorded once the access log middleware is executed after
	// performing the request, in contrast to the ReceivedAt value which is
	// recorded before the access log, but we need the value to be able to
	// produce the log.
	FinishedAt time.Time
	// GorouterTime is calculated in the reporter
	GorouterTime float64

	RoutePool                         *route.EndpointPool
	RouteEndpoint                     *route.Endpoint
	ProxyResponseWriter               utils.ProxyResponseWriter
	RouteServiceURL                   *url.URL
	ShouldRouteToInternalRouteService bool
	FailedAttempts                    int

	LocalAddress string

	// RoundTripSuccessful will be set once a request has successfully reached a backend instance.
	RoundTripSuccessful bool

	TraceInfo TraceInfo

	BackendReqHeaders http.Header

	// CallerIdentity contains the identity of the calling application extracted
	// from the client certificate. Will be nil for requests without identity.
	CallerIdentity *CallerIdentity

	// AuthResult captures the outcome of identity-aware routing authorization.
	// Will be nil if no authorization was performed.
	AuthResult *AuthResult

	// TlsSNI is the SNI value used during the TLS handshake (for RTR log on 421).
	TlsSNI string
}

func (r *RequestInfo) ProvideTraceInfo() (TraceInfo, error) {
	if r.TraceInfo != (TraceInfo{}) {
		return r.TraceInfo, nil
	}

	// use UUID as TraceID so that it can be used in VCAP_REQUEST_ID per RFC 4122
	guid, err := uuid.GenerateUUID()
	if err != nil {
		return TraceInfo{}, err
	}

	traceID, spanID, err := generateTraceAndSpanIDFromGUID(guid)
	if err != nil {
		return TraceInfo{}, err
	}

	r.TraceInfo = TraceInfo{
		UUID:    guid,
		TraceID: traceID,
		SpanID:  spanID,
	}

	return r.TraceInfo, nil
}

func (r *RequestInfo) SetTraceInfo(traceID, spanID string) error {
	if len(traceID) >= 20 {
		guid := traceID[0:8] + "-" + traceID[8:12] + "-" + traceID[12:16] + "-" + traceID[16:20] + "-" + traceID[20:]
		_, err := gouuid.ParseHex(guid)
		if err == nil {
			r.TraceInfo = TraceInfo{
				TraceID: traceID,
				SpanID:  spanID,
				UUID:    guid,
			}
			return nil
		}
	}

	guid, err := uuid.GenerateUUID()
	if err != nil {
		return err
	}

	r.TraceInfo = TraceInfo{
		TraceID: traceID,
		SpanID:  spanID,
		UUID:    guid,
	}
	return nil
}

func generateTraceAndSpanIDFromGUID(guid string) (string, string, error) {
	traceHex := strings.Replace(guid, "-", "", -1)
	traceID, err := model.TraceIDFromHex(traceHex)
	if err != nil {
		return "", "", err
	}
	spanID := idgenerator.NewRandom128().SpanID(traceID)
	return traceID.String(), spanID.String(), nil
}

func LoggerWithTraceInfo(l *slog.Logger, r *http.Request) *slog.Logger {
	reqInfo, err := ContextRequestInfo(r)
	if err != nil {
		return l
	}
	if reqInfo.TraceInfo.TraceID == "" {
		return l
	}

	return l.With(slog.String("trace-id", reqInfo.TraceInfo.TraceID), slog.String("span-id", reqInfo.TraceInfo.SpanID))
}

// ContextRequestInfo gets the RequestInfo from the request Context
func ContextRequestInfo(req *http.Request) (*RequestInfo, error) {
	return getRequestInfo(req.Context())
}

// RequestInfoHandler adds a RequestInfo to the context of all requests that go
// through this handler
type RequestInfoHandler struct{}

// NewRequestInfo creates a RequestInfoHandler
func NewRequestInfo() negroni.Handler {
	return &RequestInfoHandler{}
}

func (r *RequestInfoHandler) ServeHTTP(w http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
	reqInfo := new(RequestInfo)
	req = req.WithContext(context.WithValue(req.Context(), RequestInfoCtxKey, reqInfo))
	reqInfo.ReceivedAt = time.Now()
	next(w, req)
}

func GetEndpoint(ctx context.Context) (*route.Endpoint, error) {
	reqInfo, err := getRequestInfo(ctx)
	if err != nil {
		return nil, err
	}
	ep := reqInfo.RouteEndpoint
	if ep == nil {
		return nil, errors.New("route endpoint not set on request info")
	}
	return ep, nil
}

func getRequestInfo(ctx context.Context) (*RequestInfo, error) {
	ri := ctx.Value(RequestInfoCtxKey)
	if ri == nil {
		return nil, errors.New("RequestInfo not set on context")
	}
	reqInfo, ok := ri.(*RequestInfo)
	if !ok {
		return nil, errors.New("RequestInfo is not the correct type") // untested
	}
	return reqInfo, nil
}
