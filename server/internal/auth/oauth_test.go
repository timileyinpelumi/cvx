package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

// fakeTokenResponse is what a fake OAuth token endpoint returns for any
// exchange request — cvx doesn't care about token contents, only that it
// can be used to authorize the subsequent userinfo call.
const fakeTokenResponse = `{"access_token":"fake-access-token","token_type":"bearer","expires_in":3600}`

// newFakeGoogleServer stands up an httptest server that answers both the
// OAuth token endpoint and the Google-shaped userinfo endpoint, so
// Provider's Config.Endpoint and UserInfoURL can point at it instead of the
// real Google endpoints.
func newFakeGoogleServer(t *testing.T, sub, email string, emailVerified bool, name string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fakeTokenResponse))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); !strings.Contains(auth, "fake-access-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"sub": sub, "email": email, "email_verified": emailVerified, "name": name,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func googleProviderAgainst(srv *httptest.Server) *Provider {
	return &Provider{
		Name: "google",
		Config: &oauth2.Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURL:  "http://localhost:3000/auth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  srv.URL + "/authorize",
				TokenURL: srv.URL + "/token",
			},
		},
		UserInfoURL: srv.URL + "/userinfo",
	}
}

// newFakeGitHubServer stands up an httptest server answering the token
// endpoint plus GitHub's two-call userinfo shape: /user (whose email field
// is never trusted — see F2 in the security review) and /user/emails (the
// sole source of truth for a primary+verified address).
func newFakeGitHubServer(t *testing.T, id int64, login, name, publicEmail string, emails []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fakeTokenResponse))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": id, "login": login, "name": name, "email": publicEmail})
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(emails)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func githubProviderAgainst(srv *httptest.Server) *Provider {
	return &Provider{
		Name: "github",
		Config: &oauth2.Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURL:  "http://localhost:3000/auth/github/callback",
			Scopes:       []string{"read:user", "user:email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  srv.URL + "/authorize",
				TokenURL: srv.URL + "/token",
			},
		},
		UserInfoURL: srv.URL + "/user",
		EmailsURL:   srv.URL + "/user/emails",
	}
}

func newOAuthEchoServer(a *Auth) *echo.Echo {
	e := echo.New()
	e.GET("/auth/providers", a.ListProviders)
	e.GET("/auth/:provider/start", a.AuthStart)
	e.GET("/auth/:provider/callback", a.AuthCallback)
	return e
}

// startFlow drives GET /auth/:provider/start and returns the state value
// (parsed out of the redirect Location) and the state cookie the handler
// set, both needed to drive the matching callback request.
func startFlow(t *testing.T, e *echo.Echo, provider string) (state string, stateCookie *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/"+provider+"/start", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state = loc.Query().Get("state")
	if state == "" {
		t.Fatalf("want non-empty state in redirect Location %q", loc)
	}
	wantName := oauthStateCookieName(provider)
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == wantName {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		t.Fatalf("want %s cookie set, got %+v", wantName, cookies)
	}
	return state, stateCookie
}

func TestOAuthStartRedirectsWithStateCookie(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "a@e.com", true, "A")
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"google": googleProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	state, cookie := startFlow(t, e, "google")
	if state == "" || cookie.Value != state {
		t.Fatalf("want cookie value to match state, got state=%q cookie=%+v", state, cookie)
	}
	if !cookie.HttpOnly {
		t.Fatal("want state cookie HttpOnly")
	}
	if cookie.Name != "cvx_oauth_state_google" {
		t.Fatalf("want provider-scoped cookie name, got %q", cookie.Name)
	}
}

func TestOAuthStartUnknownProvider404(t *testing.T) {
	st := newStore(t)
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{}}
	e := newOAuthEchoServer(a)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/bogus/start", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body)
	}
}

func TestOAuthCallbackGoogleHappyPath(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "ada@example.com", true, "Ada Lovelace")
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"google": googleProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	state, stateCookie := startFlow(t, e, "google")

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=fake-code&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("want redirect to /, got %q", loc)
	}

	var sessionCookie, clearedStateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			sessionCookie = c
		}
		if c.Name == oauthStateCookieName("google") {
			clearedStateCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("want session cookie set, got %+v", rec.Result().Cookies())
	}
	if clearedStateCookie == nil || clearedStateCookie.Value != "" || clearedStateCookie.MaxAge > 0 {
		t.Fatalf("want state cookie cleared on successful callback, got %+v", clearedStateCookie)
	}
	userID, ok := Verify(sessionCookie.Value, "secret")
	if !ok {
		t.Fatal("want session cookie to verify")
	}

	u, err := st.GetUser(userID)
	if err != nil || u == nil {
		t.Fatalf("want user persisted, got %v, %v", u, err)
	}
	if u.Provider != "google" || u.ProviderID != "sub-1" || u.Email != "ada@example.com" || u.Name != "Ada Lovelace" {
		t.Fatalf("got %+v", u)
	}
}

// TestOAuthCallbackGoogleUnverifiedEmailDenied drives F3: Google's
// email_verified=false must refuse sign-in (redirect to /?error=forbidden),
// never trusting an unverified email as proof of identity.
func TestOAuthCallbackGoogleUnverifiedEmailDenied(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "unverified@example.com", false, "Unverified")
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"google": googleProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	state, stateCookie := startFlow(t, e, "google")
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=fake-code&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/?error=forbidden" {
		t.Fatalf("want Location /?error=forbidden, got %q", loc)
	}

	u, err := st.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("want no user created for unverified email, got %+v", u)
	}
}

