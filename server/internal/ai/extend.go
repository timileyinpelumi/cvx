package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/model"
)

const extendProfileSystemPrompt = `You convert a short note from the candidate into additions to their existing profile.

First decide whether the note tells you anything about the candidate's work,
skills, education, or experience. If it does not (mashed characters, a test
string, a greeting, content unrelated to the candidate's background), set
useful to false, put one short sentence in notUsefulReason, and leave every
additions array empty. Otherwise set useful to true and notUsefulReason to "".

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
- A note can also carry the material that has no home among items: a
  credential goes in newCertifications, a spoken language in newLanguages
  (never a programming language, that is a skill), and a hobby, community,
  or outside activity in newInterests. Copy the candidate's own words.
- Leave newSkills, newItems, or bulletAdditions as empty arrays (not
  omitted) when the note has nothing to add for that category.
- A separate "Gap context" block may be present: it names a requirement
  from a recent generation that the user is answering. Use it only to
  choose wording and placement (which existing item the addition fits,
  which of the note's facts matter most, the requirement's own terms for
  what the note states). It is NOT content: never add a skill, bullet, or
  item that the note itself does not state, even when the context names
  it. A note that does not support the requirement produces additions for
  what it does support, or nothing at all.`

// ExtendProfile turns a candidate's typed note into ProfileAdditions:
// derived new skills, whole new items, and/or bullets to append to items
// that already exist in p. Like CoverLetter, there is no id-based guardrail
// here (the LLM proposes content, never ids) — model.MergeAdditions is what
// enforces that any referenced item id actually exists before anything is
// written to the profile.
func ExtendProfile(ctx context.Context, llm LLM, p model.Profile, note string, gapContext string) (model.ProfileAdditions, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.ProfileAdditions{}, fmt.Errorf("extend profile: marshal profile: %w", err)
	}

	blocks := []ContentBlock{
		{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)},
		{Text: fmt.Sprintf("Note:\n%s", note)},
	}
	if gapContext != "" {
		blocks = append(blocks, ContentBlock{Text: fmt.Sprintf("Gap context (the user is answering this gap):\n%s", gapContext)})
	}

	raw, err := llm.GenerateJSON(ctx, extendProfileSystemPrompt, blocks, profileAdditionsSchema)
	if err != nil {
		return model.ProfileAdditions{}, fmt.Errorf("extend profile: %w", err)
	}

	var out struct {
		Useful          bool   `json:"useful"`
		NotUsefulReason string `json:"notUsefulReason"`
		model.ProfileAdditions
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return model.ProfileAdditions{}, fmt.Errorf("extend profile: unmarshal response: %w", err)
	}
	if !out.Useful {
		return model.ProfileAdditions{}, fmt.Errorf("%w: %s", ErrNoteNotUseful, out.NotUsefulReason)
	}

	return out.ProfileAdditions, nil
}
