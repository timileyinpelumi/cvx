package eval

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func sampleFixturesForAggregate() []FixtureResult {
	return []FixtureResult{
		{
			ID: "f1",
			Checks: []Check{
				{Name: "guardrail", Pass: true},
				{Name: "itemCount", Pass: true},
				{Name: "summary", Pass: false},
			},
			Resume: &ResumeRubric{
				Selection:       RubricScore{Score: 6},
				Vocabulary:      RubricScore{Score: 7},
				BulletStrength:  RubricScore{Score: 8},
				Honesty:         RubricScore{Score: 9},
				GapQuality:      RubricScore{Score: 5},
				HeadlineSummary: RubricScore{Score: 6},
			},
		},
		{
			ID: "f2",
			Checks: []Check{
				{Name: "guardrail", Pass: false, Detail: "fabricated id"},
			},
			Error: "fabricated id",
		},
		{
			ID: "f3",
			Checks: []Check{
				{Name: "guardrail", Pass: true},
				{Name: "itemCount", Pass: true},
				{Name: "bulletsPerItem", Pass: true},
				{Name: "summary", Pass: true},
			},
			Resume: &ResumeRubric{
				Selection:       RubricScore{Score: 8},
				Vocabulary:      RubricScore{Score: 9},
				BulletStrength:  RubricScore{Score: 10},
				Honesty:         RubricScore{Score: 10},
				GapQuality:      RubricScore{Score: 7},
				HeadlineSummary: RubricScore{Score: 8},
			},
			Cover: &CoverRubric{
				Specificity: RubricScore{Score: 8},
				Voice:       RubricScore{Score: 9},
				Factuality:  RubricScore{Score: 10},
			},
		},
		{
			// A harness/infra failure (e.g. the LLM call itself errored) —
			// distinct from f2's true guardrail violation. No checks ran,
			// so it must contribute nothing to DeterministicPassRate, and
			// must be counted separately from GuardrailFailedFixtures.
			ID:              "f4",
			GenerationError: true,
			Error:           "tailor: request timed out",
		},
	}
}

func TestAggregateDeterministicPassRate(t *testing.T) {
	agg := aggregate(sampleFixturesForAggregate())
	// checks: f1 has 3 (2 pass), f2 has 1 (0 pass), f3 has 4 (4 pass),
	// f4 (GenerationError) contributes 0 checks => 6/8, f4 excluded entirely.
	want := 6.0 / 8.0
	if !almostEqual(agg.DeterministicPassRate, want) {
		t.Fatalf("want pass rate %v, got %v", want, agg.DeterministicPassRate)
	}
}

func TestAggregateFixtureCounts(t *testing.T) {
	agg := aggregate(sampleFixturesForAggregate())
	// f1 scored, f2 guardrail-failed, f3 scored (+cover), f4 errored (infra).
	if agg.TotalFixtures != 4 {
		t.Errorf("want TotalFixtures=4, got %d", agg.TotalFixtures)
	}
	if agg.ErroredFixtures != 1 {
		t.Errorf("want ErroredFixtures=1, got %d", agg.ErroredFixtures)
	}
	if agg.GuardrailFailedFixtures != 1 {
		t.Errorf("want GuardrailFailedFixtures=1, got %d", agg.GuardrailFailedFixtures)
	}

	wantScored := map[string]int{
		"selection": 2, "vocabulary": 2, "bulletStrength": 2, "honesty": 2, "gapQuality": 2, "headlineSummary": 2,
		"specificity": 1, "voice": 1, "factuality": 1,
	}
	for dim, want := range wantScored {
		if got := agg.ScoredFixtures[dim]; got != want {
			t.Errorf("ScoredFixtures[%s]: want %d, got %d", dim, want, got)
		}
	}
}

func TestAggregateMeanByDimension(t *testing.T) {
	agg := aggregate(sampleFixturesForAggregate())

	cases := map[string]float64{
		"selection":       7,
		"vocabulary":      8,
		"bulletStrength":  9,
		"honesty":         9.5,
		"gapQuality":      6,
		"headlineSummary": 7,
		"specificity":     8,
		"voice":           9,
		"factuality":      10,
	}
	for dim, want := range cases {
		got, ok := agg.MeanByDimension[dim]
		if !ok {
			t.Fatalf("missing dimension %s", dim)
		}
		if !almostEqual(got, want) {
			t.Errorf("dimension %s: want %v, got %v", dim, want, got)
		}
	}
	if len(agg.MeanByDimension) != len(cases) {
		t.Fatalf("want %d dimensions, got %d: %+v", len(cases), len(agg.MeanByDimension), agg.MeanByDimension)
	}
}

func TestAggregateOverallMean(t *testing.T) {
	agg := aggregate(sampleFixturesForAggregate())
	// unweighted mean of the 9 dimension means above:
	// (7+8+9+9.5+6+7+8+9+10)/9
	want := (7.0 + 8 + 9 + 9.5 + 6 + 7 + 8 + 9 + 10) / 9
	if !almostEqual(agg.OverallMean, want) {
		t.Fatalf("want overall mean %v, got %v", want, agg.OverallMean)
	}
}

