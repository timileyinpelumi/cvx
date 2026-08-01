package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

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
	loadDotEnv(".env")

	if err := os.MkdirAll("data", 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}
	st, err := store.Open(filepath.Join("data", "cvx.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	llm, llmDesc, err := ai.NewFromEnv()
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	log.Printf("llm: %s", llmDesc)

	srv := &httpapi.Server{
		Store: st,
		LLM:   llm,
		Mail: func(t model.Tailored, pdf []byte, filename string) (bool, error) {
			return mail.Send(t, pdf, filename, "")
		},
	}

	e := echo.New()
	e.GET("/healthz", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	srv.Register(e)

	addr := os.Getenv("CVX_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	e.Logger.Fatal(e.Start(addr))
}
