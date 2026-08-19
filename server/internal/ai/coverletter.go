package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cvx/internal/model"
)

const coverLetterSystemPrompt = `You write a cover letter for a candidate applying to a specific role, using only their profile JSON.

Before writing, pick the two or three requirements in the role that this
profile most concretely supports, and the specific profile facts — named
systems, technologies, numbers, outcomes — that evidence each one. Build the
letter out of those.

Structure — return a "greeting", 2 or 3 "paragraphs", and a "closing":
- Paragraph 1 names the role and the single strongest genuine reason this
  candidate fits it.
- Paragraph 2 gives the concrete evidence: systems built, technologies used,
  and measurable outcomes drawn from the profile, each tied to something the
  role actually asks for.
- The last paragraph closes briefly.

Specificity rules:
- Every sentence should say something that could only be written about this
  candidate and this role. Cut anything that would read the same for any
  other applicant.
- Prefer the concrete fact to the generic claim: name the system and the
  number instead of writing "proven track record", "passionate about",
  "extensive experience", or "strong communication skills".
- Where the role asks for something the profile does not cover, either point
  honestly at the closest adjacent experience or leave it out. Never paper
  over a gap with enthusiasm.

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

	// Bounded QC loop: exactly one generation plus at most one corrective
	// rewrite — never a retry-until-clean loop; a second bad draft is a hard
	// error the caller already degrades on.
	var violations []string
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := llm.GenerateJSON(ctx, coverLetterSystemPrompt, blocks, coverLetterSchema)
		if err != nil {
			return model.CoverLetter{}, fmt.Errorf("cover letter: %w", err)
		}

		var cl model.CoverLetter
		if err := json.Unmarshal(raw, &cl); err != nil {
			return model.CoverLetter{}, fmt.Errorf("cover letter: unmarshal response: %w", err)
		}

		violations = checkCoverLetter(cl)
		if len(violations) == 0 {
			return cl, nil
		}
		blocks = append(blocks, ContentBlock{Text: "Previous draft:\n" + string(raw)}, rewriteBlock(violations))
	}
	return model.CoverLetter{}, fmt.Errorf("cover letter failed quality checks: %s", strings.Join(violations, "; "))
}
