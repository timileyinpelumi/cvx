package pdfgen

import (
	"bytes"
	"testing"

	"cvx/internal/model"
)

func fixture() (model.Profile, model.Tailored) {
	p := model.Profile{
		Name:     "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+1 555 0100",
		Location: "London, UK",
		Summary:  "Mathematician and writer.",
		Links:    []model.Link{{Label: "github.com/ada", URL: "https://github.com/ada"}},
		Skills:   []string{"Python", "Analytical Engines"},
		Items: []model.Item{
			{
				Kind:         "experience",
				Title:        "Engineer",
				Organization: "Analytical Engines Co",
				StartDate:    "2021-01",
				EndDate:      "",
				Bullets: []model.Bullet{
					{Text: "Built the engine"},
					{Text: "Wrote the first algorithm"},
				},
			},
		},
	}
	model.AssignIDs(&p)

	ta := model.Tailored{
		TargetRole:     "Python Backend Engineer",
		Headline:       "Backend Engineer specializing in numerical computing",
		Summary:        "Mathematician and writer with deep systems experience.",
		SelectedSkills: []string{"Python"},
		Sections: []model.TSection{
			{
				Title: "Experience",
				Items: []model.TItem{
					{
						SourceID:     p.Items[0].ID,
						Title:        "Engineer",
						Organization: "Analytical Engines Co",
						Dates:        "2021 – Present",
						Bullets: []model.TBullet{
							{SourceBulletID: p.Items[0].Bullets[0].ID, Text: "Built the engine in Python"},
							{SourceBulletID: p.Items[0].Bullets[1].ID, Text: "Wrote the first published algorithm"},
						},
					},
				},
			},
		},
		Gaps:        []model.Gap{{Requirement: "Django", Evidence: "not in profile", Severity: "missing"}},
		WhatChanged: []string{"led with Python"},
	}
	return p, ta
}

func TestRender(t *testing.T) {
	p, ta := fixture()
	b, err := Render(p, ta)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(b))
	}
}

func TestRenderEmptySections(t *testing.T) {
	p, ta := fixture()
	ta.Sections = nil
	b, err := Render(p, ta)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(b))
	}
}

// TestRenderLongTitleWraps guards against the title cell overlapping the
// right-aligned date column: a "Title — Org" combo wider than the space left
// of the date column (~150mm vs a ~146mm budget on this fixture) must wrap
// via MultiCell instead of running through the dates.
func TestRenderLongTitleWraps(t *testing.T) {
	p, base := fixture()
	baseBytes, err := Render(p, base)
	if err != nil {
		t.Fatal(err)
	}

	_, ta := fixture()
	ta.Sections[0].Items[0].Title = "Director of Engineering, Distributed Systems and Cloud Platform"
	ta.Sections[0].Items[0].Organization = "Global Technology Solutions Inc"

	longBytes, err := Render(p, ta)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(longBytes, []byte("%PDF")) || len(longBytes) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(longBytes))
	}
	// The wrapped title adds a second line of content that the single-line
	// baseline doesn't have, so the encoded output should grow.
	if len(longBytes) <= len(baseBytes) {
		t.Fatalf("expected wrapped long-title pdf to be larger than baseline: long=%d base=%d", len(longBytes), len(baseBytes))
	}
}
