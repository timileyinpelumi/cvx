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

func TestNormalizeTailoredTrimsPastedRoleTitle(t *testing.T) {
	tl := Tailored{
		TargetRole: "Freelance website manager for Savvy Spender (running website, blog posting, SEO, ad-hoc backend changes, content updates, email outreach, newsletter editing, social media)",
	}
	NormalizeTailored(&tl)
	if got := tl.TargetRole; got != "Freelance website manager for Savvy Spender" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeTailoredClampsRoleSummary(t *testing.T) {
	tl := Tailored{RoleSummary: strings.Repeat("word ", 60)}
	NormalizeTailored(&tl)
	if len(tl.RoleSummary) > maxRoleSummary {
		t.Fatalf("role summary not clamped: %d chars", len(tl.RoleSummary))
	}
}

func TestShapeIssuesReportsFloors(t *testing.T) {
	p := Profile{
		Skills: []string{"Go", "Python", "SQL", "Docker", "AWS", "Kafka"},
		Items: []Item{
			{ID: "item-0", Bullets: []Bullet{{ID: "item-0-b-0"}, {ID: "item-0-b-1"}}},
			{ID: "item-1", Bullets: []Bullet{{ID: "item-1-b-0"}, {ID: "item-1-b-1"}}},
			{ID: "item-2", Bullets: []Bullet{{ID: "item-2-b-0"}, {ID: "item-2-b-1"}}},
		},
	}
	tl := Tailored{
		Summary:        "Too short by far.",
		SelectedSkills: []string{"Go"},
		Sections: []TSection{{Items: []TItem{
			{SourceID: "item-0", Title: "Engineer", Bullets: []TBullet{{}}},
		}}},
	}
	issues := ShapeIssues(p, tl)
	if len(issues) != 4 {
		t.Fatalf("want summary, bullet, item-count and skill issues, got %d: %v", len(issues), issues)
	}
}

// A sparse profile cannot be pushed past what it holds: asking for three
// items and six skills when the profile has one of each only invites
// invention, which the id guardrail would then reject.
func TestShapeIssuesCapsFloorsToProfile(t *testing.T) {
	p := Profile{
		Skills: []string{"Go"},
		Items:  []Item{{ID: "item-0", Bullets: []Bullet{{ID: "item-0-b-0"}}}},
	}
	tl := Tailored{
		Summary:        strings.Repeat("word ", minSummaryWords),
		SelectedSkills: []string{"Go"},
		Sections: []TSection{{Items: []TItem{
			{SourceID: "item-0", Title: "Engineer", Bullets: []TBullet{{}}},
		}}},
	}
	if issues := ShapeIssues(p, tl); len(issues) != 0 {
		t.Fatalf("want no issues for a profile this sparse, got %v", issues)
	}
}

func TestNormalizeTailoredDropsStubItemsWhilePageHoldsUp(t *testing.T) {
	item := func(bullets int) TItem {
		it := TItem{Title: "T"}
		for i := 0; i < bullets; i++ {
			it.Bullets = append(it.Bullets, TBullet{})
		}
		return it
	}
	tl := Tailored{Sections: []TSection{{Items: []TItem{
		item(3), item(1), item(2), item(2), item(1),
	}}}}
	NormalizeTailored(&tl)
	for _, it := range tl.Sections[0].Items {
		if len(it.Bullets) < minItemBullets {
			t.Fatalf("stub item survived: %+v", tl.Sections[0].Items)
		}
	}
	if got := len(tl.Sections[0].Items); got != 3 {
		t.Fatalf("want 3 items kept, got %d", got)
	}
}

// The narrator voice, exactly as it shipped: "Timileyin builds robust,
// scalable systems... He architected end-to-end payment platforms..."
func TestShapeIssuesRejectsNarratorVoice(t *testing.T) {
	p := Profile{Name: "Timileyin Pelumi"}
	cases := map[string]string{
		"names the candidate": "Timileyin builds robust, scalable systems across fintech, blockchain, and AI domains, architecting payment platforms and designing layered protocols for teams that need both depth and speed in equal measure today.",
		"third person":        "Full stack engineer who builds payment systems. He architected end-to-end crypto-to-fiat platforms and designed the layered protocol behind them, bringing deep system design and security expertise to every team he has worked with so far.",
		"first person":        "I build robust, scalable systems across fintech and blockchain, and I architected end-to-end payment platforms and designed layered protocols for teams that needed both depth and delivery speed in equal measure over five years.",
		"introduced":          "This candidate builds robust, scalable systems across fintech, blockchain, and AI domains, having architected payment platforms and designed layered protocols for teams that needed depth and delivery speed alike.",
	}
	for name, summary := range cases {
		t.Run(name, func(t *testing.T) {
			issues := ShapeIssues(p, Tailored{Summary: summary})
			if len(issues) == 0 {
				t.Fatalf("narrator voice accepted: %s", summary)
			}
		})
	}
}

func TestShapeIssuesAcceptsImpliedFirstPerson(t *testing.T) {
	p := Profile{Name: "Timileyin Pelumi"}
	tl := Tailored{
		Headline: "Full stack engineer specializing in payment infrastructure",
		Summary: "Full stack engineer with five years building payment infrastructure in Go and Python, " +
			"from crypto-to-fiat rails at ChainPal to the layered protocol behind Solana settlement. " +
			"Works close to the money path, where correctness under load and clear failure handling matter " +
			"more than throughput alone, and has taken services from first commit through production ownership. " +
			"That is the same ground this architecture role covers today.",
	}
	for _, issue := range ShapeIssues(p, tl) {
		if strings.Contains(issue, "pronoun") || strings.Contains(issue, "names the candidate") {
			t.Fatalf("clean summary flagged: %s", issue)
		}
	}
}
