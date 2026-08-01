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

// RenderCoverLetter typesets a cover letter into a PDF using the same fonts
// and margins as Render (the resume), so the two documents read as a pair.
// role is accepted for signature symmetry with the AI generation call but is
// not itself rendered — cl already carries all the letter's prose.
func RenderCoverLetter(p model.Profile, role string, cl model.CoverLetter) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginSide, marginTop, marginSide)
	pdf.SetAutoPageBreak(true, marginBottom)

	pdf.AddUTF8FontFromBytes(fontFamily, "", regularFont)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", semiboldFont)

	pdf.AddPage()

	w := usableWidth(pdf)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont(fontFamily, "B", nameSize)
	pdf.CellFormat(w, lineHeight(nameSize), p.Name, "", 1, "L", false, 0, "")

	contact := contactLine(p)
	if contact != "" {
		pdf.SetFont(fontFamily, "", contactSize)
		pdf.CellFormat(w, lineHeight(contactSize), contact, "", 1, "L", false, 0, "")
	}

	pdf.Ln(coverHeaderGap)

	pdf.SetFont(fontFamily, "", coverBodySize)
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

	pdf.SetFont(fontFamily, "B", coverBodySize)
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
