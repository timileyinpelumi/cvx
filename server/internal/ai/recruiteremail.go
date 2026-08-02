package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/model"
)

const recruiterEmailSystemPrompt = `You write the short email a candidate sends a recruiter when applying to a specific role, using only their profile JSON.

This is an email, not a cover letter: no date, no addresses, no letterhead.
The first paragraph opens plainly (a conversational first line naming the
role and why this candidate genuinely fits it); there is no separate
greeting field.

Structure — return a "subject", 1 or 2 "paragraphs", and a "closing":
- 3 to 5 sentences total across the paragraphs. Shorter is better.
- The subject is "Application for <role title>", using the posting's own
  title, kept under 80 characters.
- The paragraphs carry one or two concrete, verifiable facts from the
  profile (named systems, technologies, numbers) tied to what the role
  asks for. Cut anything that would read the same for any other applicant.
- The closing is a short sign-off such as "Best regards," — do NOT include
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
func RecruiterEmail(ctx context.Context, llm LLM, p model.Profile, roleInput string) (model.RecruiterEmail, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.RecruiterEmail{}, fmt.Errorf("recruiter email: marshal profile: %w", err)
	}

	blocks := []ContentBlock{
		{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)},
		{Text: fmt.Sprintf("Target role:\n%s", roleInput)},
	}

	raw, err := llm.GenerateJSON(ctx, recruiterEmailSystemPrompt, blocks, recruiterEmailSchema)
	if err != nil {
		return model.RecruiterEmail{}, fmt.Errorf("recruiter email: %w", err)
	}

	var re model.RecruiterEmail
	if err := json.Unmarshal(raw, &re); err != nil {
		return model.RecruiterEmail{}, fmt.Errorf("recruiter email: unmarshal response: %w", err)
	}

	return re, nil
}
