package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/model"
)

const tailorSystemPrompt = `You tailor a candidate's profile into a one-page resume for a specific role.

Hard rules:
- You may only select, reorder, and rephrase content that already exists in
  the profile JSON you are given. Never invent experience, skills, titles,
  dates, or organizations.
- Every item and bullet you output MUST cite the exact "sourceId" /
  "sourceBulletId" of the profile item/bullet it comes from. Do not fabricate
  ids.
- Select at most 5 items total, and 2-4 bullets per item — the most relevant
  to the target role.
- Be honest about gaps: list requirements from the role that the profile does
  not clearly support, each with severity "missing" (not present at all) or
  "weak" (present but thin).
- "whatChanged" must list 2-4 bullets summarizing what you changed and why.
- Mirror the target role's vocabulary only where the profile's own content
  already supports that framing — never relabel unrelated experience.
- The result must fit on one page: be concise.`

// Tailor selects, reorders, and rephrases profile content for roleInput,
// citing exact profile ids. model.ValidateTailored rejects fabricated ids
// before the result is returned.
func Tailor(ctx context.Context, llm LLM, p model.Profile, roleInput string) (model.Tailored, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.Tailored{}, fmt.Errorf("tailor: marshal profile: %w", err)
	}

	blocks := []ContentBlock{
		{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)},
		{Text: fmt.Sprintf("Target role:\n%s", roleInput)},
	}

	raw, err := llm.GenerateJSON(ctx, tailorSystemPrompt, blocks, tailoredSchema)
	if err != nil {
		return model.Tailored{}, fmt.Errorf("tailor: %w", err)
	}

	var t model.Tailored
	if err := json.Unmarshal(raw, &t); err != nil {
		return model.Tailored{}, fmt.Errorf("tailor: unmarshal response: %w", err)
	}

	if err := model.ValidateTailored(p, t); err != nil {
		return model.Tailored{}, err
	}

	return t, nil
}
