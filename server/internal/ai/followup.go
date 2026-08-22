package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cvx/internal/model"
)

const followUpSystemPrompt = `You write the short follow-up a candidate sends when an application has gone quiet.

This is a nudge, not a second application. The recruiter already has the
resume and the original email; the only job here is to be brief, easy to
reply to, and impossible to resent.

Structure — return a "subject", 1 "paragraph", and a "closing":
- 2 to 4 sentences total. Shorter is better, and shorter is more likely to
  get answered.
- The first sentence says what this is about: that they applied for the role,
  and roughly when.
- The middle may add at most ONE thing: a concrete, verifiable fact from the
  profile that is relevant to the role and was not the centerpiece of the
  original email, or a plain statement of continued interest. Never repeat
  the original email's argument in full.
- The last sentence makes replying easy: ask whether there is an update, or
  whether anything else would be useful from them.
- The subject line refers to the original application ("Following up on my
  application for <role title>"), under 80 characters.
- The closing is the exact sign-off the angle below dictates. Do NOT include
  the candidate's name anywhere; the sender appends it.

Hard rules:
- Reference only facts present in the profile JSON. Never invent employers,
  metrics, dates, or accomplishments.
- Never imply the recruiter was rude, slow, or negligent, and never ask twice
  in the same message.
- No em dashes anywhere. No exclamation marks anywhere. Sentence case
  throughout.
- Do not write a greeting; one is composed for you.
- Never leave placeholder brackets like [Company] or [Role].`

// FollowUp writes the nudge for an application that has gone quiet.
// daysSince is how long ago it was sent, so the prose can say "a couple of
// weeks ago" rather than guessing.
func FollowUp(ctx context.Context, llm LLM, p model.Profile, posting model.Posting, daysSince int) (model.RecruiterEmail, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.RecruiterEmail{}, fmt.Errorf("follow up: marshal profile: %w", err)
	}

	flavor, variant := pickFlavor(), pickVariant()
	blocks := append(
		[]ContentBlock{{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)}},
		postingBlocks(posting)...,
	)
	blocks = append(blocks,
		ContentBlock{Text: fmt.Sprintf("The application was sent %d days ago and has had no reply.", daysSince)},
		ContentBlock{Text: fmt.Sprintf("Close with exactly %q, on its own line.", flavor.EmailClosing)},
	)

	var violations []string
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := llm.GenerateJSON(ctx, followUpSystemPrompt, blocks, recruiterEmailSchema)
		if err != nil {
			return model.RecruiterEmail{}, fmt.Errorf("follow up: %w", err)
		}

		var re model.RecruiterEmail
		if err := json.Unmarshal(raw, &re); err != nil {
			return model.RecruiterEmail{}, fmt.Errorf("follow up: unmarshal response: %w", err)
		}
		re.Greeting = model.Greeting(posting.GreetingInputFrom(false), variant)

		violations = checkFollowUp(re, p.Name)
		if len(violations) == 0 {
			return re, nil
		}
		blocks = append(blocks, ContentBlock{Text: "Previous draft:\n" + string(raw)}, rewriteBlock(violations))
	}
	return model.RecruiterEmail{}, fmt.Errorf("follow up failed quality checks: %s", strings.Join(violations, "; "))
}
