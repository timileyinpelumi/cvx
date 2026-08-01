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
