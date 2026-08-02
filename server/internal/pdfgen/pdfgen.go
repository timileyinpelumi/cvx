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

//go:embed fonts/SourceSerif4-Regular.ttf
var serifRegularFont []byte

//go:embed fonts/SourceSerif4-Semibold.ttf
var serifSemiboldFont []byte

const fontFamily = "SourceSans3"
const serifFamily = "SourceSerif4"

const (
	bulletIndent = 4.0
	ruleGap      = 1.2 // space between section title baseline and the rule under it
	ruleToBody   = 2.0 // space between the rule and the first item below it
)

// 60% gray, expressed as ink coverage: 0% = white, 100% = black.
const grayInkPct = 0.60

func grayComponent() int {
	return int(255 * (1 - grayInkPct))
}

// lineHeight converts a point font size to a line height in mm, applying the
// theme's line-height multiplier.
func lineHeight(sizePt, leading float64) float64 {
	const ptToMM = 0.352778
	return sizePt * ptToMM * leading
}

func registerFonts(pdf *fpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes(fontFamily, "", regularFont)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", semiboldFont)
	pdf.AddUTF8FontFromBytes(serifFamily, "", serifRegularFont)
	pdf.AddUTF8FontFromBytes(serifFamily, "B", serifSemiboldFont)
}

