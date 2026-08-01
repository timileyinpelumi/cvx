// Package pdfgen renders a Profile+Tailored pair into a single-page-oriented,
// ATS-safe PDF: text only, no tables/images/color blocks, one column.
package pdfgen

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"

	"cvx/internal/model"
)

//go:embed fonts/SourceSans3-Regular.ttf
var regularFont []byte

//go:embed fonts/SourceSans3-Semibold.ttf
var semiboldFont []byte

const fontFamily = "SourceSans3"

const (
	marginSide   = 18.0
	marginTop    = 16.0
	marginBottom = 16.0
)

const (
	nameSize     = 19.0
	headlineSize = 10.5
	contactSize  = 8.5
	summarySize  = 9.5
	sectionSize  = 10.0
	itemSize     = 10.0
	dateSize     = 9.5
	bulletSize   = 9.5
	skillsSize   = 9.5
)

const lineHeightFactor = 1.35

const (
	gapBeforeSection  = 6.0
	gapBetweenItems   = 3.0
	gapBetweenBullets = 1.2
	bulletIndent      = 4.0
	ruleGap           = 1.2 // space between section title baseline and the rule under it
	ruleToBody        = 2.0 // space between the rule and the first item below it
)

// orphanMinRemaining is the minimum vertical space (mm) that must remain on
// the page before a section title; below this, force a page break so the
// title never lands alone at the bottom of a page.
const orphanMinRemaining = 30.0

// 60% gray, expressed as ink coverage: 0% = white, 100% = black.
const grayInkPct = 0.60

func grayComponent() int {
	return int(255 * (1 - grayInkPct))
}

// lineHeight converts a point font size to a line height in mm, applying the
// spec's 1.35 line-height multiplier.
func lineHeight(sizePt float64) float64 {
	const ptToMM = 0.352778
	return sizePt * ptToMM * lineHeightFactor
}

