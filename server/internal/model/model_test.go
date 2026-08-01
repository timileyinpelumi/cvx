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
	got := Filename("Ada  Lovelace", "Sr. Engineer (Backend)")
	if got != "Ada_Lovelace_Sr_Engineer_Backend.pdf" {
		t.Fatalf("got %q", got)
	}
}

func TestCoverFilename(t *testing.T) {
	got := CoverFilename("Ada  Lovelace", "Sr. Engineer (Backend)")
	if got != "Ada_Lovelace_Sr_Engineer_Backend_Cover_Letter.pdf" {
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
