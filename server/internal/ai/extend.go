package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/model"
)

const extendProfileSystemPrompt = `You convert a short note from the candidate into additions to their existing profile.

Hard rules:
- Use ONLY facts stated in the note. Never invent employers, dates, or metrics
  that are not in the note.
- The candidate's current profile is given to you as JSON. If the note
  clearly refers to an existing item in that profile (the same role,
  employer, or project), add new bullets to that item by referencing its
  exact id from the profile JSON in bulletAdditions. Only create a new item
  in newItems when the note describes something not already represented in
  the profile.
- List a skill in newSkills only when the note explicitly evidences it (the
  note names the technology, tool, or method). Never infer a skill that is
  merely plausible.
- Leave newSkills, newItems, or bulletAdditions as empty arrays (not
  omitted) when the note has nothing to add for that category.`

// ExtendProfile turns a candidate's typed note into ProfileAdditions:
// derived new skills, whole new items, and/or bullets to append to items
// that already exist in p. Like CoverLetter, there is no id-based guardrail
// here (the LLM proposes content, never ids) — model.MergeAdditions is what
// enforces that any referenced item id actually exists before anything is
// written to the profile.
func ExtendProfile(ctx context.Context, llm LLM, p model.Profile, note string) (model.ProfileAdditions, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.ProfileAdditions{}, fmt.Errorf("extend profile: marshal profile: %w", err)
	}

	blocks := []ContentBlock{
		{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)},
		{Text: fmt.Sprintf("Note:\n%s", note)},
	}

	raw, err := llm.GenerateJSON(ctx, extendProfileSystemPrompt, blocks, profileAdditionsSchema)
	if err != nil {
		return model.ProfileAdditions{}, fmt.Errorf("extend profile: %w", err)
	}

	var a model.ProfileAdditions
	if err := json.Unmarshal(raw, &a); err != nil {
		return model.ProfileAdditions{}, fmt.Errorf("extend profile: unmarshal response: %w", err)
	}

	return a, nil
}
