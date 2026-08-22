package model

import "testing"

func trimFixture() Tailored {
	item := func(title string, bullets int) TItem {
		it := TItem{SourceID: "item-x", Title: title}
		for i := 0; i < bullets; i++ {
			it.Bullets = append(it.Bullets, TBullet{Text: "did a thing"})
		}
		return it
	}
	return Tailored{
		Certifications: []string{"AWS Solutions Architect"},
		Languages:      []string{"English", "Yoruba"},
		Interests:      []string{"Open source"},
		Sections: []TSection{
			{Kind: SectionExperience, Title: "Experience", Items: []TItem{
				item("Backend Engineer", 4), item("Engineer", 3), item("Intern", 2),
			}},
			{Kind: SectionProjects, Title: "Projects", Items: []TItem{item("ChainPal", 2), item("Spawn", 2)}},
		},
	}
}

// The trim order is the whole design: the page loses its least valuable line
// first, and experience is the last thing to go.
func TestTrimOrder(t *testing.T) {
	tl := trimFixture()
	var steps []string
	for {
		step, ok := tl.Trim()
		if !ok {
			break
		}
		steps = append(steps, string(step))
	}

	want := []string{
		"interests",
		"languages",
		"certifications",
		"Projects: Spawn",
		"Projects: ChainPal",
		// The fattest item loses a bullet each time, so the four-bullet item
		// goes twice before the three-bullet one is touched.
		"a bullet from Backend Engineer",
		"a bullet from Backend Engineer",
		"a bullet from Engineer",
	}
	for i, w := range want {
		if i >= len(steps) || steps[i] != w {
			t.Fatalf("step %d: want %q, got %v", i, w, steps)
		}
	}
}

// Trimming stops at the floors: a resume with one item and one bullet each
// has stopped being a resume, so the fit loop gives up instead.
func TestTrimStopsAtTheFloors(t *testing.T) {
	tl := trimFixture()
	for i := 0; i < MaxTrimSteps*4; i++ {
		if _, ok := tl.Trim(); !ok {
			break
		}
	}
	total := 0
	for _, s := range tl.Sections {
		total += len(s.Items)
		for _, it := range s.Items {
			if len(it.Bullets) < minItemBullets {
				t.Fatalf("trimmed an item below the bullet floor: %+v", it)
			}
		}
	}
	if total < minResumeItems {
		t.Fatalf("trimmed below the item floor: %d items", total)
	}
	if _, ok := tl.Trim(); ok {
		t.Fatal("Trim should report nothing left to drop")
	}
}

func TestCloneIsDeep(t *testing.T) {
	original := trimFixture()
	copied := original.Clone()

	copied.Sections[0].Items[0].Bullets[0].Text = "changed"
	copied.Sections[0].Items = copied.Sections[0].Items[:1]
	copied.Interests = nil

	if original.Sections[0].Items[0].Bullets[0].Text != "did a thing" {
		t.Error("bullet text leaked back into the original")
	}
	if len(original.Sections[0].Items) != 3 {
		t.Error("item list leaked back into the original")
	}
	if len(original.Interests) != 1 {
		t.Error("interests leaked back into the original")
	}
}

func TestSectionsSortIntoReadingOrder(t *testing.T) {
	tl := Tailored{Sections: []TSection{
		{Kind: SectionInterestsUnknown(), Title: "Whatever", Items: []TItem{{Bullets: []TBullet{{}, {}}}}},
		{Kind: SectionEducation, Title: "Education", Items: []TItem{{Bullets: []TBullet{{}, {}}}}},
		{Kind: SectionExperience, Title: "Experience", Items: []TItem{{Bullets: []TBullet{{}, {}}}}},
		{Kind: SectionProjects, Title: "Projects", Items: []TItem{{Bullets: []TBullet{{}, {}}}}},
	}}
	NormalizeTailored(&tl)

	want := []string{"Experience", "Projects", "Education", "Whatever"}
	for i, w := range want {
		if tl.Sections[i].Title != w {
			t.Fatalf("position %d: want %s, got %s", i, w, tl.Sections[i].Title)
		}
	}
}

// SectionInterestsUnknown stands in for a kind the model made up, which must
// sort last rather than crashing or leading the page.
func SectionInterestsUnknown() string { return "something-invented" }
