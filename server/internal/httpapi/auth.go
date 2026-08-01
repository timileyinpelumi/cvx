package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

const (
	sessionCookieName = "cvx_session"
	sessionMaxAgeSecs = 30 * 24 * 60 * 60 // 30 days
)

// Auth is the optional passcode gate. A nil *Auth on Server is a complete
// no-op: Register skips both the /api/login route and the guarding
// middleware, so CVX_PASSCODE unset/empty means zero behavior change. When
// non-nil, the session token is generated once at startup and held only in
// memory, so restarting the server rotates it and forces re-login.
type Auth struct {
	passcode string
	token    string
}

// NewAuth builds the passcode gate for a non-empty passcode, generating a
// random 32-byte hex session token via crypto/rand.
func NewAuth(passcode string) (*Auth, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return &Auth{passcode: passcode, token: hex.EncodeToString(buf)}, nil
}

type loginRequest struct {
	Passcode string `json:"passcode"`
}

func (a *Auth) postLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return errJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if subtle.ConstantTimeCompare([]byte(req.Passcode), []byte(a.passcode)) != 1 {
		slog.Warn("login failed")
		return errJSON(c, http.StatusUnauthorized, "wrong passcode")
	}
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    a.token,
		Path:     "/",
		MaxAge:   sessionMaxAgeSecs,
		HttpOnly: true,
		// Secure intentionally omitted — cvx serves plain HTTP on localhost only.
		SameSite: http.SameSiteLaxMode,
	})
	return c.NoContent(http.StatusNoContent)
}

// middleware guards the routes it wraps: a missing or mismatched session
// cookie is rejected before the handler runs. Register applies it only to
// the /api group minus /api/login, leaving /healthz and /api/login open.
func (a *Auth) middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(a.token)) != 1 {
			return errJSON(c, http.StatusUnauthorized, "unauthorized")
		}
		return next(c)
	}
}
