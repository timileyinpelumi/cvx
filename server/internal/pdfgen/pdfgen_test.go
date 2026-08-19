package pdfgen

import (
	"bytes"
	"testing"

	"github.com/go-pdf/fpdf"

	"cvx/internal/model"
)

func fixture() (model.Profile, model.Tailored) {
	p := model.Profile{
		Name:     "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+1 555 0100",
		Location: "London, UK",
		Summary:  "Mathematician and writer.",
		Links:    []model.Link{{Label: "github.com/ada", URL: "https://github.com/ada"}},
		Skills:   []string{"Python", "Analytical Engines"},
		Items: []model.Item{
			{
				Kind:         "experience",
				Title:        "Engineer",
				Organization: "Analytical Engines Co",
				StartDate:    "2021-01",
				EndDate:      "",
				Bullets: []model.Bullet{
					{Text: "Built the engine"},
					{Text: "Wrote the first algorithm"},
				},
			},
		},
	}
	model.AssignIDs(&p)

	ta := model.Tailored{
		TargetRole:     "Python Backend Engineer",
		Headline:       "Backend Engineer specializing in numerical computing",
		Summary:        "Mathematician and writer with deep systems experience.",
		SelectedSkills: []string{"Python"},
		Sections: []model.TSection{
			{
				Title: "Experience",
				Items: []model.TItem{
					{
						SourceID:     p.Items[0].ID,
						Title:        "Engineer",
						Organization: "Analytical Engines Co",
						Dates:        "2021 – Present",
						Bullets: []model.TBullet{
							{SourceBulletID: p.Items[0].Bullets[0].ID, Text: "Built the engine in Python"},
							{SourceBulletID: p.Items[0].Bullets[1].ID, Text: "Wrote the first published algorithm"},
						},
					},
				},
			},
		},
		Gaps:        []model.Gap{{Requirement: "Django", Evidence: "not in profile", Severity: "missing"}},
		WhatChanged: []string{"led with Python"},
	}
	return p, ta
}

func TestRender(t *testing.T) {
	p, ta := fixture()
	b, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(b))
	}
}

func TestRenderEmptySections(t *testing.T) {
	p, ta := fixture()
	ta.Sections = nil
	b, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(b))
	}
}

// longTitle/longOrg combine to ~206mm, well past the ~152mm titleWidth
// budget on the A4/18mm-margin layout (verified: budget is usableWidth minus
// the "2021 – Present"-sized date column, ~151.64mm here), so they reliably
// exercise the wrap branch in renderItem rather than fitting on one line.
const (
	longTitle = "Director of Engineering, Distributed Systems and Cloud Platform Architecture"
	longOrg   = "Global Technology Solutions International Incorporated"
)

// TestRenderLongTitleWraps is an end-to-end smoke test: a "Title — Org" combo
// wider than the space left of the date column must render without error
// instead of erroring out or corrupting the document.
func TestRenderLongTitleWraps(t *testing.T) {
	p, ta := fixture()
	ta.Sections[0].Items[0].Title = longTitle
	ta.Sections[0].Items[0].Organization = longOrg

	b, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(b))
	}
}

// TestTitleNeedsWrap directly unit-tests the wrap-decision helper against the
// exact strings used by TestRenderLongTitleWraps / TestRenderItemWrapsLongTitle,
// confirming the long combo is actually over budget and the short one isn't.
func TestTitleNeedsWrap(t *testing.T) {
	pdf := newTestPDF()
	pdf.SetFont(fontFamily, "B", resolveTheme(DefaultStyle()).itemPt)

	const titleWidth = 151.64 // usableWidth minus a "2021 – Present" date column, per production math

	short := "Engineer — Analytical Engines Co"
	long := longTitle + " — " + longOrg

	if titleNeedsWrap(pdf, short, titleWidth) {
		t.Fatalf("expected short title to fit within %.2fmm", titleWidth)
	}
	if !titleNeedsWrap(pdf, long, titleWidth) {
		t.Fatalf("expected long title to exceed %.2fmm (got width %.2fmm)", titleWidth, pdf.GetStringWidth(long))
	}
}