// Render typesets p and t into a PDF in the given style and returns the raw
// document bytes.
func Render(p model.Profile, t model.Tailored, style Style) ([]byte, error) {
	cfg := resolveTheme(style)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(cfg.marginSide, cfg.marginTop, cfg.marginSide)
	pdf.SetAutoPageBreak(true, cfg.marginBottom)

	registerFonts(pdf)

	pdf.AddPage()

	renderHeader(pdf, cfg, p, t)
	if style.Normalized().SkillsFirst {
		renderSkillsSection(pdf, cfg, t.SelectedSkills)
	}
	for _, section := range t.Sections {
		renderSection(pdf, cfg, section)
	}
	if !style.Normalized().SkillsFirst {
		renderSkillsSection(pdf, cfg, t.SelectedSkills)
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

func renderHeader(pdf *fpdf.Fpdf, cfg theme, p model.Profile, t model.Tailored) {
	w := usableWidth(pdf)

	if cfg.nameInAccent {
		pdf.SetTextColor(cfg.accentR, cfg.accentG, cfg.accentB)
	} else {
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.SetFont(cfg.displayFamily, "B", cfg.namePt)
	pdf.CellFormat(w, lineHeight(cfg.namePt, cfg.leading), p.Name, "", 1, "L", false, 0, "")

	if t.Headline != "" {
		gray := grayComponent()
		pdf.SetTextColor(gray, gray, gray)
		pdf.SetFont(cfg.bodyFamily, "", cfg.headlinePt)
		pdf.CellFormat(w, lineHeight(cfg.headlinePt, cfg.leading), t.Headline, "", 1, "L", false, 0, "")
	}

	contact := contactLine(p)
	if contact != "" {
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont(cfg.bodyFamily, "", cfg.contactPt)
		pdf.CellFormat(w, lineHeight(cfg.contactPt, cfg.leading), contact, "", 1, "L", false, 0, "")
	}

	if t.Summary != "" {
		pdf.Ln(1.5)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont(cfg.bodyFamily, "", cfg.summaryPt)
		pdf.MultiCell(w, lineHeight(cfg.summaryPt, cfg.leading), t.Summary, "", "L", false)
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

// renderSectionTitle draws a section title (uppercase, display face) with the
// 0.2mm rule beneath it, leaving the cursor positioned for the section body.
// Shared by renderSection and renderSkillsSection so both section kinds
// render the exact same title style.
func renderSectionTitle(pdf *fpdf.Fpdf, cfg theme, title string) {
	ensureRoomForSectionTitle(pdf, cfg)

	pdf.Ln(cfg.gapSection + cfg.sectionTitlePad)
	w := usableWidth(pdf)

	if cfg.titlesInAccent {
		pdf.SetTextColor(cfg.accentR, cfg.accentG, cfg.accentB)
	} else {
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.SetFont(cfg.displayFamily, "B", cfg.sectionPt)
	pdf.CellFormat(w, lineHeight(cfg.sectionPt, cfg.leading), strings.ToUpper(title), "", 1, "L", false, 0, "")

	pdf.Ln(ruleGap)
	if cfg.rulesInAccent {
		pdf.SetDrawColor(cfg.accentR, cfg.accentG, cfg.accentB)
	} else {
		gray := grayComponent()
		pdf.SetDrawColor(gray, gray, gray)
	}
	pdf.SetLineWidth(0.2)
	left, _, _, _ := pdf.GetMargins()
	y := pdf.GetY()
	pdf.Line(left, y, left+w, y)
	pdf.Ln(ruleToBody)
}

func renderSection(pdf *fpdf.Fpdf, cfg theme, s model.TSection) {
	renderSectionTitle(pdf, cfg, s.Title)

	for i, item := range s.Items {
		if i > 0 {
			pdf.Ln(cfg.gapItems)
		}
		renderItem(pdf, cfg, item)
	}
}

// renderSkillsSection renders the AI-selected skills under a "Skills" section
// title, matching the section-title style used elsewhere. It is a no-op when
// skills is empty so tailored output with no selected skills doesn't grow an
// empty section. Compact lays the list out in two columns.
func renderSkillsSection(pdf *fpdf.Fpdf, cfg theme, skills []string) {
	if len(skills) == 0 {
		return
	}
	renderSectionTitle(pdf, cfg, "Skills")

	if cfg.twoColSkills && len(skills) > 3 {
		renderSkillsTwoCol(pdf, cfg, skills)
		return
	}

	w := usableWidth(pdf)
	pdf.SetFont(cfg.bodyFamily, "", cfg.skillsPt)
	pdf.SetTextColor(0, 0, 0)
	pdf.MultiCell(w, lineHeight(cfg.skillsPt, cfg.leading), strings.Join(skills, "  ·  "), "", "L", false)
}

// renderSkillsTwoCol lays the skills out as two side-by-side columns,
// splitting the list in half by count.
func renderSkillsTwoCol(pdf *fpdf.Fpdf, cfg theme, skills []string) {
	w := usableWidth(pdf)
	left, _, _, _ := pdf.GetMargins()
	colW := (w - 4) / 2
	mid := (len(skills) + 1) / 2
	cols := [2][]string{skills[:mid], skills[mid:]}

	pdf.SetFont(cfg.bodyFamily, "", cfg.skillsPt)
	pdf.SetTextColor(0, 0, 0)
	startY := pdf.GetY()
	h := lineHeight(cfg.skillsPt, cfg.leading)
	endY := startY
	for i, col := range cols {
		x := left + float64(i)*(colW+4)
		pdf.SetXY(x, startY)
		pdf.MultiCell(colW, h, strings.Join(col, "  ·  "), "", "L", false)
		if y := pdf.GetY(); y > endY {
			endY = y
		}
	}
	pdf.SetXY(left, endY)
}

// ensureRoomForSectionTitle breaks to a new page before a section title if
// less than the theme's orphan threshold remains, so a title never sits alone
// at the bottom of a page.
func ensureRoomForSectionTitle(pdf *fpdf.Fpdf, cfg theme) {
	_, pageH := pdf.GetPageSize()
	_, _, _, bottom := pdf.GetMargins()
	remaining := pageH - bottom - pdf.GetY()
	if remaining < cfg.orphanMinRemain {
		pdf.AddPage()
	}
}

// titleNeedsWrap reports whether title (rendered in the currently-set font)
// is too wide to fit in titleWidth on a single line, i.e. whether it would
// bleed into the date column if drawn with a plain CellFormat.
func titleNeedsWrap(pdf *fpdf.Fpdf, title string, titleWidth float64) bool {
	return pdf.GetStringWidth(title) > titleWidth
}

func renderItem(pdf *fpdf.Fpdf, cfg theme, it model.TItem) {
	w := usableWidth(pdf)

	title := it.Title
	if it.Organization != "" {
		title = fmt.Sprintf("%s — %s", it.Title, it.Organization)
	}

	h := lineHeight(cfg.itemPt, cfg.leading)
	dateWidth := 0.0
	if it.Dates != "" {
		pdf.SetFont(cfg.bodyFamily, "", cfg.datePt)
		dateWidth = pdf.GetStringWidth(it.Dates) + 2
	}
	titleWidth := w - dateWidth

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont(cfg.bodyFamily, "B", cfg.itemPt)

	if titleNeedsWrap(pdf, title, titleWidth) {
		// Title too wide for the space left of the date column: wrap it
		// within titleWidth (held constant across wrapped lines) via
		// MultiCell so it can never bleed into the date column, and place
		// the date on the first baseline only.
		x, y := pdf.GetX(), pdf.GetY()
		pdf.MultiCell(titleWidth, h, title, "", "L", false)
		endY := pdf.GetY()

		if it.Dates != "" {
			pdf.SetFont(cfg.bodyFamily, "", cfg.datePt)
			pdf.SetXY(x+titleWidth, y)
			pdf.CellFormat(dateWidth, h, it.Dates, "", 0, "R", false, 0, "")
		}
		pdf.SetXY(x, endY)
	} else {
		pdf.CellFormat(titleWidth, h, title, "", 0, "L", false, 0, "")
		pdf.SetFont(cfg.bodyFamily, "", cfg.datePt)
		pdf.CellFormat(dateWidth, h, it.Dates, "", 1, "R", false, 0, "")
	}

	if len(it.Bullets) > 0 {
		pdf.Ln(0.8)
	}
	for i, b := range it.Bullets {
		if i > 0 {
			pdf.Ln(cfg.gapBullets)
		}
		renderBullet(pdf, cfg, b)
	}
}

func renderBullet(pdf *fpdf.Fpdf, cfg theme, b model.TBullet) {
	w := usableWidth(pdf)
	left, _, _, _ := pdf.GetMargins()
	h := lineHeight(cfg.bulletPt, cfg.leading)

	pdf.SetFont(cfg.bodyFamily, "", cfg.bulletPt)
	pdf.SetTextColor(0, 0, 0)

	pdf.SetX(left)
	pdf.CellFormat(bulletIndent, h, "•", "", 0, "L", false, 0, "")
	pdf.SetX(left + bulletIndent)
	pdf.MultiCell(w-bulletIndent, h, b.Text, "", "L", false)
}
