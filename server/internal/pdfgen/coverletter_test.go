package pdfgen

import (
	"bytes"
	"testing"

	"cvx/internal/model"
)

func coverLetterFixture() model.CoverLetter {
	return model.CoverLetter{
		Greeting: "Dear hiring team,",
		Paragraphs: []string{
			"I am writing to apply for the Python Backend Engineer role.",
			"My experience building the analytical engine aligns well with this position.",
		},
		Closing: "Sincerely,",
	}
}

func TestRenderCoverLetter(t *testing.T) {
	p, _ := fixture()
	cl := coverLetterFixture()

	b, err := RenderCoverLetter(p, "Python Backend Engineer", cl)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 1000 {
		t.Fatalf("bad pdf: %d bytes", len(b))
	}
}

// TestRenderCoverLetterEmptyParagraphs checks that a cover letter with no
// paragraphs (e.g. an LLM that returned an empty list) still renders a valid
// document with the header, greeting, and closing rather than erroring out.
func TestRenderCoverLetterEmptyParagraphs(t *testing.T) {
	p, _ := fixture()
	cl := coverLetterFixture()
	cl.Paragraphs = nil

	b, err := RenderCoverLetter(p, "Python Backend Engineer", cl)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || len(b) < 500 {
		t.Fatalf("bad pdf: %d bytes", len(b))
	}
}

// TestRenderCoverLetterWithParagraphsIsLarger proves the paragraph loop
// actually emits content: a letter with paragraphs must produce a larger
// document than the same letter with paragraphs cleared.
func TestRenderCoverLetterWithParagraphsIsLarger(t *testing.T) {
	p, _ := fixture()

	withParas := coverLetterFixture()
	withBytes, err := RenderCoverLetter(p, "Python Backend Engineer", withParas)
	if err != nil {
		t.Fatal(err)
	}

	withoutParas := coverLetterFixture()
	withoutParas.Paragraphs = nil
	withoutBytes, err := RenderCoverLetter(p, "Python Backend Engineer", withoutParas)
	if err != nil {
		t.Fatal(err)
	}

	if len(withBytes) <= len(withoutBytes) {
		t.Fatalf("expected pdf with paragraphs to be larger: with=%d without=%d", len(withBytes), len(withoutBytes))
	}
}
