// Package pdftext turns a PDF back into plain text. It is used in both
// directions: to read an uploaded resume for providers that cannot take a
// PDF part, and to read cvx's own output back to check what actually landed
// on the page.
package pdftext

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractText pulls plain text out of a PDF for providers that can't take a
// PDF file part natively (they get the extracted text as a regular text
// block instead).
func ExtractText(pdfBytes []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}

	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extract page %d: %w", i, err)
		}
		buf.WriteString(text)
	}

	text := strings.TrimSpace(buf.String())
	if text == "" {
		return "", fmt.Errorf("no extractable text in PDF (is it scanned?)")
	}
	return text, nil
}

// PageCount reports how many pages a PDF has, or 0 when it cannot be read.
// Used to hold generated resumes to the one-page standard.
func PageCount(pdfBytes []byte) int {
	r, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return 0
	}
	return r.NumPage()
}
