// Package eval is the cvx quality eval harness: fixed JD fixtures x a fixture
// profile → tailor (+ optional cover letter) → deterministic checks →
// LLM-judge rubric → a scored Report. Dev-only tooling; touches no runtime
// path.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cvx/internal/ai"
	"cvx/internal/model"
)

// FixtureIndexEntry is one entry in fixtures/index.json.
type FixtureIndexEntry struct {
	ID    string `json:"id"`
	File  string `json:"file"`
	Title string `json:"title"`
}

func loadFixtureIndex(fixturesDir string) ([]FixtureIndexEntry, error) {
	b, err := os.ReadFile(filepath.Join(fixturesDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("eval: read index.json: %w", err)
	}
	var entries []FixtureIndexEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("eval: unmarshal index.json: %w", err)
	}
	return entries, nil
}

func loadFixtureProfile(fixturesDir string) (model.Profile, error) {
	b, err := os.ReadFile(filepath.Join(fixturesDir, "profile.json"))
	if err != nil {
		return model.Profile{}, fmt.Errorf("eval: read profile.json: %w", err)
	}
	var p model.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return model.Profile{}, fmt.Errorf("eval: unmarshal profile.json: %w", err)
	}
	return p, nil
}

// filterIDs returns entries whose ID is in ids, preserving index.json's
// order, or all entries when ids is empty. It errors on any id with no
// matching entry so a typo in -ids fails fast instead of silently running
// fewer fixtures than intended.
func filterIDs(entries []FixtureIndexEntry, ids []string) ([]FixtureIndexEntry, error) {
	if len(ids) == 0 {
		return entries, nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []FixtureIndexEntry
	for _, e := range entries {
		if want[e.ID] {
			out = append(out, e)
			delete(want, e.ID)
		}
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for id := range want {
			missing = append(missing, id)
		}
		return nil, fmt.Errorf("eval: unknown fixture id(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// deterministicChecks runs every Global-Constraints deterministic check
// against a tailored output that has already passed model.ValidateTailored
// (the caller only reaches here on that success path — see Run).
func deterministicChecks(p model.Profile, t model.Tailored) []Check {
	return []Check{
		{Name: "guardrail", Pass: true, Detail: "ok"},
		itemCountCheck(t),
		bulletsPerItemCheck(t),
		summaryCheck(t),
		headlineCheck(t),
		gapsCheck(t),
		selectedSkillsCheck(p, t),
	}
}

func itemCountCheck(t model.Tailored) Check {
	n := 0
	for _, s := range t.Sections {
		n += len(s.Items)
	}
	if n <= 5 {
		return Check{Name: "itemCount", Pass: true, Detail: fmt.Sprintf("%d items", n)}
	}
	return Check{Name: "itemCount", Pass: false, Detail: fmt.Sprintf("%d items, want <=5", n)}
}

func bulletsPerItemCheck(t model.Tailored) Check {
	var bad []string
	for _, s := range t.Sections {
		for _, it := range s.Items {
			n := len(it.Bullets)
			if n < 2 || n > 4 {
				bad = append(bad, fmt.Sprintf("%s has %d bullets", it.SourceID, n))
			}
		}
	}
	if len(bad) == 0 {
		return Check{Name: "bulletsPerItem", Pass: true, Detail: "all items have 2-4 bullets"}
	}
	return Check{Name: "bulletsPerItem", Pass: false, Detail: strings.Join(bad, "; ") + " (want 2-4)"}
}

func summaryCheck(t model.Tailored) Check {
	if strings.TrimSpace(t.Summary) == "" {
		return Check{Name: "summary", Pass: false, Detail: "summary is empty"}
	}
	words := len(strings.Fields(t.Summary))
	if words > 60 {
		return Check{Name: "summary", Pass: false, Detail: fmt.Sprintf("summary is %d words, want <=60", words)}
	}
	return Check{Name: "summary", Pass: true, Detail: fmt.Sprintf("%d words", words)}
}

func headlineCheck(t model.Tailored) Check {
	if strings.TrimSpace(t.Headline) == "" {
		return Check{Name: "headline", Pass: false, Detail: "headline is empty"}
	}
	return Check{Name: "headline", Pass: true, Detail: "non-empty"}
}

func gapsCheck(t model.Tailored) Check {
	var bad []string
	for i, g := range t.Gaps {
		if strings.TrimSpace(g.Requirement) == "" || (g.Severity != "missing" && g.Severity != "weak") {
			bad = append(bad, fmt.Sprintf("gap[%d]: requirement=%q severity=%q", i, g.Requirement, g.Severity))
		}
	}
	if len(bad) == 0 {
		return Check{Name: "gapsWellFormed", Pass: true, Detail: fmt.Sprintf("%d gaps", len(t.Gaps))}
	}
	return Check{Name: "gapsWellFormed", Pass: false, Detail: strings.Join(bad, "; ")}
}

func selectedSkillsCheck(p model.Profile, t model.Tailored) Check {
	profileSkills := map[string]bool{}
	for _, s := range p.Skills {
		profileSkills[strings.ToLower(s)] = true
	}
	var bad []string
	for _, s := range t.SelectedSkills {
		if !profileSkills[strings.ToLower(s)] {
			bad = append(bad, s)
		}
	}
	if len(bad) == 0 {
		return Check{Name: "selectedSkillsSubset", Pass: true, Detail: fmt.Sprintf("%d skills, all in profile", len(t.SelectedSkills))}
	}
	return Check{Name: "selectedSkillsSubset", Pass: false, Detail: "not in profile: " + strings.Join(bad, ", ")}
}

// tailorErrorPrefix is the wrapping ai.Tailor's tailor.go applies to every
// error EXCEPT the one from model.ValidateTailored: marshal-profile,
// LLM-call, and unmarshal-response failures are all `fmt.Errorf("tailor:
// ...", err)`, while the guardrail's own error is returned unwrapped. That
// asymmetry is intentional upstream (it's how tailor.go's own callers were
// already written) and is the only signal eval.Run has to tell "the
// generator/network/provider misbehaved" apart from "the model actually
// violated the citation guardrail" without ai.Tailor exposing a typed error.
const tailorErrorPrefix = "tailor: "

// isGuardrailError reports whether err came from model.ValidateTailored
// (a true guardrail violation: the model cited a fabricated id) as opposed
// to an infra/generation failure inside ai.Tailor (LLM call, marshaling, or
// response parsing) — see tailorErrorPrefix.
func isGuardrailError(err error) bool {
	return !strings.HasPrefix(err.Error(), tailorErrorPrefix)
}

// Run executes the eval harness for the selected fixtures against gen (the
// generator under test) and judge (the rubric scorer, normally a different
// model so it never grades itself). Fixtures run strictly sequentially, in
// index.json order, to stay easy on provider rate limits and keep runs
// reproducible.
//
// Per fixture: ai.Tailor (existing prompt, unmodified — its own guardrail
// call to model.ValidateTailored is what backs the "guardrail" deterministic
// check) → the rest of the deterministic checks → one judge call for the
// resume rubric → optionally a cover letter + its own judge call.
//
// A Tailor failure aborts just that fixture, and the two failure modes are
// recorded differently (see Aggregate's doc comment on why this distinction
// matters for comparing runs):
//   - A true guardrail violation (isGuardrailError) is a real deterministic
//     check result: FixtureResult.Checks gets a single failing "guardrail"
//     entry, which DOES count toward DeterministicPassRate.
//   - Any other Tailor failure (LLM call, marshal, or unmarshal) is a
//     harness/infra error, not a judgment about the generator's output:
//     FixtureResult.GenerationError is set instead, Checks stays empty, and
//     it is excluded from DeterministicPassRate entirely.
//
// Either way there is nothing valid to run the remaining checks or the
// judge against, so the fixture is skipped past that point.
func Run(ctx context.Context, gen ai.LLM, judge ai.LLM, fixturesDir string, ids []string, coverLetters bool) (Report, error) {
	entries, err := loadFixtureIndex(fixturesDir)
	if err != nil {
		return Report{}, err
	}
	entries, err = filterIDs(entries, ids)
	if err != nil {
		return Report{}, err
	}
	profile, err := loadFixtureProfile(fixturesDir)
	if err != nil {
		return Report{}, err
	}

	report := Report{GeneratedAt: time.Now().UTC()}

	for _, e := range entries {
		jdBytes, err := os.ReadFile(filepath.Join(fixturesDir, e.File))
		if err != nil {
			return Report{}, fmt.Errorf("eval: read %s: %w", e.File, err)
		}
		jdText := string(jdBytes)

		fr := FixtureResult{ID: e.ID, Title: e.Title}

		tailored, err := ai.Tailor(ctx, gen, profile, jdText)
		if err != nil {
			if isGuardrailError(err) {
				fr.Checks = []Check{{Name: "guardrail", Pass: false, Detail: err.Error()}}
			} else {
				fr.GenerationError = true
			}
			fr.Error = err.Error()
			report.Fixtures = append(report.Fixtures, fr)
			continue
		}
		fr.Checks = deterministicChecks(profile, tailored)

		rubric, err := JudgeResume(ctx, judge, profile, jdText, tailored)
		if err != nil {
			fr.Error = fmt.Sprintf("judge resume: %v", err)
			report.Fixtures = append(report.Fixtures, fr)
			continue
		}
		fr.Resume = &rubric

		if coverLetters {
			cl, err := ai.CoverLetter(ctx, gen, profile, jdText)
			if err != nil {
				fr.CoverError = err.Error()
			} else if coverRubric, err := JudgeCoverLetter(ctx, judge, profile, jdText, cl); err != nil {
				fr.CoverError = fmt.Sprintf("judge cover letter: %v", err)
			} else {
				fr.Cover = &coverRubric
			}
		}

		report.Fixtures = append(report.Fixtures, fr)
	}

	report.Aggregate = aggregate(report.Fixtures)
	return report, nil
}
