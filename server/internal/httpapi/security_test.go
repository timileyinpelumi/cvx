package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSecurityHeadersAndBodyLimit(t *testing.T) {
	e := echo.New()
	Security(e, true)
	e.GET("/x", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(echo.HeaderXRealIP, "203.0.113.1")
	// The proxy in front terminates TLS and forwards this; HSTS is only
	// meaningful, and is only set, on a request that arrived over HTTPS.
	req.Header.Set(echo.HeaderXForwardedProto, "https")
	e.ServeHTTP(rec, req)

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("production responses must carry HSTS")
	}
}

// The health check is exempt: a busy machine must never fail its own probe
// and get restarted for it.
func TestRateLimiterSkipsHealthChecks(t *testing.T) {
	e := echo.New()
	Security(e, false)
	e.GET("/healthz", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })

	for i := 0; i < generalBurst*3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set(echo.HeaderXRealIP, "203.0.113.2")
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("health check %d was limited: %d", i, rec.Code)
		}
	}
}

func TestRateLimiterRefusesAFlood(t *testing.T) {
	e := echo.New()
	Security(e, false)
	e.GET("/x", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })

	limited := false
	for i := 0; i < generalBurst*3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(echo.HeaderXRealIP, "203.0.113.3")
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatalf("a flood of %d requests was never limited", generalBurst*3)
	}
}
