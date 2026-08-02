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

// FixtureResult is one fixture's full eval outcome. Resume/Cover are nil
// when the corresponding judge call never ran (e.g. Error set because
// ai.Tailor itself failed, so there was nothing valid to judge).
type FixtureResult struct {
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	Checks     []Check       `json:"checks"`
	Error      string        `json:"error,omitempty"`
	Resume     *ResumeRubric `json:"resume,omitempty"`
	Cover      *CoverRubric  `json:"cover,omitempty"`
	CoverError string        `json:"coverError,omitempty"`
}

// Aggregate summarizes a Report's fixtures into run-level numbers.
type Aggregate struct {
	// MeanByDimension is the mean judge score per rubric dimension name
	// (e.g. "selection", "specificity"), averaged only over fixtures where
	// that dimension was actually scored.
	MeanByDimension map[string]float64 `json:"meanByDimension"`
	// OverallMean is the unweighted mean of MeanByDimension's values, so a
	// -cover run's 3 extra cover dimensions don't dilute the 6 resume
	// dimensions just because there are more of them.
	OverallMean float64 `json:"overallMean"`
	// DeterministicPassRate is checks passed / checks run, across every
	// fixture's deterministic Check list.
	DeterministicPassRate float64 `json:"deterministicPassRate"`
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

func aggregate(fixtures []FixtureResult) Aggregate {
	sums := map[string]float64{}
	counts := map[string]int{}
	addScore := func(name string, s RubricScore) {
		sums[name] += float64(s.Score)
		counts[name]++
	}

	checksTotal, checksPassed := 0, 0

	for _, f := range fixtures {
		for _, c := range f.Checks {
			checksTotal++
			if c.Pass {
				checksPassed++
			}
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
		MeanByDimension:       means,
		OverallMean:           overall,
		DeterministicPassRate: passRate,
	}
}

// Render writes an aligned text table: one row per fixture (deterministic
// checks passed/total, each rubric dimension score, any error), followed by
// the aggregate rollup.
func (r Report) Render(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)

	fmt.Fprintf(tw, "cvx eval report — label=%s generated=%s\n\n", r.Label, r.GeneratedAt.Format(time.RFC3339))

	header := []string{"ID", "CHECKS"}
	for _, d := range resumeDimensionOrder {
		header = append(header, strings.ToUpper(d))
	}
	header = append(header, "COVER_AVG", "ERROR")
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	for _, f := range r.Fixtures {
		passed := 0
		for _, c := range f.Checks {
			if c.Pass {
				passed++
			}
		}
		row := []string{f.ID, fmt.Sprintf("%d/%d", passed, len(f.Checks))}

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

		errStr := f.Error
		if errStr == "" {
			errStr = f.CoverError
		}
		row = append(row, errStr)

		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	fmt.Fprintln(tw)
	fmt.Fprintf(tw, "deterministic pass rate\t%.1f%%\n", r.Aggregate.DeterministicPassRate*100)
	for _, d := range append(append([]string{}, resumeDimensionOrder...), coverDimensionOrder...) {
		if m, ok := r.Aggregate.MeanByDimension[d]; ok {
			fmt.Fprintf(tw, "%s\t%.2f\n", d, m)
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
