package pdftext

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
)

func makeTestPDF(t *testing.T, text string) []byte {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "", 16)
	pdf.Cell(40, 10, text)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("generate test pdf: %v", err)
	}
	return buf.Bytes()
}

func TestExtractText(t *testing.T) {
	pdfBytes := makeTestPDF(t, "Golang Engineer")

	got, err := ExtractText(pdfBytes)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(got, "Golang Engineer") {
		t.Fatalf("expected extracted text to contain %q, got %q", "Golang Engineer", got)
	}
}

func TestExtractTextGarbage(t *testing.T) {
	_, err := ExtractText([]byte("not a pdf at all"))
	if err == nil {
		t.Fatal("expected error for garbage bytes")
	}
}
