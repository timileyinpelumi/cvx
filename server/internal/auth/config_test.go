package auth

import (
	"testing"
)

func TestNewDevModeWinsOverOAuthVars(t *testing.T) {
	st := newStore(t)
	a, err := New(Config{
		Store:              st,
		DevUserEmail:       "dev@localhost",
		GoogleClientID:     "id",
		GoogleClientSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.DevUserEmail != "dev@localhost" {
		t.Fatalf("want dev mode active, got %+v", a)
	}
	if len(a.Providers) != 0 {
		t.Fatalf("want no providers configured in dev mode, got %+v", a.Providers)
	}
}

func TestNewOAuthModeConfiguresOnlyProvidersWithCreds(t *testing.T) {
	st := newStore(t)
	a, err := New(Config{
		Store:              st,
		SessionSecret:      "secret",
		BaseURL:            "http://localhost:3000",
		GoogleClientID:     "id",
		GoogleClientSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Providers["google"]; !ok {
		t.Fatal("want google provider configured")
	}
	if _, ok := a.Providers["github"]; ok {
		t.Fatal("want github provider absent (no creds given)")
	}
	if got := a.Providers["google"].Config.RedirectURL; got != "http://localhost:3000/auth/google/callback" {
		t.Fatalf("want redirect URL derived from BaseURL, got %q", got)
	}
}

func TestNewErrorsWhenNothingConfigured(t *testing.T) {
	st := newStore(t)
	if _, err := New(Config{Store: st}); err == nil {
		t.Fatal("want error when neither dev user nor OAuth creds are set")
	}
}

func TestNewErrorsWhenOAuthModeMissingSessionSecret(t *testing.T) {
	st := newStore(t)
	_, err := New(Config{
		Store:              st,
		BaseURL:            "http://localhost:3000",
		GoogleClientID:     "id",
		GoogleClientSecret: "secret",
	})
	if err == nil {
		t.Fatal("want error when CVX_SESSION_SECRET is missing in OAuth mode")
	}
}

func TestNewNormalizesAllowedEmailsCaseInsensitively(t *testing.T) {
	st := newStore(t)
	a, err := New(Config{
		Store:              st,
		SessionSecret:      "secret",
		BaseURL:            "http://localhost:3000",
		GoogleClientID:     "id",
		GoogleClientSecret: "secret",
		AllowedEmails:      []string{" Ada@Example.com ", "bob@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !a.AllowedEmails["ada@example.com"] || !a.AllowedEmails["bob@example.com"] {
		t.Fatalf("want normalized lowercase emails, got %+v", a.AllowedEmails)
	}
}
