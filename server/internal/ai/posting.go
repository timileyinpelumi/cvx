package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cvx/internal/model"
)

const parsePostingPrompt = `You read a piece of text and, if it is usable input for a resume tailoring tool, structure it.

Usable input is any of:
- a job posting, complete or partial (a real one, in any language)
- the text of a job page fetched from a link, even with site chrome around it
- a role title or short role description, e.g. "Backend Engineer" or
  "Senior iOS developer, fintech"

Unusable input is everything else: random or mashed characters, test strings,
lorem ipsum, and content that is not about a job or role (an article, a
receipt, source code, a chat log). Judge only whether the text is usable; do
not judge whether the job is good. Set "reason" to one short sentence, and
leave every other field empty when the input is unusable.

When it IS usable, extract only what the text actually states. Every field is
allowed to be empty, and an empty field is always better than a guess: these
values are used verbatim in a resume, an email subject, and a greeting, so an
invented company name or contact is worse than none.

- "title": the job title, in title case. When the posting heads itself with a
  bare field of study rather than a job title ("computer engineering"), make
  it role-shaped using only what the posting supports ("Computer Engineering
  Intern"). Drop any parenthetical list of duties.
- "company": the hiring company's name, exactly as written, empty if the
  posting does not name one. Never infer it from an email domain or a URL.
- "contactName": the person the posting names as the contact for applying,
  exactly as written. Empty when it names nobody, names only a team ("the
  hiring team", "HR", "recruitment"), or gives only an email address.
- "location": the work location or remote policy as stated.
- "seniority": one of intern, junior, mid, senior, lead, unknown.
- "tone": how the posting itself is written. "formal" for public-sector,
  agency, or third-person corporate ads; "casual" for ads written in the
  second person with informal language; "neutral" for everything else.
- "mustHaves": the requirements the posting states as required, in its own
  words, at most 10, most central first.
- "niceToHaves": the requirements it states as preferred or bonus, at most 8.
- "keywords": the concrete named things the posting asks for — technologies,
  languages, frameworks, tools, platforms, certifications — copied exactly as
  the posting writes them, at most 25. Do NOT include soft competences
  ("communication", "teamwork", "attention to detail"): they cannot be
  matched against a resume by name.`

var postingSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"usable":      map[string]any{"type": "boolean"},
		"reason":      map[string]any{"type": "string"},
		"title":       map[string]any{"type": "string"},
		"company":     map[string]any{"type": "string"},
		"contactName": map[string]any{"type": "string"},
		"location":    map[string]any{"type": "string"},
		"seniority": map[string]any{
			"type": "string",
			"enum": []string{"intern", "junior", "mid", "senior", "lead", "unknown"},
		},
		"tone": map[string]any{
			"type": "string",
			"enum": []string{"formal", "neutral", "casual"},
		},
		"mustHaves":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"niceToHaves": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"keywords":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []string{
		"usable", "reason", "title", "company", "contactName", "location",
		"seniority", "tone", "mustHaves", "niceToHaves", "keywords",
	},
	"additionalProperties": false,
}

// ParsePosting reads the job input once and returns it structured. It
// subsumes the old ClassifyJobInput check: unusable input comes back as
// ErrNotJobInput with the model's reason attached, so the caller still gets
// one decision, and everything downstream reads the same parsed object
// instead of re-extracting from raw text.
func ParsePosting(ctx context.Context, llm LLM, text string) (model.Posting, error) {
	input := text
	if len(input) > classifyInputLimit {
		input = input[:classifyInputLimit]
	}

	raw, err := llm.GenerateJSON(ctx, parsePostingPrompt,
		[]ContentBlock{{Text: "Input:\n" + input}}, postingSchema)
	if err != nil {
		return model.Posting{}, fmt.Errorf("parse posting: %w", err)
	}

	var p model.Posting
	if err := json.Unmarshal(raw, &p); err != nil {
		return model.Posting{}, fmt.Errorf("parse posting: unmarshal response: %w", err)
	}
	if !p.Usable {
		return model.Posting{}, fmt.Errorf("%w: %s", ErrNotJobInput, p.Reason)
	}

	p.Raw = text
	normalizePosting(&p)
	return p, nil
}

// normalizePosting trims the model's answers to what the rest of the
// pipeline promises: bounded lists, a known seniority, and no keyword
// repeated in two casings.
func normalizePosting(p *model.Posting) {
	p.Title = strings.TrimSpace(p.Title)
	p.Company = strings.TrimSpace(p.Company)
	p.ContactName = strings.TrimSpace(p.ContactName)
	p.Location = strings.TrimSpace(p.Location)

	switch p.Seniority {
	case model.SeniorityIntern, model.SeniorityJunior, model.SeniorityMid,
		model.SenioritySenior, model.SeniorityLead:
	default:
		p.Seniority = model.SeniorityUnknown
	}
	switch p.Tone {
	case model.ToneFormal, model.ToneCasual:
	default:
		p.Tone = model.ToneNeutral
	}

	p.MustHaves = clampList(p.MustHaves, 10)
	p.NiceToHaves = clampList(p.NiceToHaves, 8)
	p.Keywords = clampList(dedupeFold(p.Keywords), 25)
}

func clampList(in []string, max int) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
		if len(out) == max {
			break
		}
	}
	return out
}

func dedupeFold(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		key := strings.ToLower(strings.TrimSpace(s))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

// postingBlocks renders the posting for a downstream prompt: the structured
// read first, so the model works from the same facts every other artifact
// does, then the raw text for everything the schema did not capture.
func postingBlocks(p model.Posting) []ContentBlock {
	var b strings.Builder
	b.WriteString("The posting, already read and structured. Treat these as the facts; do not re-derive them:\n")
	writeField(&b, "Title", p.Title)
	writeField(&b, "Company", p.Company)
	writeField(&b, "Location", p.Location)
	writeField(&b, "Seniority", p.Seniority)
	writeList(&b, "Required", p.MustHaves)
	writeList(&b, "Preferred", p.NiceToHaves)
	writeList(&b, "Named technologies", p.Keywords)

	return []ContentBlock{
		{Text: b.String()},
		{Text: "The posting in full:\n" + p.Raw},
	}
}

func writeField(b *strings.Builder, label, value string) {
	if value == "" {
		value = "not stated"
	}
	fmt.Fprintf(b, "- %s: %s\n", label, value)
}

func writeList(b *strings.Builder, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(b, "- %s: not stated\n", label)
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", label, strings.Join(values, "; "))
}
