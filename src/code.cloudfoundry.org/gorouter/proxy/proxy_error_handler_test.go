package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.cloudfoundry.org/gorouter/handlers/postselection"
)

func TestHandleReverseProxyError_AuthError_Writes403WithGenericMessage(t *testing.T) {
	rw := httptest.NewRecorder()
	authErr := postselection.NewAuthError("test:scope:rule", "caller not authorized")
	logger := slog.Default()

	handleReverseProxyError(logger, rw, authErr)

	if rw.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rw.Code)
	}
	body := rw.Body.String()
	if body != "Forbidden" {
		t.Errorf("expected body %q, got %q", "Forbidden", body)
	}
}

func TestHandleReverseProxyError_AuthErrorCustomStatus_WritesCorrectStatus(t *testing.T) {
	rw := httptest.NewRecorder()
	authErr := postselection.NewAuthErrorWithStatus("test:rule", "misdirected", http.StatusMisdirectedRequest)
	logger := slog.Default()

	handleReverseProxyError(logger, rw, authErr)

	if rw.Code != http.StatusMisdirectedRequest {
		t.Errorf("expected status 421, got %d", rw.Code)
	}
}

func TestHandleReverseProxyError_NonAuthError_Writes502WithGenericMessage(t *testing.T) {
	rw := httptest.NewRecorder()
	logger := slog.Default()

	handleReverseProxyError(logger, rw, http.ErrAbortHandler)

	if rw.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", rw.Code)
	}
	body := rw.Body.String()
	if body != "Bad Gateway" {
		t.Errorf("expected body %q, got %q", "Bad Gateway", body)
	}
}

func TestHandleReverseProxyError_NonAuthError_DoesNotLeakInternalMessage(t *testing.T) {
	rw := httptest.NewRecorder()
	logger := slog.Default()
	sensitiveErr := &sensitiveTestError{msg: "internal DB connection pool exhausted at 192.168.1.5:5432"}

	handleReverseProxyError(logger, rw, sensitiveErr)

	body := rw.Body.String()
	if body == sensitiveErr.Error() {
		t.Errorf("response body must not contain internal error message, got %q", body)
	}
}

type sensitiveTestError struct{ msg string }

func (e *sensitiveTestError) Error() string { return e.msg }
