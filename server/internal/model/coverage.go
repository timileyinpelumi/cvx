package model

import (
	"regexp"
	"strings"
)

// KeywordHit is one thing the posting named and whether the tailored resume
// says it back.
type KeywordHit struct {
	Keyword string `json:"keyword"`
	Covered bool   `json:"covered"`
	// Where names the first place it was found, for the panel to explain
	// itself: "skills", "summary", "headline", or "experience".
	Where string `json:"where,omitempty"`
}

// Coverage is the keyword panel: what the posting asked for by name, and
// which of those the resume actually contains. Purely a string match over
// what is already on the page, so it is free, instant, and cannot disagree
// with the PDF.
type Coverage struct {
	Hits    []KeywordHit `json:"hits"`
	Covered int          `json:"covered"`
	Total   int          `json:"total"`
}

// Missing returns the keywords the resume never says back, in posting order.
func (c Coverage) Missing() []string {
	var out []string
	for _, h := range c.Hits {
		if !h.Covered {
			out = append(out, h.Keyword)
		}
	}
	return out
}

// CoverageOf matches the posting's named keywords against the tailored
// resume. Matching is case-insensitive and punctuation-insensitive
// ("Node.js" matches "NodeJS", "CI/CD" matches "ci cd") but still
// token-bounded, so "Go" does not match "Google".
//
// Two passes do that: the spaced form catches keywords whose punctuation is
// a word break ("CI/CD" -> "ci cd"), and the squashed form, compared token
// by token, catches the ones whose punctuation is decorative ("Node.js" ->
// "nodejs"). Comparing squashed forms token-wise rather than by substring is
// what keeps "Go" out of "Google".
func CoverageOf(p Posting, t Tailored) Coverage {
	zones := []struct {
		name string
		text string
	}{
		{"skills", strings.Join(t.SelectedSkills, " ")},
		{"headline", t.Headline},
		{"summary", t.Summary},
		{"experience", experienceText(t)},
	}
	spaced := make([]string, len(zones))
	squashed := make([]map[string]bool, len(zones))
	for i, z := range zones {
		norm := normalizeForMatch(z.text)
		spaced[i] = " " + norm + " "
		squashed[i] = map[string]bool{}
		for _, tok := range strings.Fields(norm) {
			squashed[i][squash(tok)] = true
		}
	}

	cov := Coverage{Total: len(p.Keywords)}
	for _, kw := range p.Keywords {
		norm := normalizeForMatch(kw)
		needle, tight := " "+norm+" ", squash(norm)
		hit := KeywordHit{Keyword: kw}
		if norm != "" {
			for i := range zones {
				if strings.Contains(spaced[i], needle) || squashed[i][tight] {
					hit.Covered, hit.Where = true, zones[i].name
					cov.Covered++
					break
				}
			}
		}
		cov.Hits = append(cov.Hits, hit)
	}
	return cov
}

func experienceText(t Tailored) string {
	var b strings.Builder
	for _, s := range t.Sections {
		for _, it := range s.Items {
			b.WriteString(it.Title + " " + it.Organization + " ")
			for _, bu := range it.Bullets {
				b.WriteString(bu.Text + " ")
			}
		}
	}
	return b.String()
}

var matchNoise = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// squash removes every separator, so "node js" and "nodejs" become the same
// token. Only ever compared whole-token against whole-token.
func squash(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

// normalizeForMatch lowercases and collapses every run of punctuation to a
// single space, so the many spellings of one technology land on one string.
func normalizeForMatch(s string) string {
	return strings.TrimSpace(matchNoise.ReplaceAllString(strings.ToLower(s), " "))
}
