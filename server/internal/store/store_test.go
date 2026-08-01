package store

import (
	"database/sql"
	"path/filepath"
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

func TestProfileRoundTrip(t *testing.T) {
	s := open(t)
	if p, err := s.LoadProfile(); err != nil || p != nil {
		t.Fatalf("want nil,nil got %v,%v", p, err)
	}
	prof := model.Profile{Name: "Ada", Items: []model.Item{{Title: "Engineer"}}}
	model.AssignIDs(&prof)
	if err := s.SaveProfile(prof); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProfile(prof); err != nil { // upsert, not duplicate
		t.Fatal(err)
	}
	p, err := s.LoadProfile()
	if err != nil || p.Name != "Ada" || p.Items[0].ID != "item-0" {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestGenerations(t *testing.T) {
	s := open(t)
	ta := model.Tailored{TargetRole: "Python Backend Engineer", Gaps: []model.Gap{{Requirement: "Django", Severity: "missing"}}, WhatChanged: []string{"x"}}
	a, err := s.SaveGeneration(ta, []byte("pdf-a"), "a.pdf", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	b, _ := s.SaveGeneration(ta, []byte("pdf-b"), "b.pdf", nil, "")
	list, err := s.ListGenerations()
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("bad list: %+v %v", list, err)
	}
	if list[1].Gaps[0].Requirement != "Django" {
		t.Fatalf("gaps not persisted: %+v", list[1])
	}
	pdf, fn, err := s.GetGenerationPDF(a.ID)
	if err != nil || string(pdf) != "pdf-a" || fn != "a.pdf" {
		t.Fatalf("got %q %q %v", pdf, fn, err)
	}
	if pdf, _, err := s.GetGenerationPDF("nope"); err != nil || pdf != nil {
		t.Fatalf("want nil,nil for unknown id, got %q %v", pdf, err)
	}
}

func TestGenerationsWithCoverLetter(t *testing.T) {
	s := open(t)
	ta := model.Tailored{TargetRole: "X"}

	withCover, err := s.SaveGeneration(ta, []byte("pdf"), "x.pdf", []byte("cover-pdf"), "x-cover.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !withCover.HasCoverLetter {
		t.Fatalf("want HasCoverLetter true, got %+v", withCover)
	}
	pdf, fn, err := s.GetGenerationCoverPDF(withCover.ID)
	if err != nil || string(pdf) != "cover-pdf" || fn != "x-cover.pdf" {
		t.Fatalf("got %q %q %v", pdf, fn, err)
	}

	time.Sleep(5 * time.Millisecond)
	withoutCover, err := s.SaveGeneration(ta, []byte("pdf2"), "y.pdf", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if withoutCover.HasCoverLetter {
		t.Fatalf("want HasCoverLetter false, got %+v", withoutCover)
	}
	if pdf, fn, err := s.GetGenerationCoverPDF(withoutCover.ID); err != nil || pdf != nil || fn != "" {
		t.Fatalf("want nil,\"\" for absent cover, got %q %q %v", pdf, fn, err)
	}
	if _, _, err := s.GetGenerationCoverPDF("nope"); err != nil {
		t.Fatalf("want nil error for unknown id, got %v", err)
	}

	list, err := s.ListGenerations()
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

	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }

	if _, err := s.SaveGeneration(mk(model.Gap{Requirement: "Django", Evidence: "e1", Severity: "missing"}), []byte("p"), "a.pdf", nil, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.SaveGeneration(mk(
		model.Gap{Requirement: "django", Evidence: "e2", Severity: "weak"},
		model.Gap{Requirement: "Kubernetes", Evidence: "e-k", Severity: "missing"},
	), []byte("p"), "b.pdf", nil, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.SaveGeneration(mk(model.Gap{Requirement: "Django", Evidence: "e3-latest", Severity: "missing"}), []byte("p"), "c.pdf", nil, ""); err != nil {
		t.Fatal(err)
	}

	trends, total, err := s.GapSummary()
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

	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }

	if _, err := s.SaveGeneration(mk(
		model.Gap{Requirement: "Django", Evidence: "e1", Severity: "missing"},
		model.Gap{Requirement: "django ", Evidence: "e1-dup", Severity: "weak"},
	), []byte("p"), "a.pdf", nil, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.SaveGeneration(mk(model.Gap{Requirement: "Django", Evidence: "e2", Severity: "missing"}), []byte("p"), "b.pdf", nil, ""); err != nil {
		t.Fatal(err)
	}

	trends, total, err := s.GapSummary()
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

	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }

	// "Zeta" appears 3 times, "Alpha" appears 3 times (ties go alphabetical),
	// "Beta" appears 2 times: expect order Alpha, Zeta, Beta.
	for i := 0; i < 3; i++ {
		if _, err := s.SaveGeneration(mk(model.Gap{Requirement: "Zeta", Evidence: "e", Severity: "missing"}), []byte("p"), "z.pdf", nil, ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.SaveGeneration(mk(model.Gap{Requirement: "Alpha", Evidence: "e", Severity: "missing"}), []byte("p"), "a.pdf", nil, ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.SaveGeneration(mk(model.Gap{Requirement: "Beta", Evidence: "e", Severity: "weak"}), []byte("p"), "b.pdf", nil, ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	trends, total, err := s.GapSummary()
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
	trends, total, err := s.GapSummary()
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

	ta := model.Tailored{TargetRole: "X"}
	meta, err := s1.SaveGeneration(ta, []byte("pdf"), "x.pdf", []byte("cover-pdf"), "x-cover.pdf")
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

	pdf, fn, err := s2.GetGenerationCoverPDF(meta.ID)
	if err != nil || string(pdf) != "cover-pdf" || fn != "x-cover.pdf" {
		t.Fatalf("got %q %q %v", pdf, fn, err)
	}
}
