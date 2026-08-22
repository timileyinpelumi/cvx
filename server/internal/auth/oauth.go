package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

const oauthStateTTL = 10 * time.Minute

// oauthRequestTimeout bounds the token exchange and userinfo fetch(es) for
// one callback, so a slow or hanging provider can't tie up the handler
// indefinitely.
const oauthRequestTimeout = 10 * time.Second

// oauthStateCookieName returns the short-lived cookie name tying an
// /auth/:provider/start redirect to its matching callback (CSRF protection
// for the OAuth flow). Scoped per provider so starting a Google flow and a
// GitHub flow in two tabs can't clobber each other's state cookie.
func oauthStateCookieName(provider string) string {
	return "cvx_oauth_state_" + provider
}

// errEmailNotVerified is returned by the provider userinfo fetchers when
// the account has no usable verified email (Google: email_verified=false;
// GitHub: no primary+verified address in /user/emails). AuthCallback treats
// it as a sign-in denial (redirect to /?error=forbidden), not a transient
// upstream failure.
var errEmailNotVerified = errors.New("oauth: no verified email")

// Provider wires one OAuth2 identity provider into cvx. Endpoint URLs
// (via Config.Endpoint) and the userinfo URL(s) below are plain fields —
// not hardcoded inside request logic — specifically so tests can point
// them at an httptest server instead of the real Google/GitHub endpoints.
type Provider struct {
	// Name is the URL path segment (also the value returned in
	// GET /auth/providers and the value stored as users.provider):
	// "google" or "github".
	Name string

	Config *oauth2.Config

	// UserInfoURL is fetched after token exchange to identify the user:
	// Google's OpenID Connect userinfo endpoint, or GitHub's /user.
	UserInfoURL string

	// EmailsURL is GitHub-only: GitHub's /user endpoint omits email unless
	// the account has a public one, so a second call to /user/emails finds
	// the primary verified address. Empty for providers that don't need it.
	EmailsURL string
}

// NewGoogleProvider builds the Google provider against the real Google
// OAuth2 endpoints.
func NewGoogleProvider(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		Name: "google",
		Config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
		UserInfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
	}
}

// NewGitHubProvider builds the GitHub provider against the real GitHub
// OAuth2 endpoints.
func NewGitHubProvider(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		Name: "github",
		Config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		},
		UserInfoURL: "https://api.github.com/user",
		EmailsURL:   "https://api.github.com/user/emails",
	}
}

// ListProviders handles GET /auth/providers: the configured provider names,
// for the sign-in UI to render one button per provider. Open route (no auth
// required — the whole point is bootstrapping sign-in before any session
// exists).
func (a *Auth) ListProviders(c echo.Context) error {
	// Fixed order rather than map iteration order, so the UI's button order
	// is stable across requests.
	var names []string
	for _, name := range []string{"google", "github"} {
		if _, ok := a.Providers[name]; ok {
			names = append(names, name)
		}
	}
	if names == nil {
		names = []string{}
	}
	return c.JSON(http.StatusOK, map[string][]string{"providers": names})
}