// TestOAuthCallbackGitHubAlwaysUsesPrimaryVerifiedEmailIgnoringPublicEmail
// drives F2: even when /user carries a (GitHub-unverified) public email,
// cvx must ignore it entirely and resolve identity only via the
// primary+verified address from /user/emails.
func TestOAuthCallbackGitHubAlwaysUsesPrimaryVerifiedEmailIgnoringPublicEmail(t *testing.T) {
	st := newStore(t)
	emails := []map[string]any{
		{"email": "unverified@example.com", "primary": false, "verified": false},
		{"email": "secondary@example.com", "primary": false, "verified": true},
		{"email": "primary@example.com", "primary": true, "verified": true},
	}
	srv := newFakeGitHubServer(t, 555, "bob", "Bob Smith", "ignored-public@example.com", emails)
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"github": githubProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	state, stateCookie := startFlow(t, e, "github")
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=fake-code&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("want session cookie set")
	}
	userID, _ := Verify(sessionCookie.Value, "secret")
	u, err := st.GetUser(userID)
	if err != nil || u == nil {
		t.Fatalf("got %v, %v", u, err)
	}
	if u.ProviderID != "555" || u.Email != "primary@example.com" || u.Name != "Bob Smith" || u.Provider != "github" {
		t.Fatalf("want the primary+verified email (never the public one), got %+v", u)
	}
}

// TestOAuthCallbackGitHubNoVerifiedEmailDenied drives F2's rejection path:
// no primary+verified address anywhere in /user/emails must deny sign-in,
// not fall back to an unverified or public address.
func TestOAuthCallbackGitHubNoVerifiedEmailDenied(t *testing.T) {
	st := newStore(t)
	emails := []map[string]any{
		{"email": "unverified@example.com", "primary": true, "verified": false},
		{"email": "other@example.com", "primary": false, "verified": true},
	}
	srv := newFakeGitHubServer(t, 555, "bob", "Bob Smith", "public@example.com", emails)
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"github": githubProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	state, stateCookie := startFlow(t, e, "github")
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=fake-code&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/?error=forbidden" {
		t.Fatalf("want Location /?error=forbidden, got %q", loc)
	}

	u, err := st.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("want no user created without a verified email, got %+v", u)
	}
}

func TestOAuthCallbackStateMismatchBadRequest(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "a@e.com", true, "A")
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"google": googleProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	_, stateCookie := startFlow(t, e, "google")
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=fake-code&state=wrong-state", nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestOAuthCallbackMissingStateCookieBadRequest(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "a@e.com", true, "A")
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"google": googleProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=fake-code&state=anything", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

// TestOAuthCallbackAllowlistDeniesForbidden drives F1: a denial must be a
// browser-followable redirect (302 + Location), not a 403 with a Location
// header browsers won't act on.
func TestOAuthCallbackAllowlistDeniesForbidden(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "not-allowed@example.com", true, "Nope")
	a := &Auth{
		Store:         st,
		SessionSecret: "secret",
		Providers:     map[string]*Provider{"google": googleProviderAgainst(srv)},
		AllowedEmails: map[string]bool{"allowed@example.com": true},
	}
	e := newOAuthEchoServer(a)

	state, stateCookie := startFlow(t, e, "google")
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=fake-code&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/?error=forbidden" {
		t.Fatalf("want Location /?error=forbidden, got %q", loc)
	}

	// The denied account must not have been persisted as a user.
	rows, err := st.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	if rows != nil {
		t.Fatalf("want no user created for denied sign-in, got %+v", rows)
	}
}

func TestOAuthCallbackAllowlistAllowsListedEmail(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "allowed@example.com", true, "Allowed")
	a := &Auth{
		Store:         st,
		SessionSecret: "secret",
		Providers:     map[string]*Provider{"google": googleProviderAgainst(srv)},
		AllowedEmails: map[string]bool{"allowed@example.com": true},
	}
	e := newOAuthEchoServer(a)

	state, stateCookie := startFlow(t, e, "google")
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=fake-code&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}
}

// TestOAuthCallbackStateCookieClearedOnStateMismatch drives F7's "clear on
// every callback error return" half for the state-mismatch branch.
func TestOAuthCallbackStateCookieClearedOnStateMismatch(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "a@e.com", true, "A")
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"google": googleProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	_, stateCookie := startFlow(t, e, "google")
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=fake-code&state=wrong-state", nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == oauthStateCookieName("google") {
			cleared = c
		}
	}
	if cleared == nil || cleared.Value != "" || cleared.MaxAge > 0 {
		t.Fatalf("want state cookie cleared even on state-mismatch error, got %+v", cleared)
	}
}

func TestListProvidersOnlyConfigured(t *testing.T) {
	st := newStore(t)
	srv := newFakeGoogleServer(t, "sub-1", "a@e.com", true, "A")
	a := &Auth{Store: st, SessionSecret: "secret", Providers: map[string]*Provider{"google": googleProviderAgainst(srv)}}
	e := newOAuthEchoServer(a)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Providers []string `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 1 || got.Providers[0] != "google" {
		t.Fatalf("want [google], got %+v", got.Providers)
	}
}
