package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Check is one deterministic (code, not judge-opinion) pass/fail assertion
// against a single fixture's tailored output.
type Check struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// RubricScore is one judge-scored dimension: 0-10 plus a one-line rationale.
type RubricScore struct {
	Score     int    `json:"score"`
	Rationale string `json:"rationale"`
}

// ResumeRubric is the judge's score for a tailored resume, one field per
// Global Constraints rubric dimension.
type ResumeRubric struct {
	Selection       RubricScore `json:"selection"`
	Vocabulary      RubricScore `json:"vocabulary"`
	BulletStrength  RubricScore `json:"bulletStrength"`
	Honesty         RubricScore `json:"honesty"`
	GapQuality      RubricScore `json:"gapQuality"`
	HeadlineSummary RubricScore `json:"headlineSummary"`
}

// CoverRubric is the judge's score for a cover letter, evaluated only when
// -cover is set.
type CoverRubric struct {
	Specificity RubricScore `json:"specificity"`
	Voice       RubricScore `json:"voice"`
	Factuality  RubricScore `json:"factuality"`
}

// EmailRubric scores the forwardable application email — the artifact that
// actually reaches a recruiter, and until now the only generated artifact
// with no eval coverage at all.
type EmailRubric struct {
	Specificity RubricScore `json:"specificity"`
	Opening     RubricScore `json:"opening"`
	Subject     RubricScore `json:"subject"`
	Factuality  RubricScore `json:"factuality"`
}

// FixtureResult is one fixture's full eval outcome. Resume/Cover are nil
// when the corresponding judge call never ran (e.g. Error set because
// ai.Tailor itself failed, so there was nothing valid to judge).
type FixtureResult struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Checks []Check `json:"checks"`
	Error  string  `json:"error,omitempty"`
	// GenerationError is true when ai.Tailor failed for a harness/infra
	// reason (LLM call, marshal, or unmarshal — see eval.go's
	// isGuardrailError) rather than a true guardrail violation. Checks stays
	// empty in this case: there is no deterministic-check verdict to report,
	// and this fixture is excluded from DeterministicPassRate entirely (as
	// opposed to a guardrail violation, which IS a real failing check).
	GenerationError bool          `json:"generationError,omitempty"`
	Resume          *ResumeRubric `json:"resume,omitempty"`
	Cover           *CoverRubric  `json:"cover,omitempty"`
	CoverError      string        `json:"coverError,omitempty"`
	Email           *EmailRubric  `json:"email,omitempty"`
	EmailError      string        `json:"emailError,omitempty"`
}

// hasFailingGuardrailCheck reports whether f's deterministic checks include
// a failed "guardrail" entry (a true citation-guardrail violation, as
// opposed to a GenerationError harness failure or a clean pass).
func (f FixtureResult) hasFailingGuardrailCheck() bool {
	for _, c := range f.Checks {
		if c.Name == "guardrail" && !c.Pass {
			return true
		}
	}
	return false
}

// Aggregate summarizes a Report's fixtures into run-level numbers.
//
// IMPORTANT for comparing two runs: OverallMean and MeanByDimension are only
// computed over fixtures that were actually scored (ScoredFixtures) — a
// fixture that errored out of generation, or that failed the citation
// guardrail, never reaches the judge and silently drops out of those means.
// A higher OverallMean on a run with fewer ScoredFixtures/TotalFixtures than
// its baseline is NOT an improvement; it may just mean more fixtures failed
// and the survivors were easier to score well. Always read OverallMean
// alongside DeterministicPassRate, TotalFixtures, ErroredFixtures, and
// GuardrailFailedFixtures together, never OverallMean alone.
type Aggregate struct {
	// MeanByDimension is the mean judge score per rubric dimension name
	// (e.g. "selection", "specificity"), averaged only over fixtures where
	// that dimension was actually scored.
	MeanByDimension map[string]float64 `json:"meanByDimension"`
	// ScoredFixtures is, per rubric dimension, how many fixtures
	// contributed a score to that dimension's MeanByDimension entry. Resume
	// dimensions are always scored together (one judge call), and cover
	// dimensions (only present with -cover) are always scored together (a
	// second judge call) — but the two counts can differ from each other,
	// and both can be less than TotalFixtures, hence a per-dimension map
	// rather than one number.
	ScoredFixtures map[string]int `json:"scoredFixtures"`
	// OverallMean is the unweighted mean of MeanByDimension's values, so a
	// -cover run's 3 extra cover dimensions don't dilute the 6 resume
	// dimensions just because there are more of them.
	OverallMean float64 `json:"overallMean"`
	// DeterministicPassRate is checks passed / checks run, across every
	// scored or guardrail-failed fixture's deterministic Check list.
	// GenerationError fixtures contribute no checks and are excluded.
	DeterministicPassRate float64 `json:"deterministicPassRate"`
	// TotalFixtures is len(Report.Fixtures).
	TotalFixtures int `json:"totalFixtures"`
	// ErroredFixtures counts fixtures that failed for a harness/infra
	// reason: ai.Tailor's GenerationError, or a judge call itself failing
	// after a successful, checked tailor. Distinct from
	// GuardrailFailedFixtures, which is a real (if unwanted) result.
	ErroredFixtures int `json:"erroredFixtures"`
	// GuardrailFailedFixtures counts fixtures where the model's tailored
	// output cited a fabricated profile id — the citation guardrail itself
	// (model.ValidateTailored) rejected it.
	GuardrailFailedFixtures int `json:"guardrailFailedFixtures"`
}

