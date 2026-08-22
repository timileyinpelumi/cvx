package model

import (
	"fmt"
	"strings"
)

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

// PageAdvice is what a page that ended early needs, phrased as something the
// user can act on. Empty when the page is full: a full page needs nothing
// said about it.
//
// Spacing can only stretch so far before a resume reads as scattered lines,
// so once the fit loop has used every item, bullet, certification, language
// and interest the profile holds, a short page is a content problem. This
// says so, and names the material that would fix it.
func PageAdvice(p Profile, fill float64, trimmed int) []string {
	if fill >= FullPage || trimmed > 0 {
		return nil
	}
	var out []string
	if len(p.Certifications) == 0 {
		out = append(out, "a certification or a course you finished")
	}
	if len(p.Languages) == 0 {
		out = append(out, "the languages you speak")
	}
	if len(p.Interests) == 0 {
		out = append(out, "an interest or a community you are part of, if it says something about how you work")
	}
	if !hasKind(p, "project") {
		out = append(out, "a side project, with two lines on what it does")
	}
	if !hasKind(p, "volunteering") {
		out = append(out, "volunteering, if you have done any")
	}
	if !hasKind(p, "education") {
		out = append(out, "your education")
	}
	if len(out) == 0 {
		out = append(out, "another two bullets on the work you have already listed")
	}
	return out
}

// FullPage is the fill a resume should reach before it stops looking short.
const FullPage = 0.90

func hasKind(p Profile, kind string) bool {
	for _, it := range p.Items {
		if strings.EqualFold(strings.TrimSpace(it.Kind), kind) {
			return true
		}
	}
	return false
}
