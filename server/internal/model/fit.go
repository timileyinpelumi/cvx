package model

import "fmt"

// FitBand is the one-word verdict shown next to the score.
const (
	FitStrong  = "strong"
	FitFair    = "fair"
	FitStretch = "stretch"
)

// Fit is the "should I even send this" answer: one score out of 100, the
// band it falls in, and the reasons that moved it. Computed from what the
// pipeline already produced (the gaps the tailor reported and the keyword
// coverage of the finished resume), so it costs nothing and can never
// contradict the page.
type Fit struct {
	Score   int      `json:"score"`
	Band    string   `json:"band"`
	Reasons []string `json:"reasons"`
}

// Fit weights: a missing requirement costs more than a weak one, and
// unmatched keywords cost a little each. The scale is deliberately blunt —
// this is a "worth applying?" signal, not a ranking.
const (
	fitMissingPenalty = 12
	fitWeakPenalty    = 5
	fitKeywordWeight  = 25
)

// FitOf scores the tailored resume against the posting.
func FitOf(p Posting, t Tailored, cov Coverage) Fit {
	score := 100
	missing, weak := 0, 0
	for _, g := range t.Gaps {
		switch g.Severity {
		case "missing":
			missing++
			score -= fitMissingPenalty
		case "weak":
			weak++
			score -= fitWeakPenalty
		}
	}

	// Keyword coverage moves the score proportionally: a resume that says
	// back none of what the posting named loses the full weight.
	if cov.Total > 0 {
		score -= int(float64(fitKeywordWeight) * (1 - float64(cov.Covered)/float64(cov.Total)))
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	f := Fit{Score: score, Band: fitBand(score)}
	if missing > 0 {
		f.Reasons = append(f.Reasons, fmt.Sprintf("%s missing from your profile",
			plural(missing, "requirement is", "requirements are")))
	}
	if weak > 0 {
		f.Reasons = append(f.Reasons, fmt.Sprintf("%s covered but thinly",
			plural(weak, "requirement is", "requirements are")))
	}
	if cov.Total > 0 {
		f.Reasons = append(f.Reasons, fmt.Sprintf("your resume says back %d of the %d technologies the posting names",
			cov.Covered, cov.Total))
	}
	if len(f.Reasons) == 0 {
		f.Reasons = append(f.Reasons, "nothing the posting asks for is missing from your profile")
	}
	return f
}

func fitBand(score int) string {
	switch {
	case score >= 75:
		return FitStrong
	case score >= 50:
		return FitFair
	default:
		return FitStretch
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
