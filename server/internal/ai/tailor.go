package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"cvx/internal/model"
)

const tailorSystemPrompt = `You tailor a candidate's profile into a one-page resume for a specific role.

Step 1 — analyze the role before selecting anything. From the target role
text, identify its core responsibilities, the technologies and practices it
requires, and its seniority signals. For each one, decide whether the profile
covers it directly, covers it only through adjacent or transferable work, or
does not cover it at all.

Step 2 — select evidence. Walk each requirement from step 1 against EVERY item
in the profile (recent roles, older roles, projects, open source, education)
and note which bullets touch it. Evidence for a requirement often sits in a
side project, an open-source repo, or an older role rather than the most
recent job — check there before concluding the profile has nothing. Then rank
items and bullets by how directly they evidence those requirements, not by
recency or prestige.

Step 3 — rewrite and order the selected content so the strongest evidence for
THIS role comes first, then write the headline, summary, gaps, and
whatChanged.

Selection rules:
- Relevance beats recency: an older role, a side project, or an education
  bullet that directly evidences a requirement outranks a recent bullet that
  does not.
- On a stretch role (the profile does not match the domain head-on), select
  the work that genuinely transfers — the same languages, systems, scale, or
  data and operational practices the role also needs — and let the gaps list
  carry what is missing. Never force-fit: do not stretch a bullet beyond what
  it says, do not relabel unrelated work, and do not pad with weak items to
  reach the limit. Three or four strong items beat five where the last is
  filler.
- Do not select two bullets that make the same point; cover different
  requirements instead.
- Leave out any item that evidences none of the role's requirements, even if
  that means a shorter resume. Never include an item just to reach the limit.
- Fullness: when the profile holds more genuinely relevant evidence than
  you have selected, prefer the fuller selection. For a role the profile
  matches head-on, target 4-5 items carrying 3-4 bullets each before
  stopping; a thin resume from a rich profile wastes real evidence. This
  never overrides the rules above: padding with weak items, stretching
  bullets, or force-fitting stays forbidden, and a stretch role may stay
  short with the gaps list carrying the difference.
- Every item you include must carry 2-4 bullets. If only one of an item's
  bullets is worth showing for this role, drop the whole item rather than
  output it with a single bullet.

Bullet rules:
- Preserve the source bullet's specifics: its metrics, its scale, and the
  technologies it names. Never invent, change, or round a number, and never
  drop the outcome to make room for adjectives.
- Lead with the action and its result; cut words that carry no information.

Vocabulary mirroring:
- Where the profile's content already IS what the role asks for, describe it
  in the role's own words (its term for the technology, the practice, or the
  outcome) rather than a generic synonym.
- Anti-stuffing: mirror a term only when the source bullet genuinely supports
  it. A term the profile does not back belongs in "gaps", never in a bullet,
  a skill, the headline, or the summary. Honestly missing a keyword scores
  better than claiming it.

Hard rules:
- You may only select, reorder, and rephrase content that already exists in
  the profile JSON you are given. Never invent experience, skills, titles,
  dates, or organizations.
- Every item and bullet you output MUST cite the exact "sourceId" /
  "sourceBulletId" of the profile item/bullet it comes from. Do not fabricate
  ids.
- Select 3 to 5 items total, and 2 to 4 bullets per item. Never output an
  item with a single bullet: give it a second one from the profile or leave
  the item out.
- Select 6 to 14 skills.
- "selectedSkills" must contain only strings copied verbatim from the
  profile's top-level "skills" array — character for character, no additions
  and no reworded variants. A technology that appears in a bullet's text, or
  in a bullet's own "skills" tags, but NOT in that top-level array, must not
  appear in "selectedSkills". Order the ones you keep by relevance to the
  role.
- Be honest about gaps: list requirements from the role that the profile does
  not clearly support, each with severity "missing" (not present at all) or
  "weak" (present but thin). Name each requirement in the role's own words.
- The headline and summary are written in implied first person, the resume
  standard: no pronouns at all. Never "I", "my", "he", "she", "his", or
  "her", and never the candidate's own name. "Backend engineer who builds
  payment systems in Go", not "Timileyin builds payment systems" and not "I
  build payment systems". Open on the role, not on the person.
- The headline and summary are held to the same evidence standard as the
  bullets, and are the easiest place to over-claim. State only what the
  profile's items actually show: do not claim a domain, specialism, seniority,
  or number of years the profile does not support, and do not assert the
  role's title when the experience is only adjacent to it. Year counts are
  the most common inflation: a count like "N+ years of X" is allowed only
  when the profile's dated items IN THAT DISCIPLINE add up to N years —
  total career years never stand in for discipline years. When the math is
  not clearly supported, omit the year count entirely. Make them the
  bridge instead — the candidate's real strength, described in the terms this
  role uses, and the concrete transferable evidence for its most central
  requirement. The summary is 55 to 75 words: three or four sentences
  covering what the candidate does, the evidence that matters most for THIS
  role, and the bridge to what the role asks for. Count the words before you
  answer; a two-sentence summary is under the standard and will be rejected.
- "targetRole" is a plain job title, not the pasted heading: the role and, if
  the posting names one, the company ("Website Manager at Savvy Spender").
  Under 60 characters, no duty lists, no parentheses, no seniority padding.
- "roleSummary" is one sentence saying what the role actually covers, in the
  posting's own words ("Runs the website end to end: blog posts, SEO, backend
  changes, and email outreach."). Under 25 words, and it never repeats the
  title verbatim.
- "whatChanged" must list 2-4 bullets summarizing what you changed and why.
- The result must fit on one page: be concise.`

