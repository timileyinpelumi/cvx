package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cvx/internal/model"
)

const recruiterEmailSystemPrompt = `You write the short email a candidate sends a recruiter when applying to a specific role, using only their profile JSON.

This is an email, not a cover letter: no date, no addresses, no letterhead.

Structure — return a "subject", 1 or 2 "paragraphs", and a "closing". The
greeting is composed for you from the posting; do not write one:
- 3 to 5 sentences total across the paragraphs. Shorter is better.
- The subject follows the shape the angle below dictates and stays under 80
  characters. It always contains the posting's title exactly as given to you
  in the structured read above.
- Do not write a greeting into the paragraphs. The email already opens with
  one, so paragraph 1 starts with the first real sentence.
- The FIRST sentence of paragraph 1 states what this is: that the candidate
  is applying, the role by its title, and the company by name when the
  posting gives one. Only then does the angle below take over. An email that
  opens on a project without saying what it is about reads as a cold pitch.
- The LAST sentence of the email points at what is attached (the resume, and
  the cover letter when one is attached) in plain words.
- The paragraphs carry one or two concrete, verifiable facts from the
  profile (named systems, technologies, numbers) tied to what the role
  asks for. Cut anything that would read the same for any other applicant.
- The closing is the exact sign-off the angle below dictates. Do NOT include
  the candidate's name anywhere; the sender appends it.

Hard rules:
- Reference only facts present in the profile JSON. Never invent employers,
  metrics, dates, or accomplishments.
- Use the role's own vocabulary only where the profile genuinely supports
  it.
- No em dashes anywhere. No exclamation marks anywhere. Sentence case
  throughout.
- Never leave placeholder brackets like [Company] or [Role] — write real
  prose, or omit the detail if it is not known.`

// RecruiterEmail writes the forwardable application email for roleInput,
// citing only facts present in p. Like CoverLetter, this is prose — the
// system prompt is the only defense against fabrication.
func RecruiterEmail(ctx context.Context, llm LLM, p model.Profile, posting model.Posting) (model.RecruiterEmail, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.RecruiterEmail{}, fmt.Errorf("recruiter email: marshal profile: %w", err)
	}

	flavor, variant := pickFlavor(), pickVariant()
	blocks := append(
		[]ContentBlock{{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)}},
		postingBlocks(posting)...,
	)
	blocks = append(blocks, flavor.emailInstructions())

	// Bounded QC loop: exactly one generation plus at most one corrective
	// rewrite — never a retry-until-clean loop; a second bad draft is a hard
	// error (this path deliberately has no fallback).
	var violations []string
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := llm.GenerateJSON(ctx, recruiterEmailSystemPrompt, blocks, recruiterEmailSchema)
		if err != nil {
			return model.RecruiterEmail{}, fmt.Errorf("recruiter email: %w", err)
		}

		var re model.RecruiterEmail
		if err := json.Unmarshal(raw, &re); err != nil {
			return model.RecruiterEmail{}, fmt.Errorf("recruiter email: unmarshal response: %w", err)
		}

		re.Greeting = model.Greeting(posting.GreetingInputFrom(false), variant)

		violations = checkRecruiterEmail(re, p.Name)
		if len(violations) == 0 {
			return re, nil
		}
		blocks = append(blocks, ContentBlock{Text: "Previous draft:\n" + string(raw)}, rewriteBlock(violations))
	}
	return model.RecruiterEmail{}, fmt.Errorf("recruiter email failed quality checks: %s", strings.Join(violations, "; "))
}
