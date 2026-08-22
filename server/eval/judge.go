package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"cvx/internal/ai"
	"cvx/internal/model"
)

const judgeResumeSystemPrompt = `You are a strict, skeptical resume-quality judge. You will be given a
candidate's full profile JSON (the only source of truth about what they have
actually done), a job description, and a tailored resume JSON generated from
that profile for that job description.

Score the tailored resume on each of the following dimensions, 0-10 (0 =
fails badly, 10 = excellent), with one concise one-line rationale per
dimension:

- selection: did it pick the items and bullets most relevant to this
  specific role, and leave out weaker or irrelevant ones? Score selection
  RELATIVE TO WHAT THE PROFILE OFFERS: when the profile has little direct
  experience for the role, a high score means the most transferable items
  were chosen and the mismatch is disclosed in the gaps; do not deduct for
  the absence of experience the candidate simply does not have. Deduct for
  force-fitting irrelevant items or padding when fewer, stronger items were
  available.
- vocabulary: does it mirror the job description's own language where the
  profile genuinely supports that framing, without stuffing in keywords the
  profile does not back up? Judge only against the vocabulary the profile
  CAN honestly support: a missing job-description keyword must not lower
  this score when the profile has no basis for it and the gap is disclosed.
  Deduct for supported terms left unmirrored and for unsupported terms
  smuggled in.
- bulletStrength: are the bullets specific and outcome-oriented, not vague
  restatements of duties?
- honesty: cross-check every claim in the tailored resume against the
  profile JSON, line by line. Any claim, number, or skill that is not
  traceable to the profile must lower this score sharply, even if it sounds
  plausible. Honest transferable positioning (for example presenting a
  backend engineer as a strong transitional candidate for an adjacent role,
  with the stretch disclosed in the gaps) is NOT a dishonesty and must not
  be penalized here; only untraceable claims are.
- gapQuality: are the listed gaps real (genuinely required by the job
  description and not clearly covered by the profile) and actionable, not
  vague or invented?
- headlineSummary: do the headline and summary accurately and compellingly
  represent this candidate for this specific role? For stretch roles, the
  best possible headline and summary position the candidate honestly as a
  transition (bridging from what they have to what the role needs) — score
  that framing as high as a direct match would be for a matching profile;
  do not demand the summary claim experience the profile lacks, and do not
  reward it if it does.

Return only the structured scores; no commentary outside the schema.`

const judgeCoverLetterSystemPrompt = `You are a strict, skeptical cover-letter judge. You will be given a
candidate's full profile JSON (the only source of truth about what they have
actually done), a job description, and a cover letter generated from that
profile for that job description.

Score the cover letter on each of the following dimensions, 0-10 (0 = fails
badly, 10 = excellent), with one concise one-line rationale per dimension:

- specificity: does it reference concrete, relevant facts from the profile
  and the job description, rather than generic filler that could apply to
  any candidate or any role?
- voice: is it professional, natural prose (not stilted or overly
  formulaic), free of placeholder brackets, em dashes, exclamation marks,
  and Title Case / all-caps overuse?
- factuality: cross-check every claim against the profile JSON. Any
  employer, metric, date, or accomplishment not traceable to the profile
  must lower this score sharply.

Return only the structured scores; no commentary outside the schema.`

const judgeRecruiterEmailSystemPrompt = `You are a strict, skeptical judge of application emails. You will be given a
candidate's full profile JSON (the only source of truth about what they have
actually done), a job description, and the email generated from that profile
for that job.

This is the email a candidate sends a recruiter with their resume attached,
not a cover letter. Score it on each dimension, 0-10 (0 = fails badly, 10 =
excellent), with one concise one-line rationale per dimension:

- specificity: does it carry concrete, verifiable facts from the profile
  (named systems, technologies, numbers) tied to what the role asks for,
  rather than filler that would read the same for any applicant?
- opening: does the first sentence say plainly what the email is — that the
  candidate is applying, for which role, at which company when the posting
  names one — before anything else? An email that opens mid-pitch, or that
  repeats the greeting, scores low.
- subject: does the subject line name the role and carry one real
  differentiator, stay under 80 characters, and avoid a bare field of study
  ("Application for computer engineering")?
- factuality: cross-check every claim against the profile JSON. Any
  employer, metric, date, or accomplishment not traceable to the profile
  must lower this score sharply.

Return only the structured scores; no commentary outside the schema.`

var rubricScoreSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"score":     map[string]any{"type": "integer"},
		"rationale": map[string]any{"type": "string"},
	},
	"required":             []string{"score", "rationale"},
	"additionalProperties": false,
}

var resumeRubricSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"selection":       rubricScoreSchema,
		"vocabulary":      rubricScoreSchema,
		"bulletStrength":  rubricScoreSchema,
		"honesty":         rubricScoreSchema,
		"gapQuality":      rubricScoreSchema,
		"headlineSummary": rubricScoreSchema,
	},
	"required":             []string{"selection", "vocabulary", "bulletStrength", "honesty", "gapQuality", "headlineSummary"},
	"additionalProperties": false,
}

var coverRubricSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"specificity": rubricScoreSchema,
		"voice":       rubricScoreSchema,
		"factuality":  rubricScoreSchema,
	},
	"required":             []string{"specificity", "voice", "factuality"},
	"additionalProperties": false,
}

var emailRubricSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"specificity": rubricScoreSchema,
		"opening":     rubricScoreSchema,
		"subject":     rubricScoreSchema,
		"factuality":  rubricScoreSchema,
	},
	"required":             []string{"specificity", "opening", "subject", "factuality"},
	"additionalProperties": false,
}

// JudgeResume scores a tailored resume against the Global Constraints
// rubric via one LLM call, given the same profile and JD text it was
// tailored from.
func JudgeResume(ctx context.Context, judge ai.LLM, p model.Profile, jdText string, t model.Tailored) (ResumeRubric, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return ResumeRubric{}, fmt.Errorf("eval: judge resume: marshal profile: %w", err)
	}
	tailoredJSON, err := json.Marshal(t)
	if err != nil {
		return ResumeRubric{}, fmt.Errorf("eval: judge resume: marshal tailored: %w", err)
	}

	blocks := []ai.ContentBlock{
		{Text: fmt.Sprintf("Candidate profile JSON (ground truth):\n%s", profileJSON)},
		{Text: fmt.Sprintf("Job description:\n%s", jdText)},
		{Text: fmt.Sprintf("Tailored resume JSON to judge:\n%s", tailoredJSON)},
	}

	raw, err := judge.GenerateJSON(ctx, judgeResumeSystemPrompt, blocks, resumeRubricSchema)
	if err != nil {
		return ResumeRubric{}, fmt.Errorf("eval: judge resume: %w", err)
	}

	var rubric ResumeRubric
	if err := json.Unmarshal(raw, &rubric); err != nil {
		return ResumeRubric{}, fmt.Errorf("eval: judge resume: unmarshal response: %w", err)
	}
	return rubric, nil
}

// JudgeCoverLetter scores a cover letter against the Global Constraints
// rubric via one LLM call, given the same profile and JD text it was
// written from.
func JudgeCoverLetter(ctx context.Context, judge ai.LLM, p model.Profile, jdText string, cl model.CoverLetter) (CoverRubric, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return CoverRubric{}, fmt.Errorf("eval: judge cover letter: marshal profile: %w", err)
	}
	coverJSON, err := json.Marshal(cl)
	if err != nil {
		return CoverRubric{}, fmt.Errorf("eval: judge cover letter: marshal cover letter: %w", err)
	}

	blocks := []ai.ContentBlock{
		{Text: fmt.Sprintf("Candidate profile JSON (ground truth):\n%s", profileJSON)},
		{Text: fmt.Sprintf("Job description:\n%s", jdText)},
		{Text: fmt.Sprintf("Cover letter JSON to judge:\n%s", coverJSON)},
	}

	raw, err := judge.GenerateJSON(ctx, judgeCoverLetterSystemPrompt, blocks, coverRubricSchema)
	if err != nil {
		return CoverRubric{}, fmt.Errorf("eval: judge cover letter: %w", err)
	}

	var rubric CoverRubric
	if err := json.Unmarshal(raw, &rubric); err != nil {
		return CoverRubric{}, fmt.Errorf("eval: judge cover letter: unmarshal response: %w", err)
	}
	return rubric, nil
}

// JudgeRecruiterEmail scores an application email against the same standard
// the pipeline promises, given the profile and JD text it was written from.
func JudgeRecruiterEmail(ctx context.Context, judge ai.LLM, p model.Profile, jdText string, re model.RecruiterEmail) (EmailRubric, error) {
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return EmailRubric{}, fmt.Errorf("eval: judge email: marshal profile: %w", err)
	}
	emailJSON, err := json.Marshal(re)
	if err != nil {
		return EmailRubric{}, fmt.Errorf("eval: judge email: marshal email: %w", err)
	}

	blocks := []ai.ContentBlock{
		{Text: fmt.Sprintf("Candidate profile JSON (ground truth):\n%s", profileJSON)},
		{Text: fmt.Sprintf("Job description:\n%s", jdText)},
		{Text: fmt.Sprintf("Application email JSON to judge:\n%s", emailJSON)},
	}

	raw, err := judge.GenerateJSON(ctx, judgeRecruiterEmailSystemPrompt, blocks, emailRubricSchema)
	if err != nil {
		return EmailRubric{}, fmt.Errorf("eval: judge email: %w", err)
	}

	var rubric EmailRubric
	if err := json.Unmarshal(raw, &rubric); err != nil {
		return EmailRubric{}, fmt.Errorf("eval: judge email: unmarshal response: %w", err)
	}
	return rubric, nil
}
