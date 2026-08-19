package auth

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"cvx/internal/store"
)

// userIDContextKey is the echo.Context key the middleware stores the
// authenticated user's id under.
const userIDContextKey = "userID"

// Auth is the authentication gate for every /api/* route and the OAuth
// entrypoints under /auth/*. Exactly one of two modes is active for the
// lifetime of a process:
//
//   - Dev mode (DevUserEmail set): every request auto-authenticates as that
//     email, auto-creating it under the "dev" provider. Providers/
//     SessionSecret are unused. For local development and tests only.
//   - OAuth mode (DevUserEmail empty): requests must carry a valid signed
//     session cookie (set by the OAuth callback after sign-in).
//
// Providers holds only the providers with credentials configured; its
// entries carry endpoint/userinfo URLs as fields specifically so tests can
// substitute httptest servers instead of the real Google/GitHub endpoints.
type Auth struct {
	Store         *store.Store
	DevUserEmail  string
	SessionSecret string
	Providers     map[string]*Provider
	// AllowedEmails, when non-empty, restricts sign-in to these emails
	// (lowercased). Empty means any provider account may sign in.
	AllowedEmails map[string]bool
	// SecureCookies marks every cookie Secure (production behind HTTPS).
	SecureCookies bool
}

func errJSON(c echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

// UserIDFromContext returns the userID the Middleware stashed in c, if any.
func UserIDFromContext(c echo.Context) (int64, bool) {
	id, ok := c.Get(userIDContextKey).(int64)
	return id, ok
}

// Middleware authenticates every request it wraps, resolving a userID into
// context (see UserIDFromContext) or rejecting the request with 401. Dev
// mode always succeeds (auto-creating the dev user on first use); OAuth
// mode requires a valid, unexpired session cookie.
func (a *Auth) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if a.DevUserEmail != "" {
			u, err := a.Store.UpsertUser("dev", a.DevUserEmail, a.DevUserEmail, a.DevUserEmail)
			if err != nil {
				slog.Error("dev user upsert failed", "err", err)
				return errJSON(c, http.StatusInternalServerError, "internal error")
			}
			c.Set(userIDContextKey, u.ID)
			return next(c)
		}

		cookie, err := c.Cookie(SessionCookieName)
		if err != nil {
			return errJSON(c, http.StatusUnauthorized, "unauthorized")
		}
		userID, ok := Verify(cookie.Value, a.SessionSecret)
		if !ok {
			return errJSON(c, http.StatusUnauthorized, "unauthorized")
		}
		c.Set(userIDContextKey, userID)
		return next(c)
	}
}

// GetMe handles GET /api/me: the authenticated user's own identity.
func (a *Auth) GetMe(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	u, err := a.Store.GetUser(userID)
	if err != nil {
		slog.Error("get user failed", "err", err)
		return errJSON(c, http.StatusInternalServerError, "internal error")
	}
	if u == nil {
		return errJSON(c, http.StatusUnauthorized, "unauthorized")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"id":       u.ID,
		"email":    u.Email,
		"name":     u.Name,
		"provider": u.Provider,
	})
}

// Logout handles POST /api/logout: clears the session cookie unconditionally.
func (a *Auth) Logout(c echo.Context) error {
	a.clearCookie(c, SessionCookieName)
	return c.NoContent(http.StatusNoContent)
}

// clearCookie overwrites name with an immediately-expired, empty-valued
// cookie, matching the attributes it was set with so browsers actually
// clear it.
func (a *Auth) clearCookie(c echo.Context, name string) {
	c.SetCookie(&http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}
