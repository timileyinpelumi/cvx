package auth

import (
	"fmt"
	"strings"

	"cvx/internal/store"
)

// Config is the env-derived input to New. Exactly one of DevUserEmail or
// the OAuth fields must be usable — see New for the precedence rules.
type Config struct {
	Store *store.Store

	// DevUserEmail, when set, puts Auth in dev mode and wins over every
	// OAuth field below (CVX_DEV_USER).
	DevUserEmail string

	// OAuth mode fields.
	SessionSecret      string // CVX_SESSION_SECRET
	BaseURL            string // CVX_BASE_URL
	GoogleClientID     string // GOOGLE_CLIENT_ID
	GoogleClientSecret string // GOOGLE_CLIENT_SECRET
	GitHubClientID     string // GITHUB_CLIENT_ID
	GitHubClientSecret string // GITHUB_CLIENT_SECRET

	// SecureCookies marks every cookie Secure — required behind HTTPS in
	// production (CVX_ENV=production).
	SecureCookies bool

	// AllowedEmails restricts sign-in when non-empty (CVX_ALLOWED_EMAILS,
	// comma-separated). Matching is case-insensitive.
	AllowedEmails []string
}

// minSessionSecretLen is the minimum acceptable length for
// CVX_SESSION_SECRET: short enough to catch placeholder/typo values, well
// short of what HMAC-SHA256 actually needs (32 bytes matches the hash's
// output size).
const minSessionSecretLen = 32

// New builds Auth from Config, applying the auth-mode precedence documented
// on Auth: dev mode (DevUserEmail set) wins outright; otherwise at least one
// OAuth provider must have both its client id and secret set, alongside
// SessionSecret and BaseURL, or auth cannot be configured and New errors —
// per the plan, auth is not optional in any mode.
func New(cfg Config) (*Auth, error) {
	a := &Auth{
		Store:         cfg.Store,
		AllowedEmails: normalizeEmails(cfg.AllowedEmails),
		SecureCookies: cfg.SecureCookies,
	}

	if cfg.DevUserEmail != "" {
		a.DevUserEmail = cfg.DevUserEmail
		return a, nil
	}

	if cfg.GoogleClientID == "" && cfg.GitHubClientID == "" {
		return nil, fmt.Errorf("no auth configured: set CVX_DEV_USER for local dev, or OAuth credentials (GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET and/or GITHUB_CLIENT_ID/GITHUB_CLIENT_SECRET) plus CVX_BASE_URL and CVX_SESSION_SECRET")
	}
	if len(cfg.SessionSecret) < minSessionSecretLen {
		return nil, fmt.Errorf("CVX_SESSION_SECRET is required in OAuth mode and must be at least %d characters (got %d)", minSessionSecretLen, len(cfg.SessionSecret))
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("CVX_BASE_URL is required in OAuth mode")
	}

	a.SessionSecret = cfg.SessionSecret
	a.Providers = map[string]*Provider{}
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		a.Providers["google"] = NewGoogleProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.BaseURL+"/auth/google/callback")
	}
	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		a.Providers["github"] = NewGitHubProvider(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.BaseURL+"/auth/github/callback")
	}
	if len(a.Providers) == 0 {
		return nil, fmt.Errorf("no OAuth provider has both its client id and secret set (checked GOOGLE_CLIENT_ID/SECRET, GITHUB_CLIENT_ID/SECRET)")
	}

	return a, nil
}

func normalizeEmails(emails []string) map[string]bool {
	if len(emails) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, e := range emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			out[e] = true
		}
	}
	return out
}
