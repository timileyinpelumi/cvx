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

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	loadDotEnv(".env")

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
	slog.Info("llm", "provider", llmDesc)

	// CVX_PASSCODE unset/empty leaves auth nil, which is a complete no-op in
	// httpapi.Server.Register — existing localhost workflow is untouched.
	var auth *httpapi.Auth
	if passcode := os.Getenv("CVX_PASSCODE"); passcode != "" {
		auth, err = httpapi.NewAuth(passcode)
		if err != nil {
			slog.Error("auth", "err", err)
			os.Exit(1)
		}
		slog.Info("auth", "passcode", "enabled")
	}

	srv := &httpapi.Server{
		Store: st,
		LLM:   llm,
		Mail: func(t model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error) {
			if coverPDF != nil {
				return mail.Send(t, pdf, filename, "", mail.Attachment{Filename: coverFilename, Content: coverPDF})
			}
			return mail.Send(t, pdf, filename, "")
		},
		Auth: auth,
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
