package httpapi

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"cvx/internal/auth"
	"cvx/internal/store"
)

// startedAt is process start, for the uptime the panel shows. A restart is
// the first thing worth knowing when something looks wrong.
var startedAt = time.Now()

// adminEmails is the allowlist, read once. Single-tenant deployment: a role
// column would be a schema for a set with one member in it.
var adminEmails = parseEmails(os.Getenv("CVX_ADMIN_EMAILS"))

func parseEmails(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if e := strings.ToLower(strings.TrimSpace(part)); e != "" {
			out[e] = true
		}
	}
	return out
}

// IsAdmin reports whether an email is on the allowlist. Exported so the
// auth layer can tell the UI whether to show the panel at all.
func IsAdmin(email string) bool {
	return len(adminEmails) > 0 && adminEmails[strings.ToLower(strings.TrimSpace(email))]
}

// requireAdmin gates the admin routes. Signed in is not enough: an account
// on the allowlist, or nothing. With no allowlist configured the panel is
// closed entirely, so forgetting to set it fails shut.
func (s *Server) requireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		userID, ok := auth.UserIDFromContext(c)
		if !ok {
			return errJSON(c, http.StatusUnauthorized, "unauthorized")
		}
		if len(adminEmails) == 0 {
			return errJSON(c, http.StatusForbidden, "the admin panel is not enabled")
		}
		u, err := s.Store.GetUser(userID)
		if err != nil || u == nil || !IsAdmin(u.Email) {
			return errJSON(c, http.StatusForbidden, "not an admin")
		}
		return next(c)
	}
}

// registerAdmin mounts the panel's API under the authenticated group.
func (s *Server) registerAdmin(api *echo.Group) {
	admin := api.Group("/admin", s.requireAdmin)
	admin.GET("/overview", s.adminOverview)
	admin.GET("/events", s.adminEvents)
	admin.GET("/users", s.adminUsers)
}

type adminOverview struct {
	Totals    store.Totals    `json:"totals"`
	Funnel    store.Funnel    `json:"funnel"`
	Activity  []store.Count   `json:"activity"`
	Daily     []store.Count   `json:"daily"`
	Usage     []store.Count   `json:"usage"`
	Failures  []store.Count   `json:"failures"`
	Health    adminHealth     `json:"health"`
	Window    int             `json:"windowDays"`
	Latency   []store.Latency `json:"latency"`
	Quality   store.Quality   `json:"quality"`
	Slowest   []store.Event   `json:"slowest"`
	Providers []store.Count   `json:"providers"`
	ErrorRate []store.Count   `json:"errorRate"`
}

// latencyKinds are the durations worth a percentile: the pipeline stages a
// person waits on, plus the request as a whole.
var latencyKinds = []string{
	store.EventGenerate, store.EventLLM, store.EventPreview,
	store.EventBulletRewrite, store.EventProfileUpload, store.EventRequest,
}

// adminHealth is what is true right now, as opposed to what has happened.
type adminHealth struct {
	UptimeSeconds int64  `json:"uptimeSeconds"`
	Goroutines    int    `json:"goroutines"`
	HeapBytes     uint64 `json:"heapBytes"`
	LLM           string `json:"llm"`
	MailEnabled   bool   `json:"mailEnabled"`
	OAuthEnabled  bool   `json:"oauthEnabled"`
	Production    bool   `json:"production"`
}

func (s *Server) adminOverview(c echo.Context) error {
	days := clampDays(c.QueryParam("days"), 14)
	since := time.Now().UTC().AddDate(0, 0, -days)

	totals, err := s.Store.AdminTotals()
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	funnel, err := s.Store.FunnelSince(since)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	activity, err := s.Store.CountsByKind(since)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	daily, err := s.Store.DailyCounts(store.EventGenerate, days)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	usage, err := s.Store.TokenUsage(since)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	failures, err := s.Store.FailureGroups(since, 15)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	latency, err := s.Store.LatencyByKind(since, latencyKinds)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	quality, err := s.Store.QualitySince(since)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	slowest, err := s.Store.SlowRequests(since, 8)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	providers, err := s.Store.LLMByProvider(since)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	errorRate, err := s.Store.ErrorRateByDay(days)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return c.JSON(http.StatusOK, adminOverview{
		Totals: totals, Funnel: funnel, Activity: activity,
		Daily: daily, Usage: usage, Failures: failures, Window: days,
		Latency: latency, Quality: quality, Slowest: slowest,
		Providers: providers, ErrorRate: errorRate,
		Health: adminHealth{
			UptimeSeconds: int64(time.Since(startedAt).Seconds()),
			Goroutines:    runtime.NumGoroutine(),
			HeapBytes:     mem.HeapAlloc,
			LLM:           s.LLMDescription,
			MailEnabled:   os.Getenv("RESEND_API_KEY") != "",
			OAuthEnabled:  os.Getenv("GOOGLE_CLIENT_ID") != "" || os.Getenv("GITHUB_CLIENT_ID") != "",
			Production:    os.Getenv("CVX_ENV") == "production",
		},
	})
}

func (s *Server) adminEvents(c echo.Context) error {
	f := store.EventFilter{
		Kind:    c.QueryParam("kind"),
		OnlyBad: c.QueryParam("failed") == "1",
		Limit:   atoiOr(c.QueryParam("limit"), 100),
	}
	if id := atoiOr(c.QueryParam("user"), 0); id > 0 {
		f.UserID = int64(id)
	}
	if days := atoiOr(c.QueryParam("days"), 0); days > 0 {
		f.Since = time.Now().UTC().AddDate(0, 0, -days)
	}

	events, err := s.Store.ListEvents(f)
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"events": events})
}

func (s *Server) adminUsers(c echo.Context) error {
	users, err := s.Store.ListAdminUsers(atoiOr(c.QueryParam("limit"), 100))
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"users": users})
}

func clampDays(raw string, fallback int) int {
	days := atoiOr(raw, fallback)
	if days < 1 || days > 90 {
		return fallback
	}
	return days
}

func atoiOr(raw string, fallback int) int {
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return fallback
}
