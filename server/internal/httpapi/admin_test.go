package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cvx/internal/store"
)

// The panel fails shut: with no allowlist configured, a signed-in user is
// still refused. Forgetting to set the env var must not open it.
func TestAdminIsClosedWithoutAnAllowlist(t *testing.T) {
	adminEmails = parseEmails("")
	_, e := newTestServer(t)

	for _, path := range []string{"/api/admin/overview", "/api/admin/events", "/api/admin/users"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: want 403, got %d", path, rec.Code)
		}
	}
}

// A signed-in account not on the list is refused too: authentication is not
// authorization.
func TestAdminRefusesNonAdmins(t *testing.T) {
	adminEmails = parseEmails("someone-else@example.com")
	t.Cleanup(func() { adminEmails = parseEmails("") })

	_, e := newTestServer(t)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body)
	}
}

func TestAdminOverviewReportsWhatHappened(t *testing.T) {
	// devAuth signs every request in as this address.
	adminEmails = parseEmails("dev@test.local")
	t.Cleanup(func() { adminEmails = parseEmails("") })

	s, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))
	e.ServeHTTP(httptest.NewRecorder(), generateRequestBody("Python Backend Engineer"))

	if err := s.Store.RecordEvent(store.Event{
		Kind: store.EventLLM, Target: "test-model", MS: 100, OK: true,
		Meta: map[string]any{"tokens": 1200, "cost": 0.05},
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/overview?days=7", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("overview: %d %s", rec.Code, rec.Body)
	}

	var got adminOverview
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.Users == 0 || got.Totals.DBBytes == 0 {
		t.Fatalf("totals empty: %+v", got.Totals)
	}
	if got.Funnel.Uploads == 0 || got.Funnel.Generations == 0 {
		t.Fatalf("the funnel missed real activity: %+v", got.Funnel)
	}
	if len(got.Usage) == 0 || got.Usage[0].Tokens != 1200 {
		t.Fatalf("usage: %+v", got.Usage)
	}
	if len(got.Daily) != 7 {
		t.Fatalf("want a bucket per day, got %d", len(got.Daily))
	}
	if got.Health.UptimeSeconds < 0 {
		t.Fatal("uptime should be reported")
	}
}

func TestAdminEventsFilters(t *testing.T) {
	adminEmails = parseEmails("dev@test.local")
	t.Cleanup(func() { adminEmails = parseEmails("") })

	s, e := newTestServer(t)
	for _, ev := range []store.Event{
		{Kind: store.EventGenerate, OK: true},
		{Kind: store.EventGenerate, OK: false, Detail: "boom"},
		{Kind: store.EventPreview, OK: true},
	} {
		if err := s.Store.RecordEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	get := func(query string) []store.Event {
		t.Helper()
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/events"+query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", query, rec.Code, rec.Body)
		}
		var body struct {
			Events []store.Event `json:"events"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Events
	}

	if got := get("?kind=generate"); len(got) != 2 {
		t.Fatalf("kind filter: %d", len(got))
	}
	if got := get("?failed=1"); len(got) != 1 || got[0].Detail != "boom" {
		t.Fatalf("failure filter: %+v", got)
	}
}
