package model

import "testing"

func TestCoverageMatchesAcrossSpellings(t *testing.T) {
	p := Posting{Keywords: []string{"Go", "Node.js", "CI/CD", "Kubernetes", "PostgreSQL"}}
	tl := Tailored{
		SelectedSkills: []string{"Golang", "NodeJS", "PostgreSQL"},
		Headline:       "Backend engineer",
		Summary:        "Builds services with ci cd pipelines behind them.",
		Sections: []TSection{{Items: []TItem{{
			Bullets: []TBullet{{Text: "Wrote the payment service in Go and shipped it weekly."}},
		}}}},
	}

	cov := CoverageOf(p, tl)
	byKeyword := map[string]KeywordHit{}
	for _, h := range cov.Hits {
		byKeyword[h.Keyword] = h
	}

	if !byKeyword["Go"].Covered || byKeyword["Go"].Where != "experience" {
		t.Errorf("Go should match the bullet, got %+v", byKeyword["Go"])
	}
	if !byKeyword["Node.js"].Covered {
		t.Error("Node.js should match NodeJS")
	}
	if !byKeyword["CI/CD"].Covered {
		t.Error("CI/CD should match 'ci cd'")
	}
	if byKeyword["Kubernetes"].Covered {
		t.Error("Kubernetes appears nowhere and must not be covered")
	}
	if cov.Covered != 4 || cov.Total != 5 {
		t.Fatalf("want 4 of 5 covered, got %d of %d", cov.Covered, cov.Total)
	}
	if len(cov.Missing()) != 1 || cov.Missing()[0] != "Kubernetes" {
		t.Fatalf("missing list wrong: %v", cov.Missing())
	}
}

// "Go" must not match "Google": the match is token-bounded, not substring.
func TestCoverageDoesNotMatchInsideWords(t *testing.T) {
	cov := CoverageOf(
		Posting{Keywords: []string{"Go", "R"}},
		Tailored{Summary: "Worked at Google on reporting."},
	)
	if cov.Covered != 0 {
		t.Fatalf("substring match leaked: %+v", cov.Hits)
	}
}

func TestFitBandsAndReasons(t *testing.T) {
	full := Coverage{Covered: 5, Total: 5}
	clean := FitOf(Posting{}, Tailored{}, full)
	if clean.Score != 100 || clean.Band != FitStrong {
		t.Fatalf("a clean fit should be 100/strong, got %+v", clean)
	}

	gappy := FitOf(Posting{}, Tailored{Gaps: []Gap{
		{Severity: "missing"}, {Severity: "missing"}, {Severity: "weak"},
	}}, Coverage{Covered: 2, Total: 8})
	if gappy.Band == FitStrong {
		t.Fatalf("two missing requirements and 2/8 keywords should not be strong: %+v", gappy)
	}
	if len(gappy.Reasons) != 3 {
		t.Fatalf("want a reason per signal, got %v", gappy.Reasons)
	}

	if floor := FitOf(Posting{}, Tailored{Gaps: manyGaps(20)}, Coverage{Total: 5}); floor.Score != 0 {
		t.Fatalf("score must floor at 0, got %d", floor.Score)
	}
}

func manyGaps(n int) []Gap {
	out := make([]Gap, n)
	for i := range out {
		out[i] = Gap{Severity: "missing"}
	}
	return out
}
