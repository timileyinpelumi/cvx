package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"cvx/internal/ai"
	"cvx/internal/auth"
	"cvx/internal/store"
)

// Recorder writes events. Every call is best effort: telemetry must never
// fail or slow the thing it is measuring, so errors are logged and dropped.
type Recorder struct {
	Store *store.Store
}

// Record appends one event, attributing it to the signed-in user when there
// is one.
func (r *Recorder) Record(c echo.Context, e store.Event) {
	if r == nil || r.Store == nil {
		return
	}
	if e.UserID == 0 && c != nil {
		if userID, ok := auth.UserIDFromContext(c); ok {
			e.UserID = userID
		}
	}
	if err := r.Store.RecordEvent(e); err != nil {
		slog.Warn("event not recorded", "kind", e.Kind, "err", err)
	}
}

// Done is the common shape: something happened, it took this long, and it
// either worked or it did not.
func (r *Recorder) Done(c echo.Context, kind, target string, started time.Time, err error, meta map[string]any) {
	e := store.Event{
		Kind:   kind,
		Target: target,
		MS:     time.Since(started).Milliseconds(),
		OK:     err == nil,
		Meta:   meta,
	}
	if err != nil {
		e.Detail = truncate(err.Error(), 300)
	}
	r.Record(c, e)
}

// slowRequest is the latency past which a successful request is worth
// keeping. Everything slower, and everything that failed, lands in the
// event store; the rest would just be noise at one row per page load.
const slowRequest = 3 * time.Second

// requestIDHeader is echoed back so a user can quote it and a log line can
// be found.
const requestIDHeader = "X-Request-Id"

// Telemetry logs every request with an id and a user, and persists the ones
// worth keeping. Container logs die with the container; the event store is
// on the volume.
func (r *Recorder) Telemetry() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			started := time.Now()
			id := c.Response().Header().Get(echo.HeaderXRequestID)
			if id == "" {
				id = randomID()
			}
			c.Response().Header().Set(requestIDHeader, id)
			c.Set(requestIDKey, id)

			err := next(c)
			status := c.Response().Status
			if err != nil {
				// Echo has not written the error yet at this point.
				if he, ok := err.(*echo.HTTPError); ok {
					status = he.Code
				} else {
					status = 500
				}
			}

			took := time.Since(started)
			userID, _ := auth.UserIDFromContext(c)

			attrs := []any{
				"id", id, "method", c.Request().Method, "path", c.Request().URL.Path,
				"status", status, "ms", took.Milliseconds(),
			}
			if userID != 0 {
				attrs = append(attrs, "user", userID)
			}
			switch {
			case status >= 500:
				slog.Error("request", attrs...)
			case status >= 400:
				slog.Warn("request", attrs...)
			default:
				slog.Info("request", attrs...)
			}

			if status >= 400 || took > slowRequest {
				detail := ""
				if err != nil {
					detail = truncate(err.Error(), 300)
				}
				r.Record(c, store.Event{
					UserID: userID,
					Kind:   store.EventRequest,
					Target: c.Request().Method + " " + routeOf(c),
					MS:     took.Milliseconds(),
					OK:     status < 400,
					Detail: detail,
					Meta:   map[string]any{"status": status, "id": id},
				})
			}
			return err
		}
	}
}

// UsageSink records what each model call consumed, for the cost panel.
func (r *Recorder) UsageSink() ai.UsageSink {
	return func(_ context.Context, u ai.Usage, ms int64, err error) {
		e := store.Event{
			Kind:   store.EventLLM,
			Target: u.Model,
			MS:     ms,
			OK:     err == nil,
			Meta: map[string]any{
				"provider": u.Provider,
				"prompt":   u.PromptTokens,
				"output":   u.OutputTokens,
				"tokens":   u.TotalTokens,
				"cost":     u.Cost(),
			},
		}
		if err != nil {
			e.Detail = truncate(err.Error(), 300)
		}
		// No echo.Context here: the sink is called from inside the client,
		// below the request. The event still carries the model and cost,
		// which is what the panel groups by.
		r.Record(nil, e)
	}
}

// routeOf prefers the registered pattern over the raw path, so a thousand
// generation ids do not become a thousand distinct rows.
func routeOf(c echo.Context) string {
	if p := c.Path(); p != "" {
		return p
	}
	return c.Request().URL.Path
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// requestIDKey is where the per-request id lives in the echo context, for
// handlers that want to log with it.
const requestIDKey = "cvx_request_id"

// randomID is short on purpose: it is quoted in a support message, not used
// as a key.
func randomID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// SignupEvent and SigninEvent are re-exported so main can record auth
// events without importing the store's vocabulary separately.
const (
	SignupEvent = store.EventSignup
	SigninEvent = store.EventSignin
)
