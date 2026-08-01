package store

import (
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
	a, err := s.SaveGeneration(ta, []byte("pdf-a"), "a.pdf")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	b, _ := s.SaveGeneration(ta, []byte("pdf-b"), "b.pdf")
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
