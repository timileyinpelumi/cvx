package model

import (
	"strings"
	"testing"
)

func TestIntakeAsksForDatesFirst(t *testing.T) {
	p := Profile{
		Skills: []string{"Go", "Python", "SQL", "Docker", "AWS", "Kafka"},
		Items: []Item{
			{ID: "item-0", Kind: "experience", Title: "Engineer", Organization: "Venix",
				Bullets: []Bullet{{Text: "Cut checkout errors by 30%"}}},
			{ID: "item-1", Kind: "experience", Title: "Engineer", Organization: "ChainPal",
				StartDate: "2021-01", Bullets: []Bullet{{Text: "Shipped the settlement path in 2 weeks"}}},
			{ID: "item-2", Kind: "education", Title: "BSc", Organization: "FUTA", StartDate: "2015"},
		},
	}

	got := IntakeQuestions(p)
	if len(got) == 0 {
		t.Fatal("an undated item should be asked about")
	}
	if got[0].ID != "dates:item-0" || got[0].ItemID != "item-0" {
		t.Fatalf("dates should lead: %+v", got[0])
	}
	if !strings.Contains(got[0].Ask, "Venix") {
		t.Fatalf("the question should name the job: %q", got[0].Ask)
	}
}

// A bullet with a figure in it is evidence; one without is a claim, and only
// the person can supply the figure.
func TestIntakeAsksForOutcomesOnlyWhereMissing(t *testing.T) {
	p := Profile{
		Skills: []string{"Go", "Python", "SQL", "Docker", "AWS", "Kafka"},
		Items: []Item{
			{ID: "item-0", Kind: "experience", Organization: "Venix", StartDate: "2021",
				Bullets: []Bullet{{Text: "Built the payment service"}}},
			{ID: "item-1", Kind: "experience", Organization: "ChainPal", StartDate: "2020",
				Bullets: []Bullet{{Text: "Cut settlement time by 40%"}}},
			{ID: "item-2", Kind: "education", Organization: "FUTA", StartDate: "2015"},
		},
	}

	var asked []string
	for _, q := range IntakeQuestions(p) {
		asked = append(asked, q.ID)
	}
	joined := strings.Join(asked, " ")
	if !strings.Contains(joined, "outcome:item-0") {
		t.Fatalf("the bullet with no number should be asked about: %v", asked)
	}
	if strings.Contains(joined, "outcome:item-1") {
		t.Fatalf("a bullet that already carries a number should be left alone: %v", asked)
	}
	if strings.Contains(joined, "outcome:item-2") {
		t.Fatalf("education is not asked for outcomes: %v", asked)
	}
}

func TestIntakeAsksForVolumeAndSkillsWhenThin(t *testing.T) {
	p := Profile{
		Items:  []Item{{ID: "item-0", Organization: "Venix", StartDate: "2021", Bullets: []Bullet{{Text: "Shipped 3 services"}}}},
		Skills: []string{"Go"},
	}

	byID := map[string]Question{}
	for _, q := range IntakeQuestions(p) {
		byID[q.ID] = q
	}
	for _, want := range []string{"more-items", "skills", "education"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("missing question %q: %v", want, byID)
		}
	}
	if !strings.Contains(byID["more-items"].Why, "one entry") {
		t.Errorf("the reason should say what is actually there: %q", byID["more-items"].Why)
	}
}

// An abandoned intake is worse than no intake, so the list is bounded and
// every question carries an example answer.
func TestIntakeStaysShortAndAnswerable(t *testing.T) {
	var p Profile
	for i := 0; i < 12; i++ {
		p.Items = append(p.Items, Item{ID: "item-" + string(rune('0'+i)), Organization: "Somewhere"})
	}

	got := IntakeQuestions(p)
	if len(got) > maxQuestions {
		t.Fatalf("want at most %d questions, got %d", maxQuestions, len(got))
	}
	for _, q := range got {
		if q.Ask == "" || q.Why == "" || q.Placeholder == "" {
			t.Fatalf("a question without a reason or an example: %+v", q)
		}
	}
}

// A complete profile is not interrogated.
func TestIntakeIsSilentOnACompleteProfile(t *testing.T) {
	p := Profile{
		Skills: []string{"Go", "Python", "SQL", "Docker", "AWS", "Kafka"},
		Items: []Item{
			{ID: "item-0", Kind: "experience", Organization: "Venix", StartDate: "2021",
				Bullets: []Bullet{{Text: "Cut errors by 30%"}}},
			{ID: "item-1", Kind: "experience", Organization: "ChainPal", StartDate: "2020",
				Bullets: []Bullet{{Text: "Shipped 4 services"}}},
			{ID: "item-2", Kind: "education", Organization: "FUTA", StartDate: "2015"},
		},
	}
	if got := IntakeQuestions(p); len(got) != 0 {
		t.Fatalf("nothing to ask, got %+v", got)
	}
}
