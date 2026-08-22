package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// The NotUseful errors mean the input was readable but is not the kind of
// content the endpoint needs. Handlers map them to 422 with fixed copy; the
// LLM's own reason is logged, never shown.
var (
	ErrNotJobInput   = errors.New("input is not a job ad or role")
	ErrNotResume     = errors.New("document is not a resume")
	ErrNoteNotUseful = errors.New("note carries no profile information")
)

const classifySkillsPrompt = `You judge whether each entry in a list is a plausible professional skill for a resume: a technology, tool, language, framework, method, or competence (e.g. "Go", "Postgres", "CI/CD", "Team leadership", "C++").

Not a skill: mashed or random characters, test strings, sentences, full job
titles, company names, and content with no professional meaning.

Judge each entry independently. Return one verdict per entry, in the same
order, with the entry echoed back verbatim in "skill".`

var skillsVerdictSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"verdicts": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skill": map[string]any{"type": "string"},
					"valid": map[string]any{"type": "boolean"},
				},
				"required":             []string{"skill", "valid"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"verdicts"},
	"additionalProperties": false,
}

// ClassifySkills judges the given entries in one batched call and returns the
// ones that are not plausible skills. An empty input returns nil without a
// call.
func ClassifySkills(ctx context.Context, llm LLM, skills []string) ([]string, error) {
	if len(skills) == 0 {
		return nil, nil
	}

	list, err := json.Marshal(skills)
	if err != nil {
		return nil, fmt.Errorf("classify skills: marshal: %w", err)
	}
	raw, err := llm.GenerateJSON(ctx, classifySkillsPrompt,
		[]ContentBlock{{Text: "Entries:\n" + string(list)}}, skillsVerdictSchema)
	if err != nil {
		return nil, fmt.Errorf("classify skills: %w", err)
	}

	var v struct {
		Verdicts []struct {
			Skill string `json:"skill"`
			Valid bool   `json:"valid"`
		} `json:"verdicts"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("classify skills: unmarshal response: %w", err)
	}

	var invalid []string
	for _, verdict := range v.Verdicts {
		if !verdict.Valid {
			invalid = append(invalid, verdict.Skill)
		}
	}
	return invalid, nil
}

// classifyInputLimit keeps the verdict call cheap: real job ads state what
// they are well inside this prefix.
const classifyInputLimit = 8000
