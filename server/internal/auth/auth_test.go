package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"cvx/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "cvx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// echoWithGuardedRoute wires a as the /api middleware ahead of a probe route
// that reports the userID stashed in context, mirroring how httpapi.Server
// wires the real /api group.
func echoWithGuardedRoute(a *Auth) *echo.Echo {
	e := echo.New()
	api := e.Group("/api")
	api.Use(a.Middleware)
	api.GET("/probe", func(c echo.Context) error {
		id, _ := UserIDFromContext(c)
		return c.JSON(http.StatusOK, map[string]int64{"userID": id})
	})
	return e
}

func TestDevModeAutoAuthenticatesAndUpsertsUser(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, DevUserEmail: "dev@localhost"}
	e := echoWithGuardedRoute(a)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/probe", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["userID"] == 0 {
		t.Fatalf("want non-zero userID stashed in context, got %+v", got)
	}

	u, err := st.GetUser(got["userID"])
	if err != nil || u == nil {
		t.Fatalf("want dev user to exist in store, got %v, %v", u, err)
	}
	if u.Provider != "dev" || u.Email != "dev@localhost" {
		t.Fatalf("got %+v", u)
	}

	// A second request resolves the same user (upsert, not duplicate).
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/probe", nil))
	var got2 map[string]int64
	if err := json.Unmarshal(rec2.Body.Bytes(), &got2); err != nil {
		t.Fatal(err)
	}
	if got2["userID"] != got["userID"] {
		t.Fatalf("want same userID across requests, got %d then %d", got["userID"], got2["userID"])
	}
}

func TestOAuthModeMissingCookieUnauthorized(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, SessionSecret: "secret"}
	e := echoWithGuardedRoute(a)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/probe", nil))
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

func TestOAuthModeInvalidCookieUnauthorized(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, SessionSecret: "secret"}
	e := echoWithGuardedRoute(a)

	req := httptest.NewRequest(http.MethodGet, "/api/probe", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "bogus"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", rec.Code, rec.Body)
	}
}

func TestOAuthModeValidCookieAuthenticates(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, SessionSecret: "secret"}
	e := echoWithGuardedRoute(a)

	u, err := st.UpsertUser("google", "g-1", "a@e.com", "A")
	if err != nil {
		t.Fatal(err)
	}
	cookie := Sign(u.ID, SessionTTL, "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/probe", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["userID"] != u.ID {
		t.Fatalf("want userID %d, got %+v", u.ID, got)
	}
}

// newMeEchoServer mirrors httpapi.Server.Register's real wiring for /api/me
// and /api/logout: logout is registered outside the guarded group (F6 — a
// bad/expired cookie must still be clearable), me stays behind it.
func newMeEchoServer(a *Auth) *echo.Echo {
	e := echo.New()
	e.POST("/api/logout", a.Logout)
	api := e.Group("/api")
	api.Use(a.Middleware)
	api.GET("/me", a.GetMe)
	return e
}

func TestGetMeReturnsUserShape(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, DevUserEmail: "dev@localhost"}
	e := newMeEchoServer(a)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		ID       int64  `json:"id"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || got.Email != "dev@localhost" || got.Provider != "dev" {
		t.Fatalf("got %+v", got)
	}
}

func TestLogoutClearsCookieAnd204s(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, DevUserEmail: "dev@localhost"}
	e := newMeEchoServer(a)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/logout", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want exactly one cookie set (the clear), got %+v", cookies)
	}
	c := cookies[0]
	if c.Name != SessionCookieName {
		t.Fatalf("want cookie named %q, got %q", SessionCookieName, c.Name)
	}
	if c.Value != "" || c.MaxAge > 0 {
		t.Fatalf("want cleared cookie (empty value, non-positive MaxAge), got %+v", c)
	}
}

// TestLogoutWithNoCookieStill204s drives F6: logout must not require a
// valid (or any) session cookie — an OAuth-mode caller with a missing or
// already-expired cookie must still be able to clear it and get 204, not
// the 401 the guarded /api group would otherwise produce.
func TestLogoutWithNoCookieStill204s(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, SessionSecret: "secret"} // OAuth mode, no dev bypass
	e := newMeEchoServer(a)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/logout", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}
}

// TestLogoutWithBogusCookieStill204s is the same check with an
// invalid/tampered cookie attached, rather than no cookie at all.
func TestLogoutWithBogusCookieStill204s(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, SessionSecret: "secret"}
	e := newMeEchoServer(a)

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "bogus"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}
}
