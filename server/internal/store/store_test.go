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
