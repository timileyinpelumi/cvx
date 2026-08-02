package pdfgen

import (
	"bytes"

	"github.com/go-pdf/fpdf"

	"cvx/internal/model"
)

const (
	coverBodySize         = 10.0
	coverLineHeightFactor = 1.5
	coverHeaderGap        = 6.0
	coverParagraphGap     = 4.0
	coverClosingGap       = 4.0
)

// coverLineHeight applies the cover letter's wider 1.5 line-height multiplier
// (vs. the resume's 1.35) to a point font size.
func coverLineHeight(sizePt float64) float64 {
	const ptToMM = 0.352778
	return sizePt * ptToMM * coverLineHeightFactor
}

// RenderCoverLetter typesets a cover letter into a PDF using the same theme
// (fonts, accent, margins) as Render (the resume), so the two documents read
// as a pair. role is accepted for signature symmetry with the AI generation
// call but is not itself rendered — cl already carries all the letter's
// prose. Body copy stays at coverBodySize in every theme for readability.
func RenderCoverLetter(p model.Profile, role string, cl model.CoverLetter, style Style) ([]byte, error) {
	cfg := resolveTheme(style)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(cfg.marginSide, cfg.marginTop, cfg.marginSide)
	pdf.SetAutoPageBreak(true, cfg.marginBottom)

	registerFonts(pdf)

	pdf.AddPage()

	w := usableWidth(pdf)

	if cfg.nameInAccent {
		pdf.SetTextColor(cfg.accentR, cfg.accentG, cfg.accentB)
	} else {
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.SetFont(cfg.displayFamily, "B", cfg.namePt)
	pdf.CellFormat(w, lineHeight(cfg.namePt, cfg.leading), p.Name, "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	contact := contactLine(p)
	if contact != "" {
		pdf.SetFont(cfg.bodyFamily, "", cfg.contactPt)
		pdf.CellFormat(w, lineHeight(cfg.contactPt, cfg.leading), contact, "", 1, "L", false, 0, "")
	}

	pdf.Ln(coverHeaderGap)

	pdf.SetFont(cfg.bodyFamily, "", coverBodySize)
	if cl.Greeting != "" {
		pdf.MultiCell(w, coverLineHeight(coverBodySize), cl.Greeting, "", "L", false)
		pdf.Ln(coverParagraphGap)
	}

	for _, para := range cl.Paragraphs {
		if para == "" {
			continue
		}
		pdf.MultiCell(w, coverLineHeight(coverBodySize), para, "", "L", false)
		pdf.Ln(coverParagraphGap)
	}

	if cl.Closing != "" {
		pdf.MultiCell(w, coverLineHeight(coverBodySize), cl.Closing, "", "L", false)
		pdf.Ln(coverClosingGap)
	}

	pdf.SetFont(cfg.bodyFamily, "B", coverBodySize)
	pdf.MultiCell(w, coverLineHeight(coverBodySize), p.Name, "", "L", false)

	if err := pdf.Error(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
