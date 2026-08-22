package pdfgen

import (
	"strings"
	"testing"

	"cvx/internal/model"
	"cvx/internal/pdftext"
)

func TestVerifyPassesOnARealRender(t *testing.T) {
	p, ta := fixture()
	pdf, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if issues := Verify(pdf, p, ta); len(issues) > 0 {
		t.Fatalf("a freshly rendered resume must read back clean:\n%s", strings.Join(issues, "\n"))
	}
}

// The whole point: text the pipeline believes is on the page but which never
// made it through rendering has to be caught mechanically.
func TestVerifyCatchesTextThatNeverRendered(t *testing.T) {
	p, ta := fixture()
	pdf, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}

	claimed := ta
	claimed.Summary = "This sentence was never rendered onto the page at all."
	issues := Verify(pdf, p, claimed)
	if len(issues) == 0 {
		t.Fatal("want the unrendered summary reported")
	}
	if !strings.Contains(issues[0], "summary") {
		t.Fatalf("want the summary named in the issue, got %q", issues[0])
	}
}

func TestVerifyRejectsUnreadablePDF(t *testing.T) {
	issues := Verify([]byte("not a pdf"), model.Profile{}, model.Tailored{})
	if len(issues) != 1 || !strings.Contains(issues[0], "ATS") {
		t.Fatalf("want one unreadable-file issue, got %v", issues)
	}
}

// A resume that spills onto a second page is a defect the standard exists to
// prevent, and it is invisible in every check that only looks at text.
func TestVerifyCatchesASecondPage(t *testing.T) {
	p, ta := fixture()
	item := ta.Sections[0].Items[0]
	for i := 0; i < 40; i++ {
		ta.Sections[0].Items = append(ta.Sections[0].Items, item)
	}
	pdf, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, issue := range Verify(pdf, p, ta) {
		if strings.Contains(issue, "pages") {
			found = true
		}
	}
	if !found {
		t.Fatal("want the page overflow reported")
	}
}

// Privacy is a rendering guarantee, not a UI preference: what the page does
// not say cannot be scraped off it.
func TestHiddenContactDetailsNeverReachThePage(t *testing.T) {
	p, ta := fixture()
	style := DefaultStyle()
	style.HidePhone, style.HideLocation = true, true

	pdf, err := Render(p, ta, style)
	if err != nil {
		t.Fatal(err)
	}
	text, err := pdftext.ExtractText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, p.Phone) {
		t.Error("phone number rendered despite HidePhone")
	}
	if strings.Contains(text, p.Location) {
		t.Error("location rendered despite HideLocation")
	}
	if !strings.Contains(text, p.Email) {
		t.Error("email must still be there: it is how anyone replies")
	}
}

// The point of over-selecting: a deep resume lands on exactly one page by
// dropping from the bottom of the tailor's ranking, not by refusing to
// select the material in the first place.
func TestRenderFitsOnePageByTrimming(t *testing.T) {
	p, ta := fixture()
	item := ta.Sections[0].Items[0]
	for i := 0; i < 12; i++ {
		ta.Sections[0].Items = append(ta.Sections[0].Items, item)
	}
	ta.Interests = []string{"Open source"}
	ta.Languages = []string{"English"}

	pdf, layout, err := RenderWithLayout(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if pages := pdftext.PageCount(pdf); pages != 1 {
		t.Fatalf("want one page, got %d (trimmed %d)", pages, len(layout.Trimmed))
	}
	if len(layout.Trimmed) == 0 {
		t.Fatal("a resume this long must have been trimmed to fit")
	}
	if layout.Trimmed[0] != "interests" {
		t.Fatalf("the weakest line should go first, got %v", layout.Trimmed)
	}
}

// A thin resume keeps everything it has, and the optional one-line sections
// are on the page rather than held back.
func TestRenderKeepsThinResumeIntact(t *testing.T) {
	p, ta := fixture()
	ta.Certifications = []string{"AWS Solutions Architect"}
	ta.Languages = []string{"English", "Yoruba"}
	ta.Interests = []string{"Open source"}

	pdf, layout, err := RenderWithLayout(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Trimmed) != 0 {
		t.Fatalf("nothing should be trimmed from a thin resume: %v", layout.Trimmed)
	}
	text, err := pdftext.ExtractText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AWS Solutions Architect", "Yoruba", "Open source"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q never reached the page", want)
		}
	}
}
