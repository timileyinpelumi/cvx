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
