package main

import (
	"bufio"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"cvx/internal/ai"
	"cvx/internal/auth"
	"cvx/internal/httpapi"
	"cvx/internal/mail"
	"cvx/internal/model"
	"cvx/internal/store"
)

// loadDotEnv sets environment variables from KEY=VALUE lines in path,
// skipping blank lines and #-comments. It never overrides a variable that is
// already set, so real deployment env always wins over the .env file.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, set := os.LookupEnv(key); set {
			continue
		}
		os.Setenv(key, strings.TrimSpace(value))
	}
}

// splitCommaEnv splits a comma-separated env var into trimmed, non-empty
// parts, so CVX_ALLOWED_EMAILS="" or unset produces nil (no restriction)
// rather than a slice containing one empty string.
func splitCommaEnv(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	loadDotEnv(".env")

	// Production guard: dev auto-auth must never run where real traffic can
	// reach it.
	production := os.Getenv("CVX_ENV") == "production"
	if production && os.Getenv("CVX_DEV_USER") != "" {
		slog.Error("CVX_DEV_USER is set with CVX_ENV=production; refusing to start")
		os.Exit(1)
	}

	if err := os.MkdirAll("data", 0o755); err != nil {
		slog.Error("mkdir data", "err", err)
		os.Exit(1)
	}
	st, err := store.Open(filepath.Join("data", "cvx.db"))
	if err != nil {
		slog.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	llm, llmDesc, err := ai.NewFromEnv()
	if err != nil {
		slog.Error("llm", "err", err)
		os.Exit(1)
	}
	// Every LLM call is memoized in SQLite (14-day TTL) keyed on the full
	// request, so identical work across all surfaces never hits the provider
	// twice. cvxeval builds its own raw LLM and stays uncached on purpose.
	st.PruneLLMCache()
	llm = ai.NewCachedLLM(llm, llmDesc, st.GetLLMCache, st.PutLLMCache)
	slog.Info("llm", "provider", llmDesc, "cache", "sqlite")

	// Auth is not optional (see docs/superpowers/plans/2026-08-02-cvx-v1.3.md
	// Global Constraints): CVX_DEV_USER wins for local dev, otherwise OAuth
	// vars must be fully set, otherwise the server refuses to start.
	authGate, err := auth.New(auth.Config{
		Store:              st,
		DevUserEmail:       os.Getenv("CVX_DEV_USER"),
		SessionSecret:      os.Getenv("CVX_SESSION_SECRET"),
		BaseURL:            os.Getenv("CVX_BASE_URL"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		AllowedEmails:      splitCommaEnv(os.Getenv("CVX_ALLOWED_EMAILS")),
		SecureCookies:      production,
	})
	if err != nil {
		slog.Error("auth", "err", err)
		os.Exit(1)
	}
	if authGate.DevUserEmail != "" {
		slog.Info("auth", "mode", "dev", "user", authGate.DevUserEmail)
	} else {
		providers := make([]string, 0, len(authGate.Providers))
		for name := range authGate.Providers {
			providers = append(providers, name)
		}
		slog.Info("auth", "mode", "oauth", "providers", providers)
	}

	srv := &httpapi.Server{
		Store: st,
		LLM:   llm,
		Mail: func(to string, t model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error) {
			if coverPDF != nil {
				return mail.Send(to, t, pdf, filename, "", mail.Attachment{Filename: coverFilename, Content: coverPDF})
			}
			return mail.Send(to, t, pdf, filename, "")
		},
		RecruiterMail: func(to, subject string, paragraphs []string, closing string, name string, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error) {
			if coverPDF != nil {
				return mail.SendRecruiter(to, subject, paragraphs, closing, name, pdf, filename, "", mail.Attachment{Filename: coverFilename, Content: coverPDF})
			}
			return mail.SendRecruiter(to, subject, paragraphs, closing, name, pdf, filename, "")
		},
		Auth: authGate,
	}

	e := echo.New()
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:   true,
		LogURI:      true,
		LogStatus:   true,
		LogLatency:  true,
		LogError:    true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			attrs := []any{
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"latency_ms", v.Latency.Milliseconds(),
			}
			if v.Error != nil {
				attrs = append(attrs, "err", v.Error)
			}
			switch {
			case v.Status >= 500:
				slog.Error("request", attrs...)
			case v.Status >= 400:
				slog.Warn("request", attrs...)
			default:
				slog.Info("request", attrs...)
			}
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.GET("/healthz", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	srv.Register(e)

	addr := os.Getenv("CVX_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if err := e.Start(addr); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