// TestRenderItemWrapsLongTitle is the regression guard: it calls renderItem
// directly (white-box, same package) and measures how far it advances the
// cursor. A wrapped 2-line title must consume noticeably more vertical space
// than a single-line title renders in — if the wrap branch in renderItem were
// deleted (reverting to a single CellFormat that just overflows into the date
// column), the long-title item would advance the cursor by exactly one line,
// same as the short-title item, and this test would fail. Verified manually:
// see task-5-report.md "delete-branch-verify" note.
func TestRenderItemWrapsLongTitle(t *testing.T) {
	shortItem := model.TItem{Title: "Engineer", Organization: "Analytical Engines Co", Dates: "2021 – Present"}
	longItem := model.TItem{Title: longTitle, Organization: longOrg, Dates: "2021 – Present"}

	cfg := resolveTheme(DefaultStyle())
	shortPDF := newTestPDF()
	y0 := shortPDF.GetY()
	renderItem(shortPDF, cfg, shortItem)
	if err := shortPDF.Error(); err != nil {
		t.Fatal(err)
	}
	shortDelta := shortPDF.GetY() - y0

	longPDF := newTestPDF()
	y1 := longPDF.GetY()
	renderItem(longPDF, cfg, longItem)
	if err := longPDF.Error(); err != nil {
		t.Fatal(err)
	}
	longDelta := longPDF.GetY() - y1

	singleLine := lineHeight(cfg.itemPt, cfg.leading)

	if longDelta <= shortDelta {
		t.Fatalf("expected long-title item to consume more vertical space than short-title item: long=%.2fmm short=%.2fmm", longDelta, shortDelta)
	}
	if longDelta < 2*singleLine {
		t.Fatalf("expected long title to wrap to at least 2 lines: consumed %.2fmm, single line=%.2fmm", longDelta, singleLine)
	}
}

// TestRenderSelectedSkills is the regression guard for the Skills section:
// tailored output with SelectedSkills set must render a larger document than
// the same output with SelectedSkills cleared, proving the section (and its
// joined "  ·  " line) is actually emitted rather than silently dropped.
func TestRenderSelectedSkills(t *testing.T) {
	p, ta := fixture()
	ta.SelectedSkills = []string{"Python", "Distributed Systems", "PostgreSQL"}

	withSkills, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(withSkills, []byte("%PDF")) || len(withSkills) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(withSkills))
	}

	ta.SelectedSkills = nil
	withoutSkills, err := Render(p, ta, DefaultStyle())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(withoutSkills, []byte("%PDF")) || len(withoutSkills) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(withoutSkills))
	}

	if bytes.Equal(withSkills, withoutSkills) {
		t.Fatal("expected rendering with SelectedSkills to differ from rendering without")
	}
	if len(withSkills) <= len(withoutSkills) {
		t.Fatalf("expected pdf with SelectedSkills to be larger: with=%d without=%d", len(withSkills), len(withoutSkills))
	}
}

// newTestPDF builds an Fpdf with the same margins/fonts as Render, positioned
// on a fresh page, for tests that exercise unexported render functions
// directly instead of going through Render.
func newTestPDF() *fpdf.Fpdf {
	cfg := resolveTheme(DefaultStyle())
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(cfg.marginSide, cfg.marginTop, cfg.marginSide)
	pdf.SetAutoPageBreak(true, cfg.marginBottom)
	pdf.AddUTF8FontFromBytes(fontFamily, "", regularFont)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", semiboldFont)
	pdf.AddPage()
	return pdf
}

// TestStretchFillsSparsePage: the fixture is a one-item resume, well under a
// page at base spacing. The refit pass must raise the fill without spilling
// to a second page, and Render must take that path (its fill beats a raw
// typeset's).
func TestStretchFillsSparsePage(t *testing.T) {
	p, ta := fixture()
	cfg := resolveTheme(DefaultStyle())

	_, pages, fill := typeset(p, ta, cfg, false)
	if pages != 1 {
		t.Fatalf("fixture should be one page, got %d", pages)
	}
	if fill >= minFill {
		t.Fatalf("fixture should be under-full, got fill %.2f", fill)
	}

	_, pages2, fill2 := typeset(p, ta, stretched(cfg, fill), false)
	if pages2 != 1 {
		t.Fatalf("stretched layout spilled to %d pages", pages2)
	}
	if fill2 <= fill {
		t.Fatalf("stretch did not raise fill: %.2f -> %.2f", fill, fill2)
	}

	if _, err := Render(p, ta, DefaultStyle()); err != nil {
		t.Fatal(err)
	}
}

// TestTightenedReducesSpacing pins the tighten pass's direction.
func TestTightenedReducesSpacing(t *testing.T) {
	cfg := resolveTheme(DefaultStyle())
	tight := tightened(cfg)
	if tight.leading >= cfg.leading || tight.gapSection >= cfg.gapSection {
		t.Fatalf("tightened did not reduce spacing: %+v vs %+v", tight, cfg)
	}
}