// AuthStart handles GET /auth/:provider/start: generates a random state,
// stashes it in a short-lived cookie, and redirects to the provider's
// consent screen.
func (a *Auth) AuthStart(c echo.Context) error {
	p, ok := a.Providers[c.Param("provider")]
	if !ok {
		return errJSON(c, http.StatusNotFound, "unknown provider")
	}

	state, err := randomState()
	if err != nil {
		slog.Error("oauth state generation failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, "internal error")
	}
	c.SetCookie(&http.Cookie{
		Name:     oauthStateCookieName(p.Name),
		Value:    state,
		Path:     "/",
		MaxAge:   int(oauthStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return c.Redirect(http.StatusFound, p.Config.AuthCodeURL(state))
}

// AuthCallback handles GET /auth/:provider/callback: verifies the OAuth
// state, exchanges the code for a token, fetches the provider's userinfo,
// enforces the email allowlist (if configured), upserts the user, and sets
// the session cookie.
func (a *Auth) AuthCallback(c echo.Context) error {
	providerName := c.Param("provider")

	// The state cookie is one-shot: read it, then clear it immediately —
	// before any call below writes the response. A header set after
	// WriteHeader has run is silently dropped by net/http (this isn't a
	// test-only quirk), so clearing must happen up front, not via defer, or
	// every error path below would leave the stale cookie behind.
	stateCookie, stateCookieErr := c.Cookie(oauthStateCookieName(providerName))
	a.clearCookie(c, oauthStateCookieName(providerName))

	p, ok := a.Providers[providerName]
	if !ok {
		return errJSON(c, http.StatusNotFound, "unknown provider")
	}

	if stateCookieErr != nil || stateCookie.Value == "" || stateCookie.Value != c.QueryParam("state") {
		return errJSON(c, http.StatusBadRequest, "invalid oauth state")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), oauthRequestTimeout)
	defer cancel()

	token, err := p.Config.Exchange(ctx, c.QueryParam("code"))
	if err != nil {
		slog.Error("oauth exchange failed", "provider", p.Name, "err", err)
		return errJSON(c, http.StatusBadGateway, "oauth exchange failed")
	}

	client := p.Config.Client(ctx, token)
	providerID, email, name, err := fetchUserInfo(ctx, client, p)
	if err != nil {
		if errors.Is(err, errEmailNotVerified) {
			slog.Warn("oauth sign-in denied: no verified email", "provider", p.Name)
			return c.Redirect(http.StatusFound, "/?error=forbidden")
		}
		slog.Error("oauth userinfo failed", "provider", p.Name, "err", err)
		return errJSON(c, http.StatusBadGateway, "oauth userinfo failed")
	}

	if len(a.AllowedEmails) > 0 && !a.AllowedEmails[strings.ToLower(email)] {
		slog.Warn("oauth sign-in denied by allowlist", "provider", p.Name, "email", email)
		return c.Redirect(http.StatusFound, "/?error=forbidden")
	}

	u, created, err := a.Store.UpsertUser(p.Name, providerID, email, name)
	if err != nil {
		slog.Error("upsert user failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, "internal error")
	}
	if a.OnSignin != nil {
		a.OnSignin(u.ID, p.Name, created)
	}
	if created && a.OnSignup != nil {
		// Off the request path: a slow mail provider must not sit between
		// someone and the app they just signed in to.
		go a.OnSignup(u.Email, u.Name)
	}

	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    Sign(u.ID, SessionTTL, a.SessionSecret),
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return c.Redirect(http.StatusFound, "/")
}

func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// fetchUserInfo dispatches to the provider-specific userinfo shape.
func fetchUserInfo(ctx context.Context, client *http.Client, p *Provider) (providerID, email, name string, err error) {
	if p.Name == "github" {
		return fetchGitHubUser(ctx, client, p)
	}
	return fetchGoogleUser(ctx, client, p)
}

func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// fetchGoogleUser refuses sign-in (errEmailNotVerified) when
// email_verified is false — an unverified email is not proof of account
// ownership, so it must never be trusted as the user's identity.
func fetchGoogleUser(ctx context.Context, client *http.Client, p *Provider) (string, string, string, error) {
	var body struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := getJSON(ctx, client, p.UserInfoURL, &body); err != nil {
		return "", "", "", err
	}
	if !body.EmailVerified {
		return "", "", "", errEmailNotVerified
	}
	return body.Sub, body.Email, body.Name, nil
}

// fetchGitHubUser never trusts the (optional, unverified-by-GitHub) public
// email on /user — it always resolves the email via /user/emails and only
// accepts the address marked both primary and verified, refusing sign-in
// (errEmailNotVerified) when no such address exists.
func fetchGitHubUser(ctx context.Context, client *http.Client, p *Provider) (string, string, string, error) {
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := getJSON(ctx, client, p.UserInfoURL, &user); err != nil {
		return "", "", "", err
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := getJSON(ctx, client, p.EmailsURL, &emails); err != nil {
		return "", "", "", err
	}
	var email string
	for _, e := range emails {
		if e.Primary && e.Verified {
			email = e.Email
			break
		}
	}
	if email == "" {
		return "", "", "", errEmailNotVerified
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}
	return strconv.FormatInt(user.ID, 10), email, name, nil
}
