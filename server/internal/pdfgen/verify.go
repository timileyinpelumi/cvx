package pdfgen

import (
	"fmt"
	"strings"

	"cvx/internal/model"
	"cvx/internal/pdftext"
)

// verifySampleChars is how much of a bullet has to survive the round trip.
// A whole bullet can legitimately differ at the tail (hyphenation, a trailing
// space swallowed by the extractor); a clipped or dropped bullet fails well
// inside this prefix.
const verifySampleChars = 40

// Verify reads the generated PDF back with the same extractor cvx uses on
// uploaded resumes and reports what did not survive: text that never made it
// onto the page, and a resume that spilled past one page. It is the
// mechanical version of squinting at the preview, and it doubles as an ATS
// check, because a parser that cannot read the file back is exactly what an
// applicant tracking system is.
//
// A nil result means the page says everything it was supposed to.
func Verify(pdfBytes []byte, p model.Profile, t model.Tailored) []string {
	text, err := pdftext.ExtractText(pdfBytes)
	if err != nil {
		return []string{fmt.Sprintf("the PDF has no extractable text (%v); an ATS would read nothing", err)}
	}
	// Case-insensitive: section titles are uppercased by the renderer, and a
	// case difference is not a missing line.
	hay := strings.ToLower(normalizeSpace(text))

	var missing []string
	check := func(label, want string) {
		want = strings.ToLower(normalizeSpace(want))
		if want == "" {
			return
		}
		if len([]rune(want)) > verifySampleChars {
			want = string([]rune(want)[:verifySampleChars])
		}
		if !strings.Contains(hay, want) {
			missing = append(missing, fmt.Sprintf("%s did not survive rendering: %q", label, want))
		}
	}

	check("the name", p.Name)
	check("the email", p.Email)
	check("the headline", t.Headline)
	check("the summary", t.Summary)
	for _, sec := range t.Sections {
		check("section "+sec.Title, sec.Title)
		for _, it := range sec.Items {
			check("item "+it.Title, it.Title)
			for i, b := range it.Bullets {
				check(fmt.Sprintf("bullet %d of %q", i+1, it.Title), b.Text)
			}
		}
	}
	for _, s := range t.SelectedSkills {
		check("skill "+s, s)
	}

	if pages := pdftext.PageCount(pdfBytes); pages > 1 {
		missing = append(missing, fmt.Sprintf("the resume runs to %d pages; the standard is one", pages))
	}
	return missing
}

// normalizeSpace collapses every run of whitespace to one space, because a
// line break in the PDF is not a difference in the text.
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
