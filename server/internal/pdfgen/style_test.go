package pdfgen

import "testing"

func TestValidateAndNormalize(t *testing.T) {
	if err := DefaultStyle().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := Style{Theme: "neon", Accent: "#123456", Density: "loose"}
	if err := bad.Validate(); err == nil {
		t.Fatal("want error for invalid style")
	}
	if n := bad.Normalized(); n != DefaultStyle() {
		t.Fatalf("want defaults, got %+v", n)
	}
	keep := Style{Theme: "classic", Accent: "#0F766E", Density: "tight", SkillsFirst: true}
	if got := keep.Normalized(); got != keep {
		t.Fatalf("valid style must survive Normalized, got %+v", got)
	}
}

func TestRenderAllThemes(t *testing.T) {
	p, ta := fixture()
	var outputs [][]byte
	for _, style := range []Style{
		{Theme: "classic", Accent: "#2244D9", Density: "normal"},
		{Theme: "modern", Accent: "#2244D9", Density: "normal"},
		{Theme: "compact", Accent: "#2244D9", Density: "normal"},
		{Theme: "modern", Accent: "#B3341E", Density: "tight", SkillsFirst: true},
	} {
		pdf, err := Render(p, ta, style)
		if err != nil {
			t.Fatalf("%+v: %v", style, err)
		}
		if len(pdf) < 1000 || string(pdf[:5]) != "%PDF-" {
			t.Fatalf("%+v: implausible pdf (%d bytes)", style, len(pdf))
		}
		outputs = append(outputs, pdf)
	}
	if string(outputs[0]) == string(outputs[1]) || string(outputs[1]) == string(outputs[2]) {
		t.Fatal("themes must produce different documents")
	}
}

func TestRenderTwoColSkills(t *testing.T) {
	p, ta := fixture()
	ta.SelectedSkills = []string{"Go", "Python", "SQL", "Docker", "Redis", "Kafka", "Terraform"}
	pdf, err := Render(p, ta, Style{Theme: "compact", Accent: "#1C2422", Density: "normal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) < 1000 || string(pdf[:5]) != "%PDF-" {
		t.Fatalf("implausible pdf (%d bytes)", len(pdf))
	}
}
