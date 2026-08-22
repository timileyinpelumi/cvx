package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cvx/internal/model"
)

func open(t *testing.T) *Store {
	s, err := Open(filepath.Join(t.TempDir(), "cvx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// testUser creates a distinct user via UpsertUser and returns its id, so
// store tests can exercise per-user scoping without hand-rolling users-table
// rows.
func testUser(t *testing.T, s *Store, provider, providerID string) int64 {
	t.Helper()
	u, _, err := s.UpsertUser(provider, providerID, providerID+"@example.com", providerID)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestProfileRoundTrip(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "u-1")
	if p, err := s.LoadProfile(userID); err != nil || p != nil {
		t.Fatalf("want nil,nil got %v,%v", p, err)
	}
	prof := model.Profile{Name: "Ada", Items: []model.Item{{Title: "Engineer"}}}
	model.AssignIDs(&prof)
	if err := s.SaveProfile(userID, prof); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProfile(userID, prof); err != nil { // upsert, not duplicate
		t.Fatal(err)
	}
	p, err := s.LoadProfile(userID)
	if err != nil || p.Name != "Ada" || p.Items[0].ID != "item-0" {
		t.Fatalf("got %+v, %v", p, err)
	}
}

// TestProfileUpsertIsPerUser checks that SaveProfile/LoadProfile are scoped
// by user_id: two users each get their own row, a repeat save for one user
// upserts in place (never creates a second row for that user, never touches
// the other user's row), and the table ends up with exactly one row per user.
func TestProfileUpsertIsPerUser(t *testing.T) {
	s := open(t)
	userA := testUser(t, s, "google", "a-1")
	userB := testUser(t, s, "google", "b-1")

	profA := model.Profile{Name: "Ada"}
	model.AssignIDs(&profA)
	if err := s.SaveProfile(userA, profA); err != nil {
		t.Fatal(err)
	}
	profB := model.Profile{Name: "Bob"}
	model.AssignIDs(&profB)
	if err := s.SaveProfile(userB, profB); err != nil {
		t.Fatal(err)
	}

	gotA, err := s.LoadProfile(userA)
	if err != nil || gotA == nil || gotA.Name != "Ada" {
		t.Fatalf("got %+v, %v", gotA, err)
	}
	gotB, err := s.LoadProfile(userB)
	if err != nil || gotB == nil || gotB.Name != "Bob" {
		t.Fatalf("got %+v, %v", gotB, err)
	}

	// Re-saving for userA upserts in place: userA's row updates, userB's is untouched.
	profA2 := model.Profile{Name: "Ada v2"}
	model.AssignIDs(&profA2)
	if err := s.SaveProfile(userA, profA2); err != nil {
		t.Fatal(err)
	}
	gotA2, err := s.LoadProfile(userA)
	if err != nil || gotA2 == nil || gotA2.Name != "Ada v2" {
		t.Fatalf("got %+v, %v", gotA2, err)
	}
	gotBAgain, err := s.LoadProfile(userB)
	if err != nil || gotBAgain == nil || gotBAgain.Name != "Bob" {
		t.Fatalf("userB profile changed unexpectedly: got %+v, %v", gotBAgain, err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM profile`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("want 2 profile rows (one per user), got %d", count)
	}
}

// TestLoadProfileAbsentForUserWithNoProfile checks that a user who has never
// saved a profile gets nil,nil, even once other users' profiles exist.
func TestLoadProfileAbsentForUserWithNoProfile(t *testing.T) {
	s := open(t)
	userA := testUser(t, s, "google", "a-1")
	userB := testUser(t, s, "google", "b-1")

	prof := model.Profile{Name: "Ada"}
	model.AssignIDs(&prof)
	if err := s.SaveProfile(userA, prof); err != nil {
		t.Fatal(err)
	}

	p, err := s.LoadProfile(userB)
	if err != nil || p != nil {
		t.Fatalf("want nil,nil for userB (no profile saved), got %v,%v", p, err)
	}
}

// TestGenerationsIsolatedPerUser checks that a generation saved by one user
// is completely invisible to another: absent from ListGenerations,
// GetGenerationPDF/GetGenerationCoverPDF return nil, and it doesn't factor
// into the other user's GapSummary.
func TestGenerationsIsolatedPerUser(t *testing.T) {
	s := open(t)
	userA := testUser(t, s, "google", "a-1")
	userB := testUser(t, s, "google", "b-1")

	ta := model.Tailored{TargetRole: "X", Gaps: []model.Gap{{Requirement: "Django", Severity: "missing"}}}
	metaA, err := s.SaveGeneration(userA, ta, []byte("pdf-a"), "a.pdf", []byte("cover-a"), "a-cover.pdf", "")
	if err != nil {
		t.Fatal(err)
	}

	listB, err := s.ListGenerations(userB)
	if err != nil || len(listB) != 0 {
		t.Fatalf("want empty list for userB, got %+v, %v", listB, err)
	}
	listA, err := s.ListGenerations(userA)
	if err != nil || len(listA) != 1 || listA[0].ID != metaA.ID {
		t.Fatalf("want 1 generation for userA, got %+v, %v", listA, err)
	}

	if pdf, fn, err := s.GetGenerationPDF(userB, metaA.ID); err != nil || pdf != nil || fn != "" {
		t.Fatalf("want nil pdf for userB accessing userA's generation, got %q %q %v", pdf, fn, err)
	}
	if pdf, fn, err := s.GetGenerationPDF(userA, metaA.ID); err != nil || string(pdf) != "pdf-a" || fn != "a.pdf" {
		t.Fatalf("want userA to see their own pdf, got %q %q %v", pdf, fn, err)
	}

	if pdf, fn, err := s.GetGenerationCoverPDF(userB, metaA.ID); err != nil || pdf != nil || fn != "" {
		t.Fatalf("want nil cover pdf for userB accessing userA's generation, got %q %q %v", pdf, fn, err)
	}

	trendsB, totalB, err := s.GapSummary(userB)
	if err != nil || totalB != 0 || len(trendsB) != 0 {
		t.Fatalf("want empty gap summary for userB, got total=%d trends=%+v err=%v", totalB, trendsB, err)
	}
	trendsA, totalA, err := s.GapSummary(userA)
	if err != nil || totalA != 1 {
		t.Fatalf("want total 1 for userA, got total=%d trends=%+v err=%v", totalA, trendsA, err)
	}
}

func TestGenerations(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "u-1")
	ta := model.Tailored{TargetRole: "Python Backend Engineer", Gaps: []model.Gap{{Requirement: "Django", Severity: "missing"}}, WhatChanged: []string{"x"}}
	a, err := s.SaveGeneration(userID, ta, []byte("pdf-a"), "a.pdf", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	b, _ := s.SaveGeneration(userID, ta, []byte("pdf-b"), "b.pdf", nil, "", "")
	list, err := s.ListGenerations(userID)
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("bad list: %+v %v", list, err)
	}
	if list[1].Gaps[0].Requirement != "Django" {
		t.Fatalf("gaps not persisted: %+v", list[1])
	}
	pdf, fn, err := s.GetGenerationPDF(userID, a.ID)
	if err != nil || string(pdf) != "pdf-a" || fn != "a.pdf" {
		t.Fatalf("got %q %q %v", pdf, fn, err)
	}
	if pdf, _, err := s.GetGenerationPDF(userID, "nope"); err != nil || pdf != nil {
		t.Fatalf("want nil,nil for unknown id, got %q %v", pdf, err)
	}
}

func TestGenerationsWithCoverLetter(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "u-1")
	ta := model.Tailored{TargetRole: "X"}

	withCover, err := s.SaveGeneration(userID, ta, []byte("pdf"), "x.pdf", []byte("cover-pdf"), "x-cover.pdf", "")
	if err != nil {
		t.Fatal(err)
	}
	if !withCover.HasCoverLetter {
		t.Fatalf("want HasCoverLetter true, got %+v", withCover)
	}
	pdf, fn, err := s.GetGenerationCoverPDF(userID, withCover.ID)
	if err != nil || string(pdf) != "cover-pdf" || fn != "x-cover.pdf" {
		t.Fatalf("got %q %q %v", pdf, fn, err)
	}

	time.Sleep(5 * time.Millisecond)
	withoutCover, err := s.SaveGeneration(userID, ta, []byte("pdf2"), "y.pdf", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if withoutCover.HasCoverLetter {
		t.Fatalf("want HasCoverLetter false, got %+v", withoutCover)
	}
	if pdf, fn, err := s.GetGenerationCoverPDF(userID, withoutCover.ID); err != nil || pdf != nil || fn != "" {
		t.Fatalf("want nil,\"\" for absent cover, got %q %q %v", pdf, fn, err)
	}
	if _, _, err := s.GetGenerationCoverPDF(userID, "nope"); err != nil {
		t.Fatalf("want nil error for unknown id, got %v", err)
	}

	list, err := s.ListGenerations(userID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]GenerationMeta{}
	for _, m := range list {
		byID[m.ID] = m
	}
	if !byID[withCover.ID].HasCoverLetter {
		t.Fatalf("list: want HasCoverLetter true for %s, got %+v", withCover.ID, byID[withCover.ID])
	}
	if byID[withoutCover.ID].HasCoverLetter {
		t.Fatalf("list: want HasCoverLetter false for %s, got %+v", withoutCover.ID, byID[withoutCover.ID])
	}
}

func TestGapSummaryGroupsCasingFiltersSingletonsAndTracksNewest(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "u-1")

	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }

	if _, err := s.SaveGeneration(userID, mk(model.Gap{Requirement: "Django", Evidence: "e1", Severity: "missing"}), []byte("p"), "a.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.SaveGeneration(userID, mk(
		model.Gap{Requirement: "django", Evidence: "e2", Severity: "weak"},
		model.Gap{Requirement: "Kubernetes", Evidence: "e-k", Severity: "missing"},
	), []byte("p"), "b.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.SaveGeneration(userID, mk(model.Gap{Requirement: "Django", Evidence: "e3-latest", Severity: "missing"}), []byte("p"), "c.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}

	trends, total, err := s.GapSummary(userID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("want total 3, got %d", total)
	}
	// Kubernetes was seen once, so it's filtered as noise; only Django groups.
	if len(trends) != 1 {
		t.Fatalf("want 1 trend, got %+v", trends)
	}
	tr := trends[0]
	if tr.Requirement != "Django" {
		t.Fatalf("want requirement %q, got %q", "Django", tr.Requirement)
	}
	if tr.Count != 3 || tr.Missing != 2 || tr.Weak != 1 {
		t.Fatalf("want count 3 missing 2 weak 1, got %+v", tr)
	}
	if tr.LastEvidence != "e3-latest" {
		t.Fatalf("want lastEvidence from newest generation, got %q", tr.LastEvidence)
	}
}

// TestGapSummaryDedupesWithinGeneration guards against counting gaps
// per-occurrence instead of per-generation: a single generation whose
// tailored output lists "Django" and "django " (same normalized key) must
// still contribute only 1 to Count, not 2.
func TestGapSummaryDedupesWithinGeneration(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "u-1")

	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }

	if _, err := s.SaveGeneration(userID, mk(
		model.Gap{Requirement: "Django", Evidence: "e1", Severity: "missing"},
		model.Gap{Requirement: "django ", Evidence: "e1-dup", Severity: "weak"},
	), []byte("p"), "a.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.SaveGeneration(userID, mk(model.Gap{Requirement: "Django", Evidence: "e2", Severity: "missing"}), []byte("p"), "b.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}

	trends, total, err := s.GapSummary(userID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("want total 2 generations, got %d", total)
	}
	if len(trends) != 1 {
		t.Fatalf("want 1 trend, got %+v", trends)
	}
	tr := trends[0]
	if tr.Count != 2 {
		t.Fatalf("want count 2 (one per generation, not per gap occurrence), got %d", tr.Count)
	}
	if tr.Missing != 2 || tr.Weak != 0 {
		t.Fatalf("want missing 2 weak 0 (first occurrence in gen 1 wins severity), got missing=%d weak=%d", tr.Missing, tr.Weak)
	}
}

func TestGapSummaryOrdering(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "u-1")

	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }

	// "Zeta" appears 3 times, "Alpha" appears 3 times (ties go alphabetical),
	// "Beta" appears 2 times: expect order Alpha, Zeta, Beta.
	for i := 0; i < 3; i++ {
		if _, err := s.SaveGeneration(userID, mk(model.Gap{Requirement: "Zeta", Evidence: "e", Severity: "missing"}), []byte("p"), "z.pdf", nil, "", ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.SaveGeneration(userID, mk(model.Gap{Requirement: "Alpha", Evidence: "e", Severity: "missing"}), []byte("p"), "a.pdf", nil, "", ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.SaveGeneration(userID, mk(model.Gap{Requirement: "Beta", Evidence: "e", Severity: "weak"}), []byte("p"), "b.pdf", nil, "", ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	trends, total, err := s.GapSummary(userID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 8 {
		t.Fatalf("want total 8, got %d", total)
	}
	if len(trends) != 3 {
		t.Fatalf("want 3 trends, got %+v", trends)
	}
	got := []string{trends[0].Requirement, trends[1].Requirement, trends[2].Requirement}
	want := []string{"Alpha", "Zeta", "Beta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want order %v, got %v", want, got)
		}
	}
}

func TestGapSummaryNoGenerations(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "u-1")
	trends, total, err := s.GapSummary(userID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(trends) != 0 {
		t.Fatalf("want 0,0, got %d,%+v", total, trends)
	}
}

// v1Schema is the original (pre-cover-letter) schema, captured verbatim so
// TestMigrationAddsCoverColumnsIdempotently can build a DB file that predates
// the cover_pdf/cover_filename columns, the way any real v1 install's
// data/cvx.db would look before an upgrade.
const v1Schema = `
CREATE TABLE IF NOT EXISTS profile (id INTEGER PRIMARY KEY CHECK (id = 1), json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS generations (
  id TEXT PRIMARY KEY, target_role TEXT NOT NULL, filename TEXT NOT NULL,
  created_at TEXT NOT NULL, tailored_json TEXT NOT NULL, pdf BLOB NOT NULL
);
`

func TestUpsertUserInsertsThenFetches(t *testing.T) {
	s := open(t)

	u, _, err := s.UpsertUser("google", "g-123", "ada@example.com", "Ada Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 || u.Provider != "google" || u.ProviderID != "g-123" || u.Email != "ada@example.com" || u.Name != "Ada Lovelace" {
		t.Fatalf("got %+v", u)
	}

	// Same (provider, providerID) fetches the same row rather than inserting
	// a duplicate, even if email/name are passed differently.
	again, _, err := s.UpsertUser("google", "g-123", "changed@example.com", "Changed Name")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != u.ID || again.Email != u.Email || again.Name != u.Name {
		t.Fatalf("want fetch of existing row unchanged, got %+v vs original %+v", again, u)
	}

	// A different provider (even with the same providerID) is a distinct user.
	other, _, err := s.UpsertUser("github", "g-123", "bob@example.com", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == u.ID {
		t.Fatalf("want distinct user for different provider, got same id %d", other.ID)
	}
}

func TestGetUserAbsentReturnsNilNil(t *testing.T) {
	s := open(t)
	u, err := s.GetUser(999)
	if err != nil || u != nil {
		t.Fatalf("want nil,nil got %v,%v", u, err)
	}
}

func TestGetUserReturnsInsertedUser(t *testing.T) {
	s := open(t)
	created, _, err := s.UpsertUser("google", "g-1", "a@e.com", "A")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetUser(created.ID)
	if err != nil || got == nil {
		t.Fatalf("got %v, %v", got, err)
	}
	if *got != created {
		t.Fatalf("want %+v, got %+v", created, *got)
	}
}

// TestUpsertUserAdoptsLegacyRowsOnFirstUserOnly seeds a profile and a
// generation the old-fashioned way (direct SQL, bypassing SaveProfile/
// SaveGeneration which now always stamp a user_id) so their user_id is NULL,
// simulating rows written before per-user auth existed. It then checks that
// creating the very first user adopts both rows, and that creating a second
// user afterwards does NOT re-adopt (the legacy rows stay with the first
// user).
func TestUpsertUserAdoptsLegacyRowsOnFirstUserOnly(t *testing.T) {
	s := open(t)

	if _, err := s.db.Exec(
		`INSERT INTO profile (json, updated_at) VALUES (?, ?)`,
		`{"name":"Legacy"}`, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO generations (id, target_role, filename, created_at, tailored_json, pdf) VALUES (?, ?, ?, ?, ?, ?)`,
		"legacy-1", "X", "x.pdf", time.Now().UTC().Format(time.RFC3339), `{"targetRole":"X"}`, []byte("pdf"),
	); err != nil {
		t.Fatal(err)
	}

	assertUserID := func(table string, want any) {
		t.Helper()
		var got sql.NullInt64
		if err := s.db.QueryRow(`SELECT user_id FROM ` + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		switch w := want.(type) {
		case nil:
			if got.Valid {
				t.Fatalf("%s: want NULL user_id, got %v", table, got.Int64)
			}
		case int64:
			if !got.Valid || got.Int64 != w {
				t.Fatalf("%s: want user_id %d, got %+v", table, w, got)
			}
		}
	}

	assertUserID("profile", nil)
	assertUserID("generations", nil)

	first, _, err := s.UpsertUser("google", "g-1", "first@example.com", "First")
	if err != nil {
		t.Fatal(err)
	}
	assertUserID("profile", first.ID)
	assertUserID("generations", first.ID)

	if _, _, err := s.UpsertUser("github", "gh-1", "second@example.com", "Second"); err != nil {
		t.Fatal(err)
	}
	// Legacy rows must still belong to the first user, not be re-adopted or
	// reassigned by the second user's creation.
	assertUserID("profile", first.ID)
	assertUserID("generations", first.ID)
}

func TestMigrationAddsUserIDColumnsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1-nouserid.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	// Drop back to a schema without user_id by recreating tables the pre-v1.3
	// way (schema already includes user_id going forward, so simulate an
	// older DB by using v1Schema plus the cover columns, still missing
	// user_id on both tables).
	if _, err := db.Exec(`DROP TABLE profile; DROP TABLE generations;`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE profile (id INTEGER PRIMARY KEY CHECK (id = 1), json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE generations (
  id TEXT PRIMARY KEY, target_role TEXT NOT NULL, filename TEXT NOT NULL,
  created_at TEXT NOT NULL, tailored_json TEXT NOT NULL, pdf BLOB NOT NULL,
  cover_pdf BLOB, cover_filename TEXT
);`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first reopen (migration) failed: %v", err)
	}
	if _, _, err := s1.UpsertUser("google", "g-1", "a@e.com", "A"); err != nil {
		t.Fatalf("upsert user after migration failed: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second reopen (idempotent migration) failed: %v", err)
	}
	defer s2.Close()
	if _, _, err := s2.UpsertUser("github", "gh-1", "b@e.com", "B"); err != nil {
		t.Fatalf("upsert user after second reopen failed: %v", err)
	}
}

// assertProfileRow checks the profile table has exactly one row and that
// its json/updated_at are byte-identical to want, used by
// TestMigrationPreservesExistingProfileRowThroughCheckRebuild to prove the
// CHECK(id=1) table-rebuild in migrateProfileDropSingleRowCheck neither
// drops nor mutates a pre-existing row's content.
func assertProfileRow(t *testing.T, s *Store, label, wantJSON, wantUpdatedAt string) {
	t.Helper()
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM profile`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("%s: want 1 profile row, got %d", label, count)
	}
	var gotJSON, gotUpdatedAt string
	if err := s.db.QueryRow(`SELECT json, updated_at FROM profile`).Scan(&gotJSON, &gotUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if gotJSON != wantJSON {
		t.Fatalf("%s: json changed by migration: want %q, got %q", label, wantJSON, gotJSON)
	}
	if gotUpdatedAt != wantUpdatedAt {
		t.Fatalf("%s: updated_at changed by migration: want %q, got %q", label, wantUpdatedAt, gotUpdatedAt)
	}
}

// TestMigrationPreservesExistingProfileRowThroughCheckRebuild builds a
// legacy v1-schema DB (profile still has CHECK (id = 1), the constraint
// migrateProfileDropSingleRowCheck exists to remove) with a real profile row
// already in it, then checks that Open() preserves that row exactly — same
// row count, same json, same updated_at — both right after the rebuild and
// again after a second Open() (proving the rebuild itself, not just the
// column-add migrations, is idempotent).
func TestMigrationPreservesExistingProfileRowThroughCheckRebuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-profile.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(v1Schema); err != nil {
		t.Fatal(err)
	}
	const wantJSON = `{"name":"Legacy Ada","items":[{"id":"item-0","title":"Engineer"}],"skills":["Go","Python"]}`
	const wantUpdatedAt = "2020-01-01T00:00:00Z"
	if _, err := db.Exec(
		`INSERT INTO profile (id, json, updated_at) VALUES (1, ?, ?)`,
		wantJSON, wantUpdatedAt,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first open (migration/rebuild) failed: %v", err)
	}
	assertProfileRow(t, s1, "after first Open (CHECK rebuild)", wantJSON, wantUpdatedAt)
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second open (idempotent no-op) failed: %v", err)
	}
	defer s2.Close()
	assertProfileRow(t, s2, "after second Open (idempotent no-op)", wantJSON, wantUpdatedAt)

	// The adopted legacy row must also be reachable through the normal Store
	// API once a user exists to own it.
	u, _, err := s2.UpsertUser("google", "g-1", "a@e.com", "A")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.LoadProfile(u.ID)
	if err != nil || got == nil || got.Name != "Legacy Ada" {
		t.Fatalf("want adopted legacy profile loadable via LoadProfile, got %+v, %v", got, err)
	}
}

func TestMigrationAddsCoverColumnsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(v1Schema); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first reopen (migration) failed: %v", err)
	}
	userID := testUser(t, s1, "google", "u-1")

	ta := model.Tailored{TargetRole: "X"}
	meta, err := s1.SaveGeneration(userID, ta, []byte("pdf"), "x.pdf", []byte("cover-pdf"), "x-cover.pdf", "")
	if err != nil {
		t.Fatalf("save using migrated columns failed: %v", err)
	}
	if !meta.HasCoverLetter {
		t.Fatalf("want HasCoverLetter true after migration, got %+v", meta)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second reopen (idempotent migration) failed: %v", err)
	}
	defer s2.Close()

	pdf, fn, err := s2.GetGenerationCoverPDF(userID, meta.ID)
	if err != nil || string(pdf) != "cover-pdf" || fn != "x-cover.pdf" {
		t.Fatalf("got %q %q %v", pdf, fn, err)
	}
}

func TestDeleteGenerationScopedPerUser(t *testing.T) {
	s := open(t)
	userA := testUser(t, s, "google", "a-1")
	userB := testUser(t, s, "google", "b-1")
	ta := model.Tailored{TargetRole: "X"}
	meta, err := s.SaveGeneration(userA, ta, []byte("pdf"), "a.pdf", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := s.DeleteGeneration(userB, meta.ID); err != nil || ok {
		t.Fatalf("other user's delete: want false,nil got %v,%v", ok, err)
	}
	if list, _ := s.ListGenerations(userA); len(list) != 1 {
		t.Fatalf("generation vanished after foreign delete: %d rows", len(list))
	}

	if ok, err := s.DeleteGeneration(userA, meta.ID); err != nil || !ok {
		t.Fatalf("owner delete: want true,nil got %v,%v", ok, err)
	}
	if ok, err := s.DeleteGeneration(userA, meta.ID); err != nil || ok {
		t.Fatalf("second delete: want false,nil got %v,%v", ok, err)
	}
	if list, _ := s.ListGenerations(userA); len(list) != 0 {
		t.Fatalf("want empty list after delete, got %d rows", len(list))
	}
}

func TestResumeStyleRoundTripPerUser(t *testing.T) {
	s := open(t)
	userA := testUser(t, s, "google", "a-1")
	userB := testUser(t, s, "google", "b-1")

	if raw, err := s.GetResumeStyle(userA); err != nil || raw != nil {
		t.Fatalf("unset: want nil,nil got %q,%v", raw, err)
	}
	if err := s.SaveResumeStyle(userA, []byte(`{"theme":"classic"}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveResumeStyle(userA, []byte(`{"theme":"compact"}`)); err != nil {
		t.Fatal(err)
	}
	raw, err := s.GetResumeStyle(userA)
	if err != nil || string(raw) != `{"theme":"compact"}` {
		t.Fatalf("got %q,%v", raw, err)
	}
	if raw, err := s.GetResumeStyle(userB); err != nil || raw != nil {
		t.Fatalf("userB must be unset: got %q,%v", raw, err)
	}
}

func TestLLMCacheRoundTripAndPrune(t *testing.T) {
	st := open(t)

	if _, ok := st.GetLLMCache("k1"); ok {
		t.Fatal("want miss on empty cache")
	}
	st.PutLLMCache("k1", []byte(`{"a":1}`))
	got, ok := st.GetLLMCache("k1")
	if !ok || string(got) != `{"a":1}` {
		t.Fatalf("want hit, got %q ok=%v", got, ok)
	}

	st.PutLLMCache("k1", []byte(`{"a":2}`))
	if got, _ := st.GetLLMCache("k1"); string(got) != `{"a":2}` {
		t.Fatalf("want overwrite, got %q", got)
	}

	// An expired row is a miss and prune removes it.
	old := time.Now().UTC().Add(-15 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := st.db.Exec(`UPDATE llm_cache SET created_at = ? WHERE key = 'k1'`, old); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.GetLLMCache("k1"); ok {
		t.Fatal("want expired row to miss")
	}
	st.PruneLLMCache()
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM llm_cache`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("want pruned cache, got %d rows (%v)", n, err)
	}
}

func TestSaveGenerationDedupesByFingerprint(t *testing.T) {
	s := open(t)
	userID := int64(1)
	ta := model.Tailored{TargetRole: "Backend Engineer"}
	fp := RoleFingerprint("https://jobs.example.com/backend#apply")

	first, err := s.SaveGeneration(userID, ta, []byte("pdf-1"), "a_1111.pdf", nil, "", fp)
	if err != nil {
		t.Fatal(err)
	}
	// Same link, fragment and trailing slash varied: still the same job.
	second, err := s.SaveGeneration(userID, ta, []byte("pdf-2"), "a_2222.pdf", nil, "", RoleFingerprint("https://jobs.example.com/backend/"))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("want refreshed row to keep id %s, got %s", first.ID, second.ID)
	}

	list, err := s.ListGenerations(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Filename != "a_2222.pdf" {
		t.Fatalf("want one refreshed generation, got %+v", list)
	}
	pdf, _, err := s.GetGenerationPDF(userID, first.ID)
	if err != nil || string(pdf) != "pdf-2" {
		t.Fatalf("want refreshed pdf, got %q %v", pdf, err)
	}

	// A different job inserts a second row; another user's same job never
	// touches the first user's row.
	third, err := s.SaveGeneration(userID, ta, []byte("pdf-3"), "b_3333.pdf", nil, "", RoleFingerprint("Backend Engineer at Acme"))
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == first.ID {
		t.Fatal("different job must not reuse the row")
	}
	other, err := s.SaveGeneration(int64(2), ta, []byte("pdf-4"), "c_4444.pdf", nil, "", fp)
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID {
		t.Fatal("another user's job must not reuse the row")
	}
}

func TestPinnedGenerationsSortFirst(t *testing.T) {
	s := open(t)
	userID := int64(1)
	ta := model.Tailored{TargetRole: "Backend Engineer"}

	older, err := s.SaveGeneration(userID, ta, []byte("p1"), "a.pdf", nil, "", "fp-a")
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.SaveGeneration(userID, ta, []byte("p2"), "b.pdf", nil, "", "fp-b")
	if err != nil {
		t.Fatal(err)
	}

	found, err := s.SetGenerationPinned(userID, older.ID, true)
	if err != nil || !found {
		t.Fatalf("pin: %v found=%v", err, found)
	}
	list, err := s.ListGenerations(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != older.ID || !list[0].Pinned || list[1].ID != newer.ID {
		t.Fatalf("want pinned first, got %+v", list)
	}

	// Unpin restores recency order; unknown ids and other users report false.
	if _, err := s.SetGenerationPinned(userID, older.ID, false); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListGenerations(userID)
	if list[0].ID != newer.ID {
		t.Fatalf("want recency order after unpin, got %+v", list)
	}
	if found, _ := s.SetGenerationPinned(int64(2), newer.ID, true); found {
		t.Fatal("another user must not pin this row")
	}
}

func TestSoftDeleteUserTombstonesAndFreesIdentity(t *testing.T) {
	s := open(t)
	created, _, err := s.UpsertUser("google", "g-9", "ada@example.com", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	first := created.ID
	if err := s.SaveProfile(first, model.Profile{Summary: "kept"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDeleteUser(first); err != nil {
		t.Fatal(err)
	}

	if got, err := s.GetUser(first); err != nil || got != nil {
		t.Fatalf("tombstoned user still resolves: %v, %v", got, err)
	}
	var email, providerID, deletedAt string
	if err := s.db.QueryRow(
		`SELECT email, provider_id, deleted_at FROM users WHERE id = ?`, first,
	).Scan(&email, &providerID, &deletedAt); err != nil {
		t.Fatalf("row should still exist: %v", err)
	}
	if !strings.HasPrefix(email, "deleted_") || !strings.HasSuffix(email, "ada@example.com") {
		t.Fatalf("email not tombstoned: %q", email)
	}
	if !strings.HasPrefix(providerID, "deleted_") || !strings.HasSuffix(providerID, "g-9") {
		t.Fatalf("provider_id not tombstoned: %q", providerID)
	}

	// The data stays put; only the account is closed.
	var profiles int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM profile WHERE user_id = ?`, first).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if profiles != 1 {
		t.Fatalf("want profile kept, got %d rows", profiles)
	}

	second, _, err := s.UpsertUser("google", "g-9", "ada@example.com", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first {
		t.Fatal("re-signup reopened the deleted account")
	}
	if p, err := s.LoadProfile(second.ID); err != nil || p != nil {
		t.Fatalf("fresh account inherited a profile: %+v, %v", p, err)
	}
}

func TestSoftDeleteUserTwiceKeepsIdentitiesUnique(t *testing.T) {
	s := open(t)
	for i := 0; i < 2; i++ {
		u, _, err := s.UpsertUser("google", "g-dup", "dup@example.com", "Dup")
		if err != nil {
			t.Fatalf("signup %d: %v", i, err)
		}
		if err := s.SoftDeleteUser(u.ID); err != nil {
			t.Fatalf("delete %d: %v", i, err)
		}
	}
}

func TestListGenerationsDerivesFitAndCoverage(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "fit-user")

	ta := model.Tailored{
		TargetRole:     "Backend Engineer",
		SelectedSkills: []string{"Go", "PostgreSQL"},
		Gaps:           []model.Gap{{Requirement: "Kubernetes", Severity: "missing"}},
		Sections: []model.TSection{{Title: "Experience", Items: []model.TItem{{
			SourceID: "item-0", Title: "Engineer",
			Bullets: []model.TBullet{{SourceBulletID: "item-0-b-0", Text: "Built it"}, {SourceBulletID: "item-0-b-1", Text: "Shipped it"}},
		}}}},
	}
	meta, err := s.SaveGeneration(userID, ta, []byte("pdf"), "a.pdf", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Before a posting is attached there is nothing to score against.
	list, err := s.ListGenerations(userID)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Fit != nil || list[0].Coverage != nil {
		t.Fatalf("want no fit without a posting, got %+v", list[0])
	}

	if err := s.SavePosting(userID, meta.ID, model.Posting{
		Usable: true, Title: "Backend Engineer", Keywords: []string{"Go", "PostgreSQL", "Kubernetes", "Kafka"},
	}); err != nil {
		t.Fatal(err)
	}

	list, err = s.ListGenerations(userID)
	if err != nil {
		t.Fatal(err)
	}
	got := list[0]
	if got.Coverage == nil || got.Coverage.Covered != 2 || got.Coverage.Total != 4 {
		t.Fatalf("coverage wrong: %+v", got.Coverage)
	}
	if got.Fit == nil || got.Fit.Score == 100 {
		t.Fatalf("fit should reflect the gap and the misses: %+v", got.Fit)
	}
}

func TestGetPostingRoundTrips(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "posting-user")
	meta, err := s.SaveGeneration(userID, model.Tailored{TargetRole: "R"}, []byte("pdf"), "a.pdf", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if p, err := s.GetPosting(userID, meta.ID); err != nil || p != nil {
		t.Fatalf("want nil before save, got %v, %v", p, err)
	}
	want := model.Posting{Usable: true, Title: "Backend Engineer", Company: "Venix"}
	if err := s.SavePosting(userID, meta.ID, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPosting(userID, meta.ID)
	if err != nil || got == nil || got.Title != want.Title || got.Company != want.Company {
		t.Fatalf("got %+v, %v", got, err)
	}

	// Scoped per user, like every other generation read.
	other := testUser(t, s, "google", "posting-other")
	if p, err := s.GetPosting(other, meta.ID); err != nil || p != nil {
		t.Fatalf("another user read the posting: %v, %v", p, err)
	}
}

// The welcome email hangs off this bool, so it has to be true exactly once.
func TestUpsertUserReportsCreation(t *testing.T) {
	s := open(t)

	u, created, err := s.UpsertUser("google", "g-new", "new@example.com", "New")
	if err != nil || !created {
		t.Fatalf("first sign-in should create: %v, %v", created, err)
	}
	if _, created, err = s.UpsertUser("google", "g-new", "new@example.com", "New"); err != nil || created {
		t.Fatalf("second sign-in should not create: %v, %v", created, err)
	}

	// A closed account frees the identity, so signing in again is a new
	// account and gets greeted like one.
	if err := s.SoftDeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, created, err = s.UpsertUser("google", "g-new", "new@example.com", "New"); err != nil || !created {
		t.Fatalf("re-signup after deletion should create: %v, %v", created, err)
	}
}