// Render typesets p and t into a PDF and returns the raw document bytes.
func Render(p model.Profile, t model.Tailored) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginSide, marginTop, marginSide)
	pdf.SetAutoPageBreak(true, marginBottom)

	pdf.AddUTF8FontFromBytes(fontFamily, "", regularFont)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", semiboldFont)

	pdf.AddPage()

	renderHeader(pdf, p, t)
	renderSkillsSection(pdf, t.SelectedSkills)
	for _, section := range t.Sections {
		renderSection(pdf, section)
	}

	if err := pdf.Error(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func usableWidth(pdf *fpdf.Fpdf) float64 {
	pageW, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	return pageW - left - right
}

func renderHeader(pdf *fpdf.Fpdf, p model.Profile, t model.Tailored) {
	w := usableWidth(pdf)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont(fontFamily, "B", nameSize)
	pdf.CellFormat(w, lineHeight(nameSize), p.Name, "", 1, "L", false, 0, "")

	if t.Headline != "" {
		gray := grayComponent()
		pdf.SetTextColor(gray, gray, gray)
		pdf.SetFont(fontFamily, "", headlineSize)
		pdf.CellFormat(w, lineHeight(headlineSize), t.Headline, "", 1, "L", false, 0, "")
	}

	contact := contactLine(p)
	if contact != "" {
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont(fontFamily, "", contactSize)
		pdf.CellFormat(w, lineHeight(contactSize), contact, "", 1, "L", false, 0, "")
	}

	if t.Summary != "" {
		pdf.Ln(1.5)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont(fontFamily, "", summarySize)
		pdf.MultiCell(w, lineHeight(summarySize), t.Summary, "", "L", false)
	}
}

func contactLine(p model.Profile) string {
	var parts []string
	if p.Email != "" {
		parts = append(parts, p.Email)
	}
	if p.Phone != "" {
		parts = append(parts, p.Phone)
	}
	if p.Location != "" {
		parts = append(parts, p.Location)
	}
	for _, l := range p.Links {
		if l.URL == "" {
			continue
		}
		if l.Label != "" {
			parts = append(parts, l.Label)
		} else {
			parts = append(parts, l.URL)
		}
	}
	return strings.Join(parts, "  ·  ")
}

// renderSectionTitle draws a section title (10pt SemiBold, uppercase) with
// the 0.2mm 60%-gray rule beneath it, leaving the cursor positioned for the
// section body. Shared by renderSection and renderSkillsSection so both
// section kinds render the exact same title style.
func renderSectionTitle(pdf *fpdf.Fpdf, title string) {
	ensureRoomForSectionTitle(pdf)

	pdf.Ln(gapBeforeSection)
	w := usableWidth(pdf)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont(fontFamily, "B", sectionSize)
	pdf.CellFormat(w, lineHeight(sectionSize), strings.ToUpper(title), "", 1, "L", false, 0, "")

	pdf.Ln(ruleGap)
	gray := grayComponent()
	pdf.SetDrawColor(gray, gray, gray)
	pdf.SetLineWidth(0.2)
	left, _, _, _ := pdf.GetMargins()
	y := pdf.GetY()
	pdf.Line(left, y, left+w, y)
	pdf.Ln(ruleToBody)
}

func renderSection(pdf *fpdf.Fpdf, s model.TSection) {
	renderSectionTitle(pdf, s.Title)

	for i, item := range s.Items {
		if i > 0 {
			pdf.Ln(gapBetweenItems)
		}
		renderItem(pdf, item)
	}
}

// renderSkillsSection renders the AI-selected skills as a single wrapped
// line under a "Skills" section title, matching the section-title style used
// elsewhere. It is a no-op when skills is empty so tailored output with no
// selected skills doesn't grow an empty section.
func renderSkillsSection(pdf *fpdf.Fpdf, skills []string) {
	if len(skills) == 0 {
		return
	}
	renderSectionTitle(pdf, "Skills")

	w := usableWidth(pdf)
	pdf.SetFont(fontFamily, "", skillsSize)
	pdf.SetTextColor(0, 0, 0)
	pdf.MultiCell(w, lineHeight(skillsSize), strings.Join(skills, "  ·  "), "", "L", false)
}

// ensureRoomForSectionTitle breaks to a new page before a section title if
// less than orphanMinRemaining space remains, so a title never sits alone at
// the bottom of a page.
func ensureRoomForSectionTitle(pdf *fpdf.Fpdf) {
	_, pageH := pdf.GetPageSize()
	_, _, _, bottom := pdf.GetMargins()
	remaining := pageH - bottom - pdf.GetY()
	if remaining < orphanMinRemaining {
		pdf.AddPage()
	}
}

// titleNeedsWrap reports whether title (rendered in the currently-set font)
// is too wide to fit in titleWidth on a single line, i.e. whether it would
// bleed into the date column if drawn with a plain CellFormat.
func titleNeedsWrap(pdf *fpdf.Fpdf, title string, titleWidth float64) bool {
	return pdf.GetStringWidth(title) > titleWidth
}

func renderItem(pdf *fpdf.Fpdf, it model.TItem) {
	w := usableWidth(pdf)

	title := it.Title
	if it.Organization != "" {
		title = fmt.Sprintf("%s — %s", it.Title, it.Organization)
	}

	h := lineHeight(itemSize)
	dateWidth := 0.0
	if it.Dates != "" {
		pdf.SetFont(fontFamily, "", dateSize)
		dateWidth = pdf.GetStringWidth(it.Dates) + 2
	}
	titleWidth := w - dateWidth

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont(fontFamily, "B", itemSize)

	if titleNeedsWrap(pdf, title, titleWidth) {
		// Title too wide for the space left of the date column: wrap it
		// within titleWidth (held constant across wrapped lines) via
		// MultiCell so it can never bleed into the date column, and place
		// the date on the first baseline only.
		x, y := pdf.GetX(), pdf.GetY()
		pdf.MultiCell(titleWidth, h, title, "", "L", false)
		endY := pdf.GetY()

		if it.Dates != "" {
			pdf.SetFont(fontFamily, "", dateSize)
			pdf.SetXY(x+titleWidth, y)
			pdf.CellFormat(dateWidth, h, it.Dates, "", 0, "R", false, 0, "")
		}
		pdf.SetXY(x, endY)
	} else {
		pdf.CellFormat(titleWidth, h, title, "", 0, "L", false, 0, "")
		pdf.SetFont(fontFamily, "", dateSize)
		pdf.CellFormat(dateWidth, h, it.Dates, "", 1, "R", false, 0, "")
	}

	if len(it.Bullets) > 0 {
		pdf.Ln(0.8)
	}
	for i, b := range it.Bullets {
		if i > 0 {
			pdf.Ln(gapBetweenBullets)
		}
		renderBullet(pdf, b)
	}
}

func renderBullet(pdf *fpdf.Fpdf, b model.TBullet) {
	w := usableWidth(pdf)
	left, _, _, _ := pdf.GetMargins()
	h := lineHeight(bulletSize)

	pdf.SetFont(fontFamily, "", bulletSize)
	pdf.SetTextColor(0, 0, 0)

	pdf.SetX(left)
	pdf.CellFormat(bulletIndent, h, "•", "", 0, "L", false, 0, "")
	pdf.SetX(left + bulletIndent)
	pdf.MultiCell(w-bulletIndent, h, b.Text, "", "L", false)
}
