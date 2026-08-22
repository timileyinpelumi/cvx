package model

import "strings"

// Seniority levels a posting can ask for. "unknown" is a real answer: plenty
// of postings never say.
const (
	SeniorityIntern  = "intern"
	SeniorityJunior  = "junior"
	SeniorityMid     = "mid"
	SenioritySenior  = "senior"
	SeniorityLead    = "lead"
	SeniorityUnknown = "unknown"
)

// Posting is the job ad, read once and structured. Everything downstream —
// the resume, the cover letter, the application email, the greeting, the
// subject line, the keyword coverage panel — reads this instead of
// re-extracting from the raw text, so the artifacts cannot disagree with
// each other about who is hiring or what for.
type Posting struct {
	// Usable is the old ClassifyJobInput verdict: whether this text is a job
	// ad, a fetched job page, or a role description at all.
	Usable bool   `json:"usable"`
	Reason string `json:"reason"`

	Title       string `json:"title"`
	Company     string `json:"company"`
	ContactName string `json:"contactName"`
	Location    string `json:"location"`
	Seniority   string `json:"seniority"`
	Tone        string `json:"tone"`

	// MustHaves and NiceToHaves are the posting's requirements in its own
	// words, separated because a gap against a must-have matters more.
	MustHaves   []string `json:"mustHaves"`
	NiceToHaves []string `json:"niceToHaves"`

	// Keywords are the concrete named things the posting asks for:
	// technologies, tools, platforms, certifications. Deliberately not
	// competences ("communication"), which cannot be matched by string.
	Keywords []string `json:"keywords"`

	// Raw is the posting text as the pipeline received it, kept so prompts
	// can still read what no schema captured.
	Raw string `json:"raw"`
}

// RoleLabel is what to call this posting in prose and filenames: its title
// when it has one, falling back to the raw text for the "Backend Engineer"
// one-liner case.
func (p Posting) RoleLabel() string {
	if t := strings.TrimSpace(p.Title); t != "" {
		return t
	}
	return strings.TrimSpace(firstLine(p.Raw))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// GreetingInputFrom builds the greeting inputs from the posting, so the
// email and the letter greet the same person the same way.
func (p Posting) GreetingInputFrom(letter bool) GreetingInput {
	return GreetingInput{
		RecruiterName: p.ContactName,
		Company:       p.Company,
		Tone:          p.Tone,
		Letter:        letter,
	}
}
