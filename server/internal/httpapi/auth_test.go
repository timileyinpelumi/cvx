package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"cvx/internal/model"
)

// newAuthTestServer builds a Server with the passcode gate enabled, mirroring
// newTestServer's fakeLLM/no-op Mail wiring so auth tests only differ in the
// Auth field.
func newAuthTestServer(t *testing.T, passcode string) (*Server, *echo.Echo) {
	t.Helper()
	a, err := NewAuth(passcode)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Store: newStore(t),
		LLM:   fakeLLM{},
		Mail:  func(model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
		Auth:  a,
	}
	e := echo.New()
	s.Register(e)
	return s, e
}

func loginRequestBody(passcode string) *http.Request {
	body, _ := json.Marshal(loginRequest{Passcode: passcode})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return req
}

// TestNoPasscodeModeFullyOpen checks that with no Auth set (the default,
// matching CVX_PASSCODE unset/empty), /api/profile is reachable without any
// cookie and /api/login doesn't exist at all — the exact same 404 it would
// have produced before this feature existed.
func TestNoPasscodeModeFullyOpen(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 (no profile, not 401), got %d: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, loginRequestBody("anything"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for /api/login when passcode disabled, got %d: %s", rec.Code, rec.Body)
	}
}

func TestPasscodeModeBlocksUnauthenticatedRequests(t *testing.T) {
	_, e := newAuthTestServer(t, "letmein")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "unauthorized" {
		t.Fatalf(`want error "unauthorized", got %+v`, got)
	}
}

func TestLoginWrongPasscode(t *testing.T) {
	_, e := newAuthTestServer(t, "letmein")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, loginRequestBody("nope"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "wrong passcode" {
		t.Fatalf(`want error "wrong passcode", got %+v`, got)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("want no cookie set on wrong passcode, got %+v", cookies)
	}
}

// TestLoginRightPasscodeSetsCookieAndUnlocksAccess drives the full happy
// path: correct passcode sets a well-formed session cookie and 204s, and
// replaying that cookie on a guarded route clears the 401.
func TestLoginRightPasscodeSetsCookieAndUnlocksAccess(t *testing.T) {
	_, e := newAuthTestServer(t, "letmein")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, loginRequestBody("letmein"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want exactly one cookie, got %+v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != sessionCookieName || cookie.Value == "" {
		t.Fatalf("got cookie %+v", cookie)
	}
	if !cookie.HttpOnly {
		t.Fatal("want cookie HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("want SameSite=Lax, got %v", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Fatalf("want Path=/, got %q", cookie.Path)
	}
	if cookie.MaxAge != 2592000 {
		t.Fatalf("want Max-Age=2592000, got %d", cookie.MaxAge)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	req.AddCookie(cookie)
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("want access allowed with valid cookie, got 401: %s", rec.Body)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 (no profile seeded) once authenticated, got %d: %s", rec.Code, rec.Body)
	}
}

func TestMissingOrBadCookieUnauthorized(t *testing.T) {
	_, e := newAuthTestServer(t, "letmein")

	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bogus"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for bad cookie, got %d: %s", rec.Code, rec.Body)
	}
}

// TestHealthzOpenEvenWithPasscode registers /healthz the same way main.go
// does — directly on e, outside Server.Register — and checks the passcode
// gate never reaches it.
func TestHealthzOpenEvenWithPasscode(t *testing.T) {
	_, e := newAuthTestServer(t, "letmein")
	e.GET("/healthz", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
}
