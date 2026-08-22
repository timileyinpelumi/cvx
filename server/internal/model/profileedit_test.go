package model

import "testing"

func storedProfile() Profile {
	return Profile{
		Name:  "Ada Lovelace",
		Email: "ada@example.com",
		Links: []Link{
			{Label: "GitHub", URL: "https://github.com/ada"},
			{Label: "ChainPal", URL: "https://github.com/ada/chainpal"},
			{Label: "Gist", URL: "https://gist.github.com/ada/x"},
		},
		Skills: []string{"Go"},
		Items:  []Item{{ID: "item-0", Title: "Engineer", Bullets: []Bullet{{ID: "item-0-b-0", Text: "Built it"}}}},
	}
}

func TestEditsFromReadsOnlyIdentityLinks(t *testing.T) {
	e := EditsFrom(storedProfile())
	if e.GitHub != "https://github.com/ada" {
		t.Fatalf("github slot: %q", e.GitHub)
	}
	if e.LinkedIn != "" || e.Portfolio != "" {
		t.Fatalf("empty slots got filled: %+v", e)
	}
}

// The editor owns a handful of facts; work history and skills are not among
// them and must survive a save untouched.
func TestApplyEditsLeavesTheRestAlone(t *testing.T) {
	got := ApplyEdits(storedProfile(), ProfileEdits{Name: "Ada Byron", Portfolio: "ada.dev"})

	if got.Name != "Ada Byron" {
		t.Fatalf("name: %q", got.Name)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "item-0" {
		t.Fatalf("history was touched: %+v", got.Items)
	}
	if len(got.Skills) != 1 {
		t.Fatalf("skills were touched: %+v", got.Skills)
	}
}

// Project links stay in the profile but out of the editor: replacing the
// identity slots must not delete them, and must not resurrect an old one.
func TestApplyEditsReplacesOnlyTheIdentitySlots(t *testing.T) {
	got := ApplyEdits(storedProfile(), ProfileEdits{
		Name:     "Ada",
		LinkedIn: "https://linkedin.com/in/ada",
	})

	var kinds []string
	for _, l := range got.Links {
		kinds = append(kinds, ClassifyLink(l))
	}
	var github, linkedin, other int
	for _, k := range kinds {
		switch k {
		case LinkGitHub:
			github++
		case LinkLinkedIn:
			linkedin++
		case LinkOther:
			other++
		}
	}
	if github != 0 {
		t.Error("an emptied GitHub slot should have removed the link")
	}
	if linkedin != 1 {
		t.Error("the LinkedIn slot did not save")
	}
	if other != 2 {
		t.Errorf("project links were dropped: %+v", got.Links)
	}
}

func TestApplyEditsCleansTheOptionalMaterial(t *testing.T) {
	got := ApplyEdits(storedProfile(), ProfileEdits{
		Name:           "Ada",
		Languages:      []string{" English ", "english", ""},
		Certifications: []Certification{{Name: " AWS "}, {Name: ""}},
	})
	if len(got.Languages) != 1 || got.Languages[0] != "English" {
		t.Fatalf("languages: %v", got.Languages)
	}
	if len(got.Certifications) != 1 || got.Certifications[0].Name != "AWS" {
		t.Fatalf("certifications: %+v", got.Certifications)
	}
}
