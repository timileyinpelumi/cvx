package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Tolerances for the gate. Judge scores are noisy run to run, so a dimension
// has to drop by more than judgeTolerance before it counts as a regression;
// the deterministic pass rate is not noisy at all, so it may not drop at
// all, and neither may fixture coverage.
const (
	judgeTolerance   = 0.5
	overallTolerance = 0.3
)

// CompareToBaseline reports how a run regressed against a saved baseline
// report. An empty result means the run is at least as good, which is what
// makes this usable as a build gate: nothing about the eval fails a build
// today, so a prompt edit can quietly cost a point on every dimension and
// nobody finds out.
func CompareToBaseline(current Report, baselinePath string) ([]string, error) {
	b, err := os.ReadFile(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("eval: read baseline: %w", err)
	}
	var base Report
	if err := json.Unmarshal(b, &base); err != nil {
		return nil, fmt.Errorf("eval: unmarshal baseline: %w", err)
	}
	return compareAggregates(base.Aggregate, current.Aggregate), nil
}

func compareAggregates(base, cur Aggregate) []string {
	var out []string

	// Fewer scored fixtures makes every mean incomparable, so it is checked
	// first and reported as its own failure.
	if cur.TotalFixtures < base.TotalFixtures {
		out = append(out, fmt.Sprintf("ran %d fixtures, baseline ran %d", cur.TotalFixtures, base.TotalFixtures))
	}
	if cur.ErroredFixtures > base.ErroredFixtures {
		out = append(out, fmt.Sprintf("%d fixtures errored, baseline had %d", cur.ErroredFixtures, base.ErroredFixtures))
	}
	if cur.GuardrailFailedFixtures > base.GuardrailFailedFixtures {
		out = append(out, fmt.Sprintf("%d fixtures failed the citation guardrail, baseline had %d",
			cur.GuardrailFailedFixtures, base.GuardrailFailedFixtures))
	}
	if cur.DeterministicPassRate < base.DeterministicPassRate {
		out = append(out, fmt.Sprintf("deterministic pass rate fell to %.1f%% from %.1f%%",
			cur.DeterministicPassRate*100, base.DeterministicPassRate*100))
	}
	if cur.OverallMean < base.OverallMean-overallTolerance {
		out = append(out, fmt.Sprintf("overall mean fell to %.2f from %.2f (tolerance %.2f)",
			cur.OverallMean, base.OverallMean, overallTolerance))
	}

	var names []string
	for name := range base.MeanByDimension {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		baseMean := base.MeanByDimension[name]
		curMean, scored := cur.MeanByDimension[name]
		if !scored {
			out = append(out, fmt.Sprintf("%s was not scored at all; baseline scored it %.2f", name, baseMean))
			continue
		}
		if curMean < baseMean-judgeTolerance {
			out = append(out, fmt.Sprintf("%s fell to %.2f from %.2f (tolerance %.2f)",
				name, curMean, baseMean, judgeTolerance))
		}
	}
	return out
}
