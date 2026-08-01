package ai

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
