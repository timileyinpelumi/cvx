package model

import (
	"strings"
	"testing"
)

// A full page needs nothing said about it, and neither does one that was
// trimmed: trimming means there was more material than fit.
func TestPageAdviceStaysQuietWhenThePageIsFull(t *testing.T) {
	p := Profile{}
	if got := PageAdvice(p, 0.95, 0); got != nil {
		t.Fatalf("full page got advice: %v", got)
	}
	if got := PageAdvice(p, 0.6, 3); got != nil {
		t.Fatalf("a trimmed page got advice: %v", got)
	}
}

// A short page with a thin profile names the material that would fill it.
func TestPageAdviceNamesWhatIsMissing(t *testing.T) {
	got := strings.Join(PageAdvice(Profile{}, 0.62, 0), "; ")
	for _, want := range []string{"certification", "languages", "interest", "side project", "volunteering", "education"} {
		if !strings.Contains(got, want) {
			t.Errorf("advice missing %q: %s", want, got)
		}
	}
}

// A profile that already holds all of it gets the one piece of advice left:
// say more about the work already listed.
func TestPageAdviceFallsBackToMoreDetail(t *testing.T) {
	p := Profile{
		Certifications: []Certification{{Name: "AWS"}},
		Languages:      []string{"English"},
		Interests:      []string{"Open source"},
		Items: []Item{
			{Kind: "project"}, {Kind: "volunteering"}, {Kind: "education"},
		},
	}
	got := PageAdvice(p, 0.7, 0)
	if len(got) != 1 || !strings.Contains(got[0], "two bullets") {
		t.Fatalf("got %v", got)
	}
}
