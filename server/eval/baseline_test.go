package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func baselineAggregate() Aggregate {
	return Aggregate{
		MeanByDimension:       map[string]float64{"selection": 8.0, "honesty": 9.0},
		ScoredFixtures:        map[string]int{"selection": 4, "honesty": 4},
		OverallMean:           8.5,
		DeterministicPassRate: 1,
		TotalFixtures:         4,
	}
}

func TestCompareAggregatesPassesAnEqualRun(t *testing.T) {
	if got := compareAggregates(baselineAggregate(), baselineAggregate()); len(got) != 0 {
		t.Fatalf("an identical run must pass the gate: %v", got)
	}
}

// Judge scores wobble, so a small dip is not a regression; a real drop is.
func TestCompareAggregatesToleratesJudgeNoise(t *testing.T) {
	cur := baselineAggregate()
	cur.MeanByDimension = map[string]float64{"selection": 7.7, "honesty": 9.0}
	if got := compareAggregates(baselineAggregate(), cur); len(got) != 0 {
		t.Fatalf("0.3 of judge noise must not fail the gate: %v", got)
	}

	cur.MeanByDimension = map[string]float64{"selection": 7.0, "honesty": 9.0}
	if got := strings.Join(compareAggregates(baselineAggregate(), cur), "; "); !strings.Contains(got, "selection fell") {
		t.Fatalf("want the selection drop reported, got %q", got)
	}
}

// The failure modes that make a mean meaningless have no tolerance at all.
func TestCompareAggregatesCatchesShrinkingCoverage(t *testing.T) {
	cases := map[string]func(*Aggregate){
		"fewer fixtures":     func(a *Aggregate) { a.TotalFixtures = 3 },
		"more errors":        func(a *Aggregate) { a.ErroredFixtures = 1 },
		"guardrail failures": func(a *Aggregate) { a.GuardrailFailedFixtures = 1 },
		"pass rate fell":     func(a *Aggregate) { a.DeterministicPassRate = 0.99 },
		"dimension not run":  func(a *Aggregate) { delete(a.MeanByDimension, "honesty") },
		"overall mean fell":  func(a *Aggregate) { a.OverallMean = 8.0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cur := baselineAggregate()
			cur.MeanByDimension = map[string]float64{"selection": 8.0, "honesty": 9.0}
			mutate(&cur)
			if got := compareAggregates(baselineAggregate(), cur); len(got) == 0 {
				t.Fatal("want a regression reported")
			}
		})
	}
}

func TestCompareToBaselineReadsASavedReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	b, err := json.Marshal(Report{Aggregate: baselineAggregate()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	worse := baselineAggregate()
	worse.DeterministicPassRate = 0.5
	got, err := CompareToBaseline(Report{Aggregate: worse}, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("want the pass-rate drop reported")
	}

	if _, err := CompareToBaseline(Report{}, filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("want an error for a missing baseline")
	}
}