func TestAggregateEmpty(t *testing.T) {
	agg := aggregate(nil)
	if agg.OverallMean != 0 || agg.DeterministicPassRate != 0 {
		t.Fatalf("want zero-value aggregate for no fixtures, got %+v", agg)
	}
	if len(agg.MeanByDimension) != 0 {
		t.Fatalf("want empty MeanByDimension, got %+v", agg.MeanByDimension)
	}
	if agg.TotalFixtures != 0 || agg.ErroredFixtures != 0 || agg.GuardrailFailedFixtures != 0 {
		t.Fatalf("want zero fixture counts, got %+v", agg)
	}
}

func sampleReport() Report {
	fixtures := sampleFixturesForAggregate()
	return Report{
		Label:       "test-run",
		GeneratedAt: time.Date(2026, 8, 2, 12, 30, 0, 0, time.UTC),
		Fixtures:    fixtures,
		Aggregate:   aggregate(fixtures),
	}
}

func TestRenderIncludesKeyInformation(t *testing.T) {
	var buf bytes.Buffer
	if err := sampleReport().Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"test-run", "f1", "f2", "f3", "f4",
		"deterministic pass rate", "overall mean", "fabricated id", "tailor: request timed out",
		"scored 2 of 4 fixtures (1 errored, 1 guardrail-failed)",
		"guardrail_fail", "error", "ok",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderDistinguishesGenerationErrorFromGuardrailFail(t *testing.T) {
	var buf bytes.Buffer
	r := Report{
		Label:       "l",
		GeneratedAt: time.Now().UTC(),
		Fixtures: []FixtureResult{
			{ID: "guardrail-fixture", Checks: []Check{{Name: "guardrail", Pass: false, Detail: "bad id"}}, Error: "bad id"},
			{ID: "infra-fixture", GenerationError: true, Error: "tailor: connection refused"},
		},
	}
	r.Aggregate = aggregate(r.Fixtures)
	if err := r.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	lineFor := func(id string) string {
		for _, line := range strings.Split(buf.String(), "\n") {
			if strings.HasPrefix(line, id) {
				return line
			}
		}
		t.Fatalf("no row found for %s\n---\n%s", id, buf.String())
		return ""
	}

	if line := lineFor("guardrail-fixture"); !strings.Contains(line, "guardrail_fail") {
		t.Errorf("want guardrail-fixture row status guardrail_fail, got: %s", line)
	}
	if line := lineFor("infra-fixture"); !strings.Contains(line, "error") || strings.Contains(line, "guardrail_fail") {
		t.Errorf("want infra-fixture row status error (not guardrail_fail), got: %s", line)
	}
}

func TestRenderHandlesMissingResumeAndCover(t *testing.T) {
	var buf bytes.Buffer
	r := Report{
		Label:       "l",
		GeneratedAt: time.Now().UTC(),
		Fixtures: []FixtureResult{
			{ID: "only-guardrail-fail", Checks: []Check{{Name: "guardrail", Pass: false, Detail: "bad id"}}, Error: "bad id"},
		},
	}
	r.Aggregate = aggregate(r.Fixtures)
	if err := r.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), "only-guardrail-fail") {
		t.Fatalf("missing fixture id in output:\n%s", buf.String())
	}
}

func TestSaveWritesJSONAndReturnsPath(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, "results")

	r := sampleReport()
	path, err := r.Save(resultsDir)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasPrefix(path, resultsDir) {
		t.Fatalf("want path under %s, got %s", resultsDir, path)
	}
	if !strings.HasSuffix(path, "-test-run.json") {
		t.Fatalf("want path ending in -test-run.json, got %s", path)
	}
	if !strings.Contains(filepath.Base(path), "20260802T123000Z") {
		t.Fatalf("want UTC timestamp in filename, got %s", filepath.Base(path))
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved report: %v", err)
	}
	var roundTripped Report
	if err := json.Unmarshal(b, &roundTripped); err != nil {
		t.Fatalf("unmarshal saved report: %v", err)
	}
	if roundTripped.Label != "test-run" {
		t.Fatalf("want label test-run, got %q", roundTripped.Label)
	}
	if len(roundTripped.Fixtures) != len(r.Fixtures) {
		t.Fatalf("want %d fixtures round-tripped, got %d", len(r.Fixtures), len(roundTripped.Fixtures))
	}
	if !almostEqual(roundTripped.Aggregate.OverallMean, r.Aggregate.OverallMean) {
		t.Fatalf("aggregate did not round-trip: want %v, got %v", r.Aggregate.OverallMean, roundTripped.Aggregate.OverallMean)
	}
}

func TestSaveCreatesResultsDir(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, "nested", "results")

	if _, err := os.Stat(resultsDir); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s should not exist yet", resultsDir)
	}

	r := sampleReport()
	if _, err := r.Save(resultsDir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(resultsDir); err != nil {
		t.Fatalf("want resultsDir created, got %v", err)
	}
}
