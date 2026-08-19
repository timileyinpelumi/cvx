package model

import (
	"strings"
	"testing"
)

func sample() Profile {
	p := Profile{Name: "Ada Lovelace", Email: "ada@example.com", Skills: []string{"Python"},
		Items: []Item{{Kind: "experience", Title: "Engineer", Bullets: []Bullet{{Text: "Built engine"}}}}}
	AssignIDs(&p)
	return p
}

func TestAssignIDs(t *testing.T) {
	p := sample()
	if p.Items[0].ID != "item-0" || p.Items[0].Bullets[0].ID != "item-0-b-0" {
		t.Fatalf("bad ids: %+v", p.Items[0])
	}
}

func TestValidateTailored(t *testing.T) {
	p := sample()
	ok := Tailored{Sections: []TSection{{Title: "Experience", Items: []TItem{{SourceID: "item-0",
		Bullets: []TBullet{{SourceBulletID: "item-0-b-0", Text: "Built the engine in Python"}}}}}}}
	if err := ValidateTailored(p, ok); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	badItem := ok
	badItem.Sections[0].Items[0].SourceID = "item-9"
	if err := ValidateTailored(p, badItem); err == nil || !strings.Contains(err.Error(), "item-9") {
		t.Fatalf("want item-9 error, got %v", err)
	}
	badBullet := sampleTailoredWithBullet("item-0", "item-0-b-9")
	if err := ValidateTailored(p, badBullet); err == nil || !strings.Contains(err.Error(), "item-0-b-9") {
		t.Fatalf("want bullet error, got %v", err)
	}
}

func sampleTailoredWithBullet(itemID, bulletID string) Tailored {
	return Tailored{Sections: []TSection{{Items: []TItem{{SourceID: itemID,
		Bullets: []TBullet{{SourceBulletID: bulletID, Text: "x"}}}}}}}
}

func TestFilename(t *testing.T) {
	got := Filename("Ada  Lovelace", "Sr. Engineer (Backend)", 1234)
	if got != "Ada_Lovelace_Sr_Engineer_Backend_1234.pdf" {
		t.Fatalf("got %q", got)
	}
}

func TestFilenameCapsLongParts(t *testing.T) {
	got := Filename(
		"Timileyin Oluwapelumi Ademidun",
		"Senior Staff Platform Reliability Engineer, Payments Infrastructure",
		1234,
	)
	if got != "Timileyin_Senior_Staff_Platform_1234.pdf" {
		t.Fatalf("got %q", got)
	}
	if len(got) > 60 {
		t.Fatalf("filename too long: %d chars", len(got))
	}
}

