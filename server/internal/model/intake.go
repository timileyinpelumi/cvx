package model

import (
	"fmt"
	"regexp"
	"strings"
)

// Question is one thing worth asking about a profile, with the reason
// attached so the UI can say why it is asking.
type Question struct {
	// ID is stable per subject, so answering the same gap twice does not
	// produce two questions.
	ID string `json:"id"`
	// Ask is the question itself, in the second person.
	Ask string `json:"ask"`
	// Why is the one-line reason, shown under the question.
	Why string `json:"why"`
	// ItemID is the profile item the answer belongs to, empty when the
	// question is about the profile as a whole.
	ItemID string `json:"itemId,omitempty"`
	// Placeholder is an example answer, which is what stops people writing
	// one word.
	Placeholder string `json:"placeholder,omitempty"`
}

// maxQuestions bounds the follow-up. Two to four questions get answered;
// ten get abandoned, and an abandoned intake is worse than no intake.
const maxQuestions = 4

// hasNumber is how a bullet is judged to carry evidence rather than a
// claim. Crude on purpose: any figure, percentage, or magnitude counts.
var hasNumber = regexp.MustCompile(`\d`)

// IntakeQuestions returns what to ask about a profile, most valuable first.
//
// Derived, not generated: the shape of the profile says exactly what is
// missing, so a pure function answers it for nothing and can never invent a
// question about a job the person does not have. An LLM here would cost a
// call and occasionally hallucinate the subject.
//
// The order is what a resume most needs and a person least volunteers:
// dates, then outcomes, then more material, then skills.
func IntakeQuestions(p Profile) []Question {
	var out []Question

	add := func(q Question) {
		if len(out) < maxQuestions {
			out = append(out, q)
		}
	}

	// Dates first: without them an item cannot be ordered or dated on the
	// page, and the guardrail forbids inventing one.
	for _, item := range p.Items {
		if len(out) >= maxQuestions {
			break
		}
		if strings.TrimSpace(item.StartDate) != "" {
			continue
		}
		add(Question{
			ID:          "dates:" + item.ID,
			ItemID:      item.ID,
			Ask:         fmt.Sprintf("When were you at %s?", subjectOf(item)),
			Why:         "Undated entries cannot be ordered on the page, and cvx will never guess a date.",
			Placeholder: "e.g. March 2023 to now, or 2021 to 2022",
		})
	}

	// Then outcomes: a bullet with no figure in it is a claim, and claims
	// are what make a resume read like everyone else's.
	for _, item := range p.Items {
		if len(out) >= maxQuestions {
			break
		}
		if item.Kind == "education" || len(item.Bullets) == 0 || itemHasNumber(item) {
			continue
		}
		add(Question{
			ID:          "outcome:" + item.ID,
			ItemID:      item.ID,
			Ask:         fmt.Sprintf("What changed because of your work at %s?", subjectOf(item)),
			Why:         "A number is the difference between evidence and a claim, and it has to come from you.",
			Placeholder: "e.g. cut checkout errors by about a third, or took the API from 2s to 300ms",
		})
	}

	// Then volume: three items is the floor a one-page resume is built on.
	if len(p.Items) < minResumeItems {
		add(Question{
			ID:          "more-items",
			Ask:         "What else have you worked on?",
			Why:         fmt.Sprintf("There %s here so far. A page needs at least %d to fill honestly.", countPhrase(len(p.Items)), minResumeItems),
			Placeholder: "A side project, a course, volunteering, anything you have actually done",
		})
	}

	// Then skills, which is the cheapest question to answer and the one
	// that most improves keyword coverage.
	if len(p.Skills) < minResumeSkills {
		add(Question{
			ID:          "skills",
			Ask:         "What do you actually work with?",
			Why:         "Job ads are matched on named tools, so the ones missing here cost you matches.",
			Placeholder: "e.g. Python, Postgres, Docker, Figma, whatever you use",
		})
	}

	// And education, which almost every resume carries and few people
	// mention unprompted.
	if !hasKind(p, "education") {
		add(Question{
			ID:          "education",
			Ask:         "Where did you study, and what?",
			Why:         "Most resumes carry it, and it fills a page that would otherwise end early.",
			Placeholder: "e.g. BSc Computer Engineering, FUTA, 2019",
		})
	}

	return out
}

func itemHasNumber(item Item) bool {
	for _, b := range item.Bullets {
		if hasNumber.MatchString(b.Text) {
			return true
		}
	}
	return false
}

// subjectOf names an item the way a person would refer to it.
func subjectOf(item Item) string {
	if org := strings.TrimSpace(item.Organization); org != "" {
		return org
	}
	if title := strings.TrimSpace(item.Title); title != "" {
		return title
	}
	return "that role"
}

func countPhrase(n int) string {
	switch n {
	case 0:
		return "is nothing"
	case 1:
		return "is one entry"
	default:
		return fmt.Sprintf("are %d entries", n)
	}
}
