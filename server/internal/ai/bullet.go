package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cvx/internal/model"
)

const rewriteBulletSystemPrompt = `You rewrite one bullet on a tailored resume.

You are given the profile bullet it comes from (the only source of truth for
what happened), the item that bullet belongs to, the role being applied for,
and the current wording on the resume. Return one replacement bullet.

Rules:
- Say only what the profile bullet says. You may reorder it, cut it, sharpen
  the verb, or use the role's own vocabulary for something the profile bullet
  already describes. You may not add a technology, a metric, a scale, a team
  size, an outcome, or a date that is not in it.
- Lead with what was done, not with the process. Prefer the concrete noun to
  the abstract one.
- One sentence, under 30 words, no trailing period unless the bullet is a
  full sentence that needs one.
- No em dashes, no exclamation marks, no placeholder brackets, and no
  "responsible for" or "helped with" phrasing.
- If the candidate asked for something specific, do that, unless it would
  require claiming something the profile bullet does not support. In that
  case return the closest honest version.`

var rewriteBulletSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"text": map[string]any{"type": "string"},
	},
	"required":             []string{"text"},
	"additionalProperties": false,
}

// maxBulletWords bounds the rewrite: a bullet that runs longer than this is
// a paragraph, and it will not fit the one-page budget.
const maxBulletWords = 34

// RewriteBullet regenerates a single bullet in place, so a weak line can be
// fixed without re-rolling the whole resume. source is the profile bullet the
// resume line cites; the rewrite is held to it.
func RewriteBullet(
	ctx context.Context,
	llm LLM,
	source model.Bullet,
	itemTitle, organization string,
	posting model.Posting,
	current, instruction string,
) (string, error) {
	blocks := []ContentBlock{
		{Text: fmt.Sprintf("The profile bullet, which is the only source of truth:\n%s", source.Text)},
		{Text: fmt.Sprintf("It belongs to: %s at %s", itemTitle, organization)},
		{Text: fmt.Sprintf("The role being applied for: %s", posting.RoleLabel())},
		{Text: fmt.Sprintf("The current wording on the resume:\n%s", current)},
	}
	if strings.TrimSpace(instruction) != "" {
		blocks = append(blocks, ContentBlock{
			Text: "What the candidate asked for:\n" + strings.TrimSpace(instruction)})
	}

	raw, err := llm.GenerateJSON(ctx, rewriteBulletSystemPrompt, blocks, rewriteBulletSchema)
	if err != nil {
		return "", fmt.Errorf("rewrite bullet: %w", err)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("rewrite bullet: unmarshal response: %w", err)
	}

	text := strings.TrimSpace(out.Text)
	if text == "" {
		return "", fmt.Errorf("rewrite bullet: model returned an empty bullet")
	}
	if v := checkProseText("bullet", text); len(v) > 0 {
		return "", fmt.Errorf("rewrite bullet failed quality checks: %s", strings.Join(v, "; "))
	}
	if n := len(strings.Fields(text)); n > maxBulletWords {
		return "", fmt.Errorf("rewrite bullet: %d words, the standard is at most %d", n, maxBulletWords)
	}
	return text, nil
}
