package model

import (
	"regexp"
	"strings"
)

// Greeting tones. The posting sets the register: a public-sector or agency
// ad that opens "Dear Applicant" gets "Dear"; a startup ad written in the
// second person gets "Hi". Guessing wrong in either direction is the kind of
// thing a recruiter notices, so the model reads the tone off the posting and
// this file decides the words.
const (
	ToneFormal  = "formal"
	ToneNeutral = "neutral"
	ToneCasual  = "casual"
)

// GreetingInput is everything the greeting is composed from: whatever the
// posting gave up about who is reading, and how formally it was written.
type GreetingInput struct {
	RecruiterName string
	Company       string
	Tone          string
	// Letter narrows the openers to the ones a printed cover letter can
	// carry: "Hi Jane," is fine in an email and wrong on a letter.
	Letter bool
}

// openers per tone, in the order a variant index walks them.
var openers = map[string][]string{
	ToneFormal:  {"Dear"},
	ToneNeutral: {"Hello", "Dear"},
	ToneCasual:  {"Hi", "Hello"},
}

// Greeting composes the line the email or letter opens with. variant picks
// between the openers a tone allows, so two applications to two postings of
// the same tone do not open identically; it is taken modulo the number of
// choices, so any int is safe.
func Greeting(in GreetingInput, variant int) string {
	return greetingOpener(in, variant) + " " + greetingAddressee(in) + ","
}

func greetingOpener(in GreetingInput, variant int) string {
	if in.Letter {
		return "Dear"
	}
	choices, ok := openers[strings.ToLower(strings.TrimSpace(in.Tone))]
	if !ok {
		choices = openers[ToneNeutral]
	}
	if variant < 0 {
		variant = -variant
	}
	return choices[variant%len(choices)]
}

// greetingAddressee answers "who is this to", in descending order of what
// the posting actually told us: a named person, the company's hiring team,
// or HR.
func greetingAddressee(in GreetingInput) string {
	if first := firstName(in.RecruiterName); first != "" {
		return first
	}
	if c := companyName(in.Company); c != "" {
		return c + " hiring team"
	}
	if in.Letter {
		return "hiring team"
	}
	return "HR"
}

// RecruiterGreeting is the neutral, no-data greeting: what an email opens
// with when the posting named nobody and nothing.
func RecruiterGreeting(recruiterName string) string {
	return Greeting(GreetingInput{RecruiterName: recruiterName}, 0)
}

var namePart = regexp.MustCompile(`^[\p{L}][\p{L}'-]*$`)

// nonNames are words a posting uses for whoever is reading; none of them is
// a first name, and greeting one by "name" reads as a mail merge.
var nonNames = map[string]bool{
	"hr": true, "the": true, "hiring": true, "team": true, "recruiter": true,
	"recruitment": true, "talent": true, "people": true, "manager": true,
	"admin": true, "sir": true, "madam": true, "applicant": true,
	"candidate": true, "company": true, "department": true,
}

// firstName pulls a usable first name out of whatever the posting called the
// contact, dropping honorifics and rejecting anything that is not a name
// ("the hiring team", "HR", an email address, a placeholder).
func firstName(s string) string {
	for _, f := range strings.Fields(strings.TrimSpace(s)) {
		f = strings.Trim(f, ".,")
		switch strings.ToLower(f) {
		case "mr", "mrs", "ms", "miss", "dr", "prof", "engr":
			continue
		}
		if !namePart.MatchString(f) || len(f) < 2 || len(f) > 20 || nonNames[strings.ToLower(f)] {
			return ""
		}
		return strings.ToUpper(f[:1]) + f[1:]
	}
	return ""
}

// companyName accepts a company only when the posting named a real one:
// placeholders, legal suffixes on their own, and anything long enough to be
// a sentence are dropped rather than pasted into the first line.
func companyName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, "[]{}<>@") {
		return ""
	}
	fields := strings.Fields(s)
	if len(fields) == 0 || len(fields) > 4 || len(s) > 40 {
		return ""
	}
	if nonNames[strings.ToLower(fields[0])] {
		return ""
	}
	// Trailing legal suffixes read as boilerplate in a greeting.
	last := strings.ToLower(strings.Trim(fields[len(fields)-1], ".,"))
	switch last {
	case "inc", "llc", "ltd", "limited", "gmbh", "plc", "co", "corp", "corporation":
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}