// TailorOptions are the user's writing knobs. The zero value (or the named
// defaults) adds nothing to the request, so the default pipeline stays
// byte-identical to the eval-gated prompt.
type TailorOptions struct {
	Tone    string // "plain" (default) | "confident"
	Summary string // "standard" (default) | "short" | "none"
	Bullets string // "full" (default) | "lean"
}

// instructions renders the non-default knobs as one adjustments block, or ""
// when everything is at its default.
func (o TailorOptions) instructions() string {
	var lines []string
	if o.Tone == "confident" {
		lines = append(lines, "Voice: write the bullets, headline, and summary with direct, assertive verb choices. Every evidence rule still applies; confidence changes word choice, never claims.")
	}
	switch o.Summary {
	case "short":
		lines = append(lines, "Summary: keep the summary to at most 35 words.")
	case "none":
		lines = append(lines, `Summary: output an empty string "" for the summary; the candidate wants no summary section.`)
	}
	if o.Bullets == "lean" {
		lines = append(lines, "Bullets: prefer 2 or 3 bullets per item, keeping only the strongest evidence.")
	}
	if len(lines) == 0 {
		return ""
	}
	return "Adjustments requested by the candidate:\n- " + strings.Join(lines, "\n- ")
}

// Tailor selects, reorders, and rephrases profile content for roleInput,
// citing exact profile ids. model.ValidateTailored rejects fabricated ids
// before the result is returned.
func Tailor(ctx context.Context, llm LLM, p model.Profile, posting model.Posting) (model.Tailored, error) {
	return TailorWithOptions(ctx, llm, p, posting, TailorOptions{})
}

// TailorWithOptions is Tailor with the user's writing knobs applied.
func TailorWithOptions(ctx context.Context, llm LLM, p model.Profile, posting model.Posting, opts TailorOptions) (model.Tailored, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return model.Tailored{}, fmt.Errorf("tailor: marshal profile: %w", err)
	}

	blocks := append(
		[]ContentBlock{{Text: fmt.Sprintf("Profile JSON:\n%s", profileJSON)}},
		postingBlocks(posting)...,
	)
	if extra := opts.instructions(); extra != "" {
		blocks = append(blocks, ContentBlock{Text: extra})
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

	if issues := model.ShapeIssues(p, t); len(issues) > 0 {
		if repaired, err := repairShape(ctx, llm, p, blocks, raw, issues); err == nil {
			t = repaired
		} else {
			// A resume that misses a floor still beats no resume, so a failed
			// repair keeps the first result rather than failing the request.
			slog.Warn("tailor shape repair failed", "err", err, "issues", len(issues))
		}
	}

	return t, nil
}

// repairShape asks for one more pass over an output that came back under a
// content floor, handing back the model's own JSON plus the specific misses.
// The result goes through the same id guardrail as the first pass.
func repairShape(
	ctx context.Context,
	llm LLM,
	p model.Profile,
	blocks []ContentBlock,
	previous []byte,
	issues []string,
) (model.Tailored, error) {
	repair := append(append([]ContentBlock{}, blocks...),
		ContentBlock{Text: fmt.Sprintf("Your previous output:\n%s", previous)},
		ContentBlock{Text: fmt.Sprintf(
			"That output falls short of the resume standard:\n- %s\n\nReturn the whole resume again, fixing exactly these points and changing nothing else. Every id must still come from the profile.",
			strings.Join(issues, "\n- "))},
	)

	raw, err := llm.GenerateJSON(ctx, tailorSystemPrompt, repair, tailoredSchema)
	if err != nil {
		return model.Tailored{}, fmt.Errorf("tailor repair: %w", err)
	}
	var t model.Tailored
	if err := json.Unmarshal(raw, &t); err != nil {
		return model.Tailored{}, fmt.Errorf("tailor repair: unmarshal response: %w", err)
	}
	if err := model.ValidateTailored(p, t); err != nil {
		return model.Tailored{}, fmt.Errorf("tailor repair: %w", err)
	}
	return t, nil
}
