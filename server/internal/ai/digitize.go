package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/model"
)

const digitizePrompt = `You extract a candidate's resume/CV into structured JSON.

Rules:
- Extract EVERYTHING present in the document. Invent nothing.
- If a field is not stated in the document, use an empty string "" (or an
  empty array for list fields) rather than guessing.
- Tag each bullet with the skills it demonstrates (technologies, tools,
  methods explicitly named in or clearly implied by that bullet).
- Preserve the candidate's own wording for bullets; do not rephrase.
- "kind" for each item should be one of: "experience", "education",
  "project", or another short lowercase label that best matches the entry.`

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
	Name     string       `json:"name"`
	Email    string       `json:"email"`
	Phone    string       `json:"phone"`
	Location string       `json:"location"`
	Summary  string       `json:"summary"`
	Links    []model.Link `json:"links"`
	Skills   []string     `json:"skills"`
	Items    []draftItem  `json:"items"`
}

// Digitize turns a PDF resume into a model.Profile with ids assigned.
func Digitize(ctx context.Context, llm LLM, pdf []byte) (model.Profile, error) {
	blocks := []ContentBlock{{PDF: pdf}}
	raw, err := llm.GenerateJSON(ctx, digitizePrompt, blocks, profileSchema)
	if err != nil {
		return model.Profile{}, fmt.Errorf("digitize: %w", err)
	}

	var d draftProfile
	if err := json.Unmarshal(raw, &d); err != nil {
		return model.Profile{}, fmt.Errorf("digitize: unmarshal response: %w", err)
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