// Report is one full eval run: a label, every fixture's result, and the
// aggregate rollup.
type Report struct {
	Label       string          `json:"label"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Fixtures    []FixtureResult `json:"fixtures"`
	Aggregate   Aggregate       `json:"aggregate"`
}

// resumeDimensionOrder and coverDimensionOrder fix the column/row order used
// by both aggregate() (for stable map iteration) and Render().
var resumeDimensionOrder = []string{"selection", "vocabulary", "bulletStrength", "honesty", "gapQuality", "headlineSummary"}
var coverDimensionOrder = []string{"specificity", "voice", "factuality"}
var emailDimensionOrder = []string{"emailSpecificity", "emailOpening", "emailSubject", "emailFactuality"}

func aggregate(fixtures []FixtureResult) Aggregate {
	sums := map[string]float64{}
	counts := map[string]int{}
	addScore := func(name string, s RubricScore) {
		sums[name] += float64(s.Score)
		counts[name]++
	}

	checksTotal, checksPassed := 0, 0
	errored, guardrailFailed := 0, 0

	for _, f := range fixtures {
		for _, c := range f.Checks {
			checksTotal++
			if c.Pass {
				checksPassed++
			}
		}

		switch {
		case f.GenerationError:
			errored++
		case f.hasFailingGuardrailCheck():
			guardrailFailed++
		case f.Resume == nil && f.Error != "":
			// Tailor + its deterministic checks succeeded, but the judge
			// call itself failed — still a harness error, not a guardrail
			// violation or a scored result.
			errored++
		}

		if f.Resume != nil {
			addScore("selection", f.Resume.Selection)
			addScore("vocabulary", f.Resume.Vocabulary)
			addScore("bulletStrength", f.Resume.BulletStrength)
			addScore("honesty", f.Resume.Honesty)
			addScore("gapQuality", f.Resume.GapQuality)
			addScore("headlineSummary", f.Resume.HeadlineSummary)
		}
		if f.Cover != nil {
			addScore("specificity", f.Cover.Specificity)
			addScore("voice", f.Cover.Voice)
			addScore("factuality", f.Cover.Factuality)
		}
		// The email's dimensions are namespaced: two of them share a name
		// with the cover letter's, and averaging an email's specificity into
		// a letter's would hide a regression in either.
		if f.Email != nil {
			addScore("emailSpecificity", f.Email.Specificity)
			addScore("emailOpening", f.Email.Opening)
			addScore("emailSubject", f.Email.Subject)
			addScore("emailFactuality", f.Email.Factuality)
		}
	}

	means := map[string]float64{}
	var sumOfMeans float64
	for name, sum := range sums {
		m := sum / float64(counts[name])
		means[name] = m
		sumOfMeans += m
	}

	var overall float64
	if len(means) > 0 {
		overall = sumOfMeans / float64(len(means))
	}

	var passRate float64
	if checksTotal > 0 {
		passRate = float64(checksPassed) / float64(checksTotal)
	}

	return Aggregate{
		MeanByDimension:         means,
		ScoredFixtures:          counts,
		OverallMean:             overall,
		DeterministicPassRate:   passRate,
		TotalFixtures:           len(fixtures),
		ErroredFixtures:         errored,
		GuardrailFailedFixtures: guardrailFailed,
	}
}

// Render writes an aligned text table: a scored/errored/guardrail-failed
// summary line, one row per fixture (status, deterministic checks
// passed/total, each rubric dimension score, any error), and the aggregate
// rollup.
func (r Report) Render(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)

	fmt.Fprintf(tw, "cvx eval report — label=%s generated=%s\n\n", r.Label, r.GeneratedAt.Format(time.RFC3339))

	scored := r.Aggregate.TotalFixtures - r.Aggregate.ErroredFixtures - r.Aggregate.GuardrailFailedFixtures
	fmt.Fprintf(tw, "scored %d of %d fixtures (%d errored, %d guardrail-failed)\n", scored, r.Aggregate.TotalFixtures, r.Aggregate.ErroredFixtures, r.Aggregate.GuardrailFailedFixtures)
	fmt.Fprintln(tw, "note: compare pass rates and fixture counts alongside means when judging two runs — a higher mean over fewer scored fixtures is not an improvement")
	fmt.Fprintln(tw)

	header := []string{"ID", "STATUS", "CHECKS"}
	for _, d := range resumeDimensionOrder {
		header = append(header, strings.ToUpper(d))
	}
	header = append(header, "COVER_AVG", "EMAIL_AVG", "ERROR")
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	for _, f := range r.Fixtures {
		passed := 0
		for _, c := range f.Checks {
			if c.Pass {
				passed++
			}
		}

		status := "ok"
		switch {
		case f.GenerationError:
			status = "error"
		case f.hasFailingGuardrailCheck():
			status = "guardrail_fail"
		case f.Resume == nil && f.Error != "":
			status = "error"
		}

		row := []string{f.ID, status, fmt.Sprintf("%d/%d", passed, len(f.Checks))}

		if f.Resume != nil {
			scores := map[string]int{
				"selection":       f.Resume.Selection.Score,
				"vocabulary":      f.Resume.Vocabulary.Score,
				"bulletStrength":  f.Resume.BulletStrength.Score,
				"honesty":         f.Resume.Honesty.Score,
				"gapQuality":      f.Resume.GapQuality.Score,
				"headlineSummary": f.Resume.HeadlineSummary.Score,
			}
			for _, d := range resumeDimensionOrder {
				row = append(row, strconv.Itoa(scores[d]))
			}
		} else {
			for range resumeDimensionOrder {
				row = append(row, "-")
			}
		}

		if f.Cover != nil {
			avg := float64(f.Cover.Specificity.Score+f.Cover.Voice.Score+f.Cover.Factuality.Score) / 3
			row = append(row, fmt.Sprintf("%.1f", avg))
		} else {
			row = append(row, "-")
		}

		if f.Email != nil {
			avg := float64(f.Email.Specificity.Score+f.Email.Opening.Score+f.Email.Subject.Score+f.Email.Factuality.Score) / 4
			row = append(row, fmt.Sprintf("%.1f", avg))
		} else {
			row = append(row, "-")
		}

		errStr := f.Error
		if errStr == "" {
			errStr = f.CoverError
		}
		if errStr == "" {
			errStr = f.EmailError
		}
		row = append(row, errStr)

		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	fmt.Fprintln(tw)
	fmt.Fprintf(tw, "deterministic pass rate\t%.1f%%\n", r.Aggregate.DeterministicPassRate*100)
	allDimensions := append(append([]string{}, resumeDimensionOrder...), coverDimensionOrder...)
	allDimensions = append(allDimensions, emailDimensionOrder...)
	for _, d := range allDimensions {
		if m, ok := r.Aggregate.MeanByDimension[d]; ok {
			fmt.Fprintf(tw, "%s\t%.2f\t(n=%d)\n", d, m, r.Aggregate.ScoredFixtures[d])
		}
	}
	fmt.Fprintf(tw, "overall mean\t%.2f\n", r.Aggregate.OverallMean)

	return tw.Flush()
}

// resultTimestampFormat is a filesystem- and sort-safe stand-in for
// RFC3339 (no colons), used only for result filenames.
const resultTimestampFormat = "20060102T150405Z"

// Save writes r as pretty JSON to resultsDir/<UTC timestamp>-<r.Label>.json,
// creating resultsDir if needed, and returns the path written.
func (r Report) Save(resultsDir string) (string, error) {
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		return "", fmt.Errorf("eval: mkdir %s: %w", resultsDir, err)
	}

	name := fmt.Sprintf("%s-%s.json", r.GeneratedAt.UTC().Format(resultTimestampFormat), r.Label)
	path := filepath.Join(resultsDir, name)

	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("eval: marshal report: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", fmt.Errorf("eval: write %s: %w", path, err)
	}
	return path, nil
}
