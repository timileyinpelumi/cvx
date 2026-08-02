package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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

// oauthStateCookieName is the short-lived cookie tying an /auth/:provider/
// start redirect to its matching callback (CSRF protection for the OAuth
// flow).
const oauthStateCookieName = "cvx_oauth_state"

const oauthStateTTL = 10 * time.Minute

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
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   int(oauthStateTTL.Seconds()),
		HttpOnly: true,
		// Secure intentionally omitted — cvx serves plain HTTP on localhost only.
		SameSite: http.SameSiteLaxMode,
	})
	return c.Redirect(http.StatusFound, p.Config.AuthCodeURL(state))
}

// AuthCallback handles GET /auth/:provider/callback: verifies the OAuth
// state, exchanges the code for a token, fetches the provider's userinfo,
// enforces the email allowlist (if configured), upserts the user, and sets
// the session cookie.
func (a *Auth) AuthCallback(c echo.Context) error {
	p, ok := a.Providers[c.Param("provider")]
	if !ok {
		return errJSON(c, http.StatusNotFound, "unknown provider")
	}

	stateCookie, err := c.Cookie(oauthStateCookieName)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != c.QueryParam("state") {
		return errJSON(c, http.StatusBadRequest, "invalid oauth state")
	}
	clearCookie(c, oauthStateCookieName)

	ctx := c.Request().Context()
	token, err := p.Config.Exchange(ctx, c.QueryParam("code"))
	if err != nil {
		slog.Error("oauth exchange failed", "provider", p.Name, "err", err)
		return errJSON(c, http.StatusBadGateway, "oauth exchange failed")
	}

	client := p.Config.Client(ctx, token)
	providerID, email, name, err := fetchUserInfo(ctx, client, p)
	if err != nil {
		slog.Error("oauth userinfo failed", "provider", p.Name, "err", err)
		return errJSON(c, http.StatusBadGateway, "oauth userinfo failed")
	}

	if len(a.AllowedEmails) > 0 && !a.AllowedEmails[strings.ToLower(email)] {
		slog.Warn("oauth sign-in denied by allowlist", "provider", p.Name, "email", email)
		c.Response().Header().Set(echo.HeaderLocation, "/?error=forbidden")
		return c.NoContent(http.StatusForbidden)
	}

	u, err := a.Store.UpsertUser(p.Name, providerID, email, name)
	if err != nil {
		slog.Error("upsert user failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, "internal error")
	}

	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    Sign(u.ID, SessionTTL, a.SessionSecret),
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		// Secure intentionally omitted — cvx serves plain HTTP on localhost only.
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

func fetchGoogleUser(ctx context.Context, client *http.Client, p *Provider) (string, string, string, error) {
	var body struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := getJSON(ctx, client, p.UserInfoURL, &body); err != nil {
		return "", "", "", err
	}
	return body.Sub, body.Email, body.Name, nil
}

func fetchGitHubUser(ctx context.Context, client *http.Client, p *Provider) (string, string, string, error) {
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := getJSON(ctx, client, p.UserInfoURL, &user); err != nil {
		return "", "", "", err
	}

	email := user.Email
	if email == "" && p.EmailsURL != "" {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := getJSON(ctx, client, p.EmailsURL, &emails); err != nil {
			return "", "", "", err
		}
		for _, e := range emails {
			if e.Primary && e.Verified {
				email = e.Email
				break
			}
		}
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}
	return strconv.FormatInt(user.ID, 10), email, name, nil
}
