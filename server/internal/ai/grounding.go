package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cvx/internal/model"
)

const groundingSystemPrompt = `You audit generated application prose against a candidate's profile.

The profile JSON is the ONLY source of truth about what this candidate has
done. You will be given one or more pieces of prose written for a job
application. Find every claim in that prose which the profile does not
support.

A claim is unsupported when the profile does not contain it: an employer,
title, date, technology, metric, scale, team size, or outcome that is not
there, or that is there but weaker than the prose says ("led a team" when the
profile says "worked with a team", "cut latency by half" when the profile
gives no number).

A claim is supported when the profile contains it in substance, even if the
prose rewords it. Rephrasing, reordering, and summarizing are all allowed.
Statements of interest or intent ("I would bring the same care to your
systems") assert nothing about the past and are always supported.

Return one entry per unsupported claim, quoting the exact sentence from the
prose, naming which piece it came from, and stating in one line what the
profile actually says instead. Return an empty list when everything checks
out. Do not report anything you are not confident about: a false alarm on
honest prose costs more than a missed rewording.`

var groundingSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"unsupported": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"artifact":    map[string]any{"type": "string"},
					"claim":       map[string]any{"type": "string"},
					"profileSays": map[string]any{"type": "string"},
				},
				"required":             []string{"artifact", "claim", "profileSays"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"unsupported"},
	"additionalProperties": false,
}

// Artifact labels used in the audit's findings, so a warning can say which
// document to look at.
const (
	ArtifactCoverLetter = "cover letter"
	ArtifactEmail       = "application email"
)

// AuditGrounding checks generated prose against the profile and returns the
// claims it cannot find support for.
//
// The resume has a hard citation guardrail: every item and bullet must carry
// a profile id, checked in code. The cover letter and the application email
// are free prose with no such structure, so the system prompt was the only
// thing standing between a user and an invented metric in a document they
// send to a recruiter. This is the check for that: advisory, surfaced as a
// warning rather than a hard failure, because a false alarm must never block
// a send.
func AuditGrounding(ctx context.Context, llm LLM, p model.Profile, artifacts map[string]string) ([]model.Ungrounded, error) {
	var texts []string
	for label, body := range artifacts {
		if strings.TrimSpace(body) == "" {
			continue
		}
		texts = append(texts, fmt.Sprintf("--- %s ---\n%s", label, body))
	}
	if len(texts) == 0 {
		return nil, nil
	}
	// Stable order: map iteration is random, and a prompt that reshuffles
	// between runs cannot hit the response cache.
	sort.Strings(texts)

	profileJSON, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("grounding audit: marshal profile: %w", err)
	}

	blocks := []ContentBlock{
		{Text: fmt.Sprintf("Profile JSON (ground truth):\n%s", profileJSON)},
		{Text: "Prose to audit:\n" + strings.Join(texts, "\n\n")},
	}

	raw, err := llm.GenerateJSON(ctx, groundingSystemPrompt, blocks, groundingSchema)
	if err != nil {
		return nil, fmt.Errorf("grounding audit: %w", err)
	}
	var out struct {
		Unsupported []model.Ungrounded `json:"unsupported"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("grounding audit: unmarshal response: %w", err)
	}
	return out.Unsupported, nil
}