func TestCoverFilename(t *testing.T) {
	got := CoverFilename("Ada  Lovelace", "Sr. Engineer (Backend)", 1234)
	if got != "Ada_Lovelace_Sr_Engineer_Backend_1234_Cover.pdf" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeAdditionsNewSkillsDedupCaseInsensitive(t *testing.T) {
	p := sample() // Skills: ["Python"]
	err := MergeAdditions(&p, ProfileAdditions{NewSkills: []string{"python", "Go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Skills) != 2 || p.Skills[0] != "Python" || p.Skills[1] != "Go" {
		t.Fatalf("want [Python Go], got %v", p.Skills)
	}
}

func TestMergeAdditionsNewItemsContinueIDSequence(t *testing.T) {
	p := sample() // Items: [item-0]
	err := MergeAdditions(&p, ProfileAdditions{
		NewItems: []ItemDraft{{
			Kind: "project", Title: "Side project", Organization: "",
			Bullets: []BulletDraft{{Text: "Built a CLI"}, {Text: "Shipped v1"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(p.Items))
	}
	newItem := p.Items[1]
	if newItem.ID != "item-1" {
		t.Fatalf("want item-1, got %q", newItem.ID)
	}
	if len(newItem.Bullets) != 2 || newItem.Bullets[0].ID != "item-1-b-0" || newItem.Bullets[1].ID != "item-1-b-1" {
		t.Fatalf("bad bullet ids: %+v", newItem.Bullets)
	}
}

func TestMergeAdditionsBulletAdditionsContinuePerItemIndex(t *testing.T) {
	p := sample() // item-0 has one bullet: item-0-b-0
	err := MergeAdditions(&p, ProfileAdditions{
		BulletAdditions: []BulletAddition{{
			ItemID:  "item-0",
			Bullets: []BulletDraft{{Text: "Led the migration"}, {Text: "Cut latency 40%"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := p.Items[0].Bullets
	if len(got) != 3 {
		t.Fatalf("want 3 bullets, got %d: %+v", len(got), got)
	}
	if got[1].ID != "item-0-b-1" || got[2].ID != "item-0-b-2" {
		t.Fatalf("bad continued bullet ids: %+v", got)
	}
}

func TestMergeAdditionsUnknownItemIDErrors(t *testing.T) {
	p := sample()
	err := MergeAdditions(&p, ProfileAdditions{
		BulletAdditions: []BulletAddition{{ItemID: "item-9", Bullets: []BulletDraft{{Text: "x"}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "item-9") {
		t.Fatalf("want error naming item-9, got %v", err)
	}
	if len(p.Items[0].Bullets) != 1 {
		t.Fatalf("want profile unchanged on error, got %+v", p.Items[0].Bullets)
	}
}

func TestNormalizeTailored(t *testing.T) {
	item := func(bullets int) TItem {
		it := TItem{SourceID: "item-0", Title: "Engineer"}
		for i := 0; i < bullets; i++ {
			it.Bullets = append(it.Bullets, TBullet{SourceBulletID: "item-0-b-0", Text: "did a thing"})
		}
		return it
	}

	ta := Tailored{
		Headline: strings.Repeat("word ", 40),
		Summary:  strings.Repeat("one two three four five six seven eight nine ten. ", 10),
		Sections: []TSection{
			{Title: "Experience", Items: []TItem{item(6), item(2), item(2), item(2)}},
			{Title: "Projects", Items: []TItem{item(2), item(2)}},
			{Title: "Empty"},
		},
		SelectedSkills: make([]string, 20),
		WhatChanged:    make([]string, 6),
		Gaps:           make([]Gap, 9),
	}
	NormalizeTailored(&ta)

	total := 0
	for _, s := range ta.Sections {
		total += len(s.Items)
		for _, it := range s.Items {
			if len(it.Bullets) > 4 {
				t.Fatalf("bullets not clamped: %d", len(it.Bullets))
			}
		}
		if len(s.Items) == 0 {
			t.Fatal("empty section not dropped")
		}
	}
	if total != 5 {
		t.Fatalf("want 5 items total, got %d", total)
	}
	if len(ta.Headline) > 110 || strings.HasSuffix(ta.Headline, " ") {
		t.Fatalf("headline not clamped cleanly: %q", ta.Headline)
	}
	if got := len(strings.Fields(ta.Summary)); got > 75 {
		t.Fatalf("summary not clamped: %d words", got)
	}
	if !strings.HasSuffix(ta.Summary, ".") {
		t.Fatalf("summary should end on a sentence: %q", ta.Summary)
	}
	if len(ta.SelectedSkills) != 14 || len(ta.WhatChanged) != 4 || len(ta.Gaps) != 6 {
		t.Fatalf("list caps not applied: %d %d %d", len(ta.SelectedSkills), len(ta.WhatChanged), len(ta.Gaps))
	}
}

func TestNormalizeTailoredLeavesCompliantAlone(t *testing.T) {
	ta := Tailored{
		Headline: "Backend Engineer",
		Summary:  "Short and sweet.",
		Sections: []TSection{{Title: "Experience", Items: []TItem{{SourceID: "item-0", Bullets: []TBullet{{}, {}}}}}},
	}
	before := ta.Headline + ta.Summary
	NormalizeTailored(&ta)
	if ta.Headline+ta.Summary != before || len(ta.Sections) != 1 || len(ta.Sections[0].Items) != 1 {
		t.Fatalf("compliant output was altered: %+v", ta)
	}
}

func TestFilenameTitleCases(t *testing.T) {
	got := Filename("TIMILEYIN PELUMI", "devops engineer", 3698)
	if got != "Timileyin_Pelumi_Devops_Engineer_3698.pdf" {
		t.Fatalf("got %q", got)
	}
}
