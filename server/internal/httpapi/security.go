package httpapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	"cvx/internal/auth"
)

// Rate limits, per client IP. Two tiers, because the two kinds of request
// cost wildly different things: reading a list is a SQLite query, generating
// a resume is several LLM calls someone is paying for.
const (
	// generalRate covers reads and small writes: enough headroom that normal
	// use never notices, low enough that a script does.
	generalRate  = rate.Limit(3)
	generalBurst = 40

	// costlyRate covers everything that reaches an LLM or renders a PDF.
	costlyRate  = rate.Limit(0.2) // 12 a minute sustained
	costlyBurst = 8

	// authRate covers the sign-in endpoints, which are the ones worth
	// hammering if you are guessing at anything.
	authRate  = rate.Limit(0.5)
	authBurst = 10
)

// expiresIn is how long an idle client's bucket is remembered. Long enough
// that a burst cannot be reset by waiting a moment, short enough that the
// map does not grow without bound.
const expiresIn = 10 * time.Minute

// maxConcurrentPerUser bounds how many expensive requests one account can
// have in flight. A rate limit alone does not stop ten parallel generations
// from one impatient click-through, and every one of those is money.
const maxConcurrentPerUser = 2

// Security wires the transport-level protections: security headers, a body
// ceiling, and the two rate limiters. Applied by main before Register, so
// every route including auth is covered.
func Security(e *echo.Echo, production bool) {
	// Fly terminates TLS and forwards the client IP; without this every
	// request rate-limits against the proxy as a single client.
	e.IPExtractor = echo.ExtractIPFromXFFHeader()

	secure := middleware.SecureConfig{
		XSSProtection:      "1; mode=block",
		ContentTypeNosniff: "nosniff",
		XFrameOptions:      "SAMEORIGIN",
		ReferrerPolicy:     "strict-origin-when-cross-origin",
	}
	if production {
		secure.HSTSMaxAge = 31536000
		secure.HSTSExcludeSubdomains = false
		secure.ContentSecurityPolicy = "frame-ancestors 'self'"
	}
	e.Use(middleware.SecureWithConfig(secure))

	// A resume PDF is a few hundred KB; anything past this is not one.
	e.Use(middleware.BodyLimit("12M"))

	// One lenient limiter over everything, with the tighter ones layered on
	// the routes that spend money (see Register). Health checks are exempt so
	// a busy machine never fails its own probe.
	e.Use(rateLimiter(generalRate, generalBurst, func(c echo.Context) bool {
		return c.Path() == "/healthz"
	}))
}

// Costly registers the tighter limits on the routes that spend money, and
// the per-user concurrency gate. Called from Register so the path list lives
// next to the routes it names.
func costlyMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{
		rateLimiter(costlyRate, costlyBurst, nil),
		concurrencyGate(),
	}
}

func authMiddleware() echo.MiddlewareFunc {
	return rateLimiter(authRate, authBurst, nil)
}

func rateLimiter(limit rate.Limit, burst int, skip middleware.Skipper) echo.MiddlewareFunc {
	cfg := middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{Rate: limit, Burst: burst, ExpiresIn: expiresIn},
		),
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return errJSON(c, http.StatusTooManyRequests, "slow down a moment and try again")
		},
	}
	if skip != nil {
		cfg.Skipper = skip
	}
	return middleware.RateLimiterWithConfig(cfg)
}

// concurrencyGate refuses a request when the same account already has
// maxConcurrentPerUser expensive ones running. Unauthenticated requests fall
// through: the auth middleware rejects them anyway.
func concurrencyGate() echo.MiddlewareFunc {
	var mu sync.Mutex
	inFlight := map[int64]int{}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := auth.UserIDFromContext(c)
			if !ok {
				return next(c)
			}

			mu.Lock()
			if inFlight[userID] >= maxConcurrentPerUser {
				mu.Unlock()
				return errJSON(c, http.StatusTooManyRequests, "one at a time, this one is still running")
			}
			inFlight[userID]++
			mu.Unlock()

			defer func() {
				mu.Lock()
				if inFlight[userID] <= 1 {
					delete(inFlight, userID)
				} else {
					inFlight[userID]--
				}
				mu.Unlock()
			}()
			return next(c)
		}
	}
}
