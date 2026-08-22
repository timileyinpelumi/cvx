package ai

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// A prose flavor is the one thing that varies between drafts. Everything an
// application email or cover letter must contain is fixed by the prompt and
// enforced by prose QC; without a flavor the same profile and the same
// posting produce the same paragraph every time, which reads like a template
// the moment anyone sends two of them.
//
// The angle only governs what comes AFTER the opening sentence: every draft
// still has to say what it is and what it is for before it says anything
// clever.
type proseFlavor struct {
	Name string
	// Angle steers the body: which piece of evidence leads.
	Angle string
	// Subject is the shape of the email subject line, which carries the role
	// title plus whatever this angle makes the sharpest differentiator.
	Subject string
	// Closings differ by medium: an email can sign off "Thanks,", a printed
	// letter cannot.
	EmailClosing  string
	LetterClosing string
}

var proseFlavors = []proseFlavor{
	{
		Name:          "project-first",
		Angle:         "After the opening sentence, lead with the single most relevant thing the candidate has built, named, and connect it to what the posting asks for.",
		Subject:       `"Application for <role title>, <the named system or project that fits best>"`,
		EmailClosing:  "Best regards,",
		LetterClosing: "Sincerely,",
	},
	{
		Name:          "requirement-first",
		Angle:         "After the opening sentence, take the posting's most central requirement and answer it directly with the profile's strongest evidence for it.",
		Subject:       `"Application for <role title>, <the central requirement the candidate covers>"`,
		EmailClosing:  "Thanks,",
		LetterClosing: "Best regards,",
	},
	{
		Name:          "outcome-first",
		Angle:         "After the opening sentence, lead with a concrete outcome from the profile (a system that shipped, a number that moved) and say what it has to do with this role.",
		Subject:       `"Application for <role title>, <the outcome in three or four words>"`,
		EmailClosing:  "Kind regards,",
		LetterClosing: "Sincerely,",
	},
	{
		Name:          "practice-first",
		Angle:         "After the opening sentence, describe how the candidate works in the discipline the role centers on, then back it with one named piece of evidence.",
		Subject:       `"Application for <role title>, <the discipline and the years or depth in it>"`,
		EmailClosing:  "Best,",
		LetterClosing: "Best regards,",
	},
	{
		Name:          "stack-first",
		Angle:         "After the opening sentence, lead with the technologies the posting names that the candidate genuinely works in, then the evidence that proves it.",
		Subject:       `"Application for <role title>, <two or three technologies from the posting>"`,
		EmailClosing:  "Best regards,",
		LetterClosing: "Sincerely,",
	},
}

// pickFlavor is a variable so tests can pin the draw; production draws
// uniformly at random per generation.
var pickFlavor = func() proseFlavor {
	return proseFlavors[rand.IntN(len(proseFlavors))]
}

// pickVariant is the greeting's variant index, drawn per generation so two
// postings of the same tone do not open with the same word.
var pickVariant = func() int { return rand.IntN(8) }

// emailInstructions renders the flavor as the per-draft angle appended to the
// fixed email prompt. The closing is dictated rather than suggested so prose
// QC's sign-off check has something exact to hold the draft to.
func (f proseFlavor) emailInstructions() ContentBlock {
	return ContentBlock{Text: fmt.Sprintf(
		"Angle for this draft (%s):\n- %s\n- Shape the subject as %s. Drop the trailing detail if it would push the subject past 80 characters.\n- Close with exactly %q, on its own line.\n- Vary the sentence shapes from a stock application email: this must not read like a template.",
		f.Name, f.Angle, f.Subject, f.EmailClosing)}
}

// letterInstructions is emailInstructions for the cover letter: same angle,
// no subject line, and a sign-off a printed letter can carry.
func (f proseFlavor) letterInstructions() ContentBlock {
	return ContentBlock{Text: fmt.Sprintf(
		"Angle for this draft (%s):\n- %s\n- Close with exactly %q, on its own line.\n- Vary the sentence shapes from a stock cover letter: this must not read like a template.",
		f.Name, f.Angle, f.LetterClosing)}
}

// closingAllowed reports whether closing is one of the sign-offs a flavor can
// dictate for this medium, which is what prose QC checks the draft against.
func closingAllowed(closing string, letter bool) bool {
	want := strings.TrimSpace(closing)
	for _, f := range proseFlavors {
		got := f.EmailClosing
		if letter {
			got = f.LetterClosing
		}
		if strings.EqualFold(want, got) {
			return true
		}
	}
	return false
}
