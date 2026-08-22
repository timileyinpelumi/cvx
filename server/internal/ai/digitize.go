package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/model"
)

const digitizePrompt = `You extract a candidate's resume/CV into structured JSON.

First decide whether the document is actually a resume or CV: a document
presenting a person's work history, skills, or qualifications. If it is not
(an invoice, an article, a slide deck, a form, arbitrary text), set isResume
to false, put one short sentence in notResumeReason, and use empty strings
and empty arrays for every other field. Otherwise set isResume to true and
notResumeReason to "".

Rules:
- Extract EVERYTHING present in the document. Invent nothing.
- If a field is not stated in the document, use an empty string "" (or an
  empty array for list fields) rather than guessing.
- Tag each bullet with the skills it demonstrates (technologies, tools,
  methods explicitly named in or clearly implied by that bullet).
- Preserve the candidate's own wording for bullets; do not rephrase.
- "kind" for each item should be one of: "experience", "education",
  "project", "volunteering", or another short lowercase label that best
  matches the entry.
- "certifications" are credentials with no bullets and no date range (a
  named certificate, licence, or award), each with whatever the document
  gives of issuer and year.
- "languages" are spoken or written human languages, with the stated level
  when the document gives one ("French (fluent)"). Not programming
  languages: those are skills.
- "interests" are hobbies, communities, and outside activities exactly as
  the document lists them. Do not invent any: an empty array is the correct
  answer for a resume that has no such section.`

// draftBullet/draftItem/draftProfile mirror model's shapes minus id fields —
// the LLM never assigns ids; model.AssignIDs does that deterministically
// after unmarshaling.
type draftBullet struct {
	Text   string   `json:"text"`
	Skills []string `json:"skills"`
}

type draftItem struct {
	Kind         string        `json:"kind"`
	Title        string        `json:"title"`
	Organization string        `json:"organization"`
	StartDate    string        `json:"startDate"`
	EndDate      string        `json:"endDate"`
	Bullets      []draftBullet `json:"bullets"`
}

type draftProfile struct {
	IsResume        bool   `json:"isResume"`
	NotResumeReason string `json:"notResumeReason"`

	Name     string       `json:"name"`
	Email    string       `json:"email"`
	Phone    string       `json:"phone"`
	Location string       `json:"location"`
	Summary  string       `json:"summary"`
	Links    []model.Link `json:"links"`
	Skills   []string     `json:"skills"`
	Items    []draftItem  `json:"items"`

	Certifications []model.Certification `json:"certifications"`
	Languages      []string              `json:"languages"`
	Interests      []string              `json:"interests"`
}

// Digitize turns a PDF resume into a model.Profile with ids assigned.
func Digitize(ctx context.Context, llm LLM, pdf []byte) (model.Profile, error) {
	return digitize(ctx, llm, digitizePrompt, []ContentBlock{{PDF: pdf}})
}

// digitizeTextPrompt is the same extraction with a looser front door. A
// person who has never written a CV pastes a LinkedIn about-section, an old
// bio, or three sentences about their last job: none of that is a resume,
// and all of it is career material. Rejecting it because it is not
// formatted like a CV would lock out exactly the people this path exists
// for.
const digitizeTextPrompt = `You extract what a person tells you about their working life into structured JSON.

The text may be a resume, but it may equally be a LinkedIn profile, a bio, a
few sentences someone typed about their last job, or a rough list. Any of
those is usable. Set isResume to true whenever the text says something about
what this person has done, studied, built, or can do.

Set isResume to false only when there is no career content at all: random
characters, lorem ipsum, an invoice, an article about something else. Put one
short sentence in notResumeReason and leave every other field empty.

Rules:
- Extract EVERYTHING the text supports. Invent nothing: no dates, employers,
  numbers, or technologies that are not there.
- If a field is not stated, use an empty string "" (or an empty array)
  rather than guessing. Missing dates are normal in this kind of text and
  are asked for later; a guessed date is a lie that reaches the page.
- Split the text into one item per job, project, course, or period. A single
  paragraph about one role is one item with one or two bullets, not five.
- Preserve the person's own wording for bullets; tidy the grammar, do not
  rewrite the substance.
- "kind" for each item should be one of: "experience", "education",
  "project", "volunteering", or another short lowercase label that best
  matches the entry.
- "certifications", "languages" and "interests" follow the same rules as a
  resume: only what the text states.`

// DigitizeText builds a profile from prose rather than a PDF, for people
// who have never had a CV to upload.
func DigitizeText(ctx context.Context, llm LLM, text string) (model.Profile, error) {
	return digitize(ctx, llm, digitizeTextPrompt, []ContentBlock{{Text: "What they said about themselves:\n" + text}})
}

func digitize(ctx context.Context, llm LLM, prompt string, blocks []ContentBlock) (model.Profile, error) {
	raw, err := llm.GenerateJSON(ctx, prompt, blocks, profileSchema)
	if err != nil {
		return model.Profile{}, fmt.Errorf("digitize: %w", err)
	}

	var d draftProfile
	if err := json.Unmarshal(raw, &d); err != nil {
		return model.Profile{}, fmt.Errorf("digitize: unmarshal response: %w", err)
	}
	if !d.IsResume {
		return model.Profile{}, fmt.Errorf("%w: %s", ErrNotResume, d.NotResumeReason)
	}

	p := model.Profile{
		Name:     d.Name,
		Email:    d.Email,
		Phone:    d.Phone,
		Location: d.Location,
		Summary:  d.Summary,
		Links:    d.Links,
		Skills:   d.Skills,
		Items:    make([]model.Item, len(d.Items)),

		Certifications: d.Certifications,
		Languages:      d.Languages,
		Interests:      d.Interests,
	}
	for i, di := range d.Items {
		item := model.Item{
			Kind:         di.Kind,
			Title:        di.Title,
			Organization: di.Organization,
			StartDate:    di.StartDate,
			EndDate:      di.EndDate,
			Bullets:      make([]model.Bullet, len(di.Bullets)),
		}
		for j, db := range di.Bullets {
			item.Bullets[j] = model.Bullet{Text: db.Text, Skills: db.Skills}
		}
		p.Items[i] = item
	}

	model.AssignIDs(&p)
	return p, nil
}
