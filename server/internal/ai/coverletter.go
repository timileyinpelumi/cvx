package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/model"
)

const coverLetterSystemPrompt = `You write a cover letter for a candidate applying to a specific role, using only their profile JSON.

Hard rules:
- Reference only facts present in the profile JSON. Never invent employers,
  metrics, dates, or accomplishments.
- Be professional and specific; use at most 3 paragraphs.
- No em dashes anywhere in the letter.
- No exclamation marks anywhere in the letter.
- Write in sentence case throughout (not Title Case, not all caps).
- Never leave placeholder brackets like [Company] or [Role] in the output —
  write real prose, or omit the detail if it is not known.
- Address the letter to the role's company by name only if the role input
  names a specific company. Otherwise use the generic greeting
  "Dear hiring team,".`

// CoverLetter writes a short cover letter for roleInput, citing only facts
// present in p. Unlike Tailor, there is no id-based guardrail to validate
// (this is prose, not a citation-structured document) — the system prompt is
// the only defense against fabrication.
func CoverLetter(ctx context.Context, llm LLM, p model.Profile, roleInput string) (model.CoverLetter, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.CoverLetter{}, fmt.Errorf("cover letter: marshal profile: %w", err)
	}

	blocks := []ContentBlock{
		{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)},
		{Text: fmt.Sprintf("Target role:\n%s", roleInput)},
	}

	raw, err := llm.GenerateJSON(ctx, coverLetterSystemPrompt, blocks, coverLetterSchema)
	if err != nil {
		return model.CoverLetter{}, fmt.Errorf("cover letter: %w", err)
	}

	var cl model.CoverLetter
	if err := json.Unmarshal(raw, &cl); err != nil {
		return model.CoverLetter{}, fmt.Errorf("cover letter: unmarshal response: %w", err)
	}

	return cl, nil
}
