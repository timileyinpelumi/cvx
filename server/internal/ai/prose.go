package ai

import (
	"fmt"
	"regexp"
	"strings"

	"cvx/internal/model"
)

// Prose QC: deterministic checks on the LLM's cover letter and recruiter
// email output, enforcing what the prompts promise. A draft that fails gets
// exactly one corrective rewrite (see the callers); this file only judges.

// Boilerplate the prompts already ban; matched case-insensitively.
var proseCliches = []string{
	"proven track record",
	"passionate about",
	"extensive experience",
	"strong communication skills",
}

var greetingShape = regexp.MustCompile(`^Dear .+,$`)

// checkProseText returns the shared per-text violations: em/en dashes,
// exclamation marks, placeholder brackets, and cliché phrases.
func checkProseText(label, s string) []string {
	var v []string
	if strings.ContainsAny(s, "—–") {
		v = append(v, label+" contains an em or en dash")
	}
	if strings.Contains(s, "!") {
		v = append(v, label+" contains an exclamation mark")
	}
	if strings.ContainsAny(s, "[]{}") {
		v = append(v, label+" contains placeholder brackets")
	}
	lower := strings.ToLower(s)
	for _, c := range proseCliches {
		if strings.Contains(lower, c) {
			v = append(v, fmt.Sprintf("%s contains the banned phrase %q", label, c))
		}
	}
	return v
}

func checkClosing(closing string) []string {
	var v []string
	if len(strings.Fields(closing)) > 5 {
		v = append(v, "closing is longer than a short sign-off")
	}
	if !strings.HasSuffix(strings.TrimSpace(closing), ",") {
		v = append(v, "closing does not end with a comma")
	}
	return v
}

func countSentences(s string) int {
	return strings.Count(s, ".") + strings.Count(s, "?")
}

func checkCoverLetter(cl model.CoverLetter) []string {
	var v []string

	if !greetingShape.MatchString(strings.TrimSpace(cl.Greeting)) {
		v = append(v, `greeting does not match the "Dear ...," shape`)
	}
	if n := len(cl.Paragraphs); n < 2 || n > 3 {
		v = append(v, fmt.Sprintf("letter has %d paragraphs, want 2 or 3", n))
	}

	total := 0
	seen := map[string]bool{}
	for i, p := range cl.Paragraphs {
		words := len(strings.Fields(p))
		total += words
		if words > 120 {
			v = append(v, fmt.Sprintf("paragraph %d is over 120 words", i+1))
		}
		key := strings.ToLower(strings.TrimSpace(p))
		if seen[key] {
			v = append(v, "a paragraph is repeated")
		}
		seen[key] = true
		v = append(v, checkProseText(fmt.Sprintf("paragraph %d", i+1), p)...)
	}
	if total < 60 || total > 220 {
		v = append(v, fmt.Sprintf("letter body is %d words, want 60 to 220", total))
	}

	v = append(v, checkClosing(cl.Closing)...)
	v = append(v, checkProseText("greeting", cl.Greeting)...)
	v = append(v, checkProseText("closing", cl.Closing)...)
	return v
}

func checkRecruiterEmail(re model.RecruiterEmail, candidateName string) []string {
	var v []string

	subject := strings.TrimSpace(re.Subject)
	if subject == "" {
		v = append(v, "subject is empty")
	}
	if len(subject) > 80 {
		v = append(v, "subject is over 80 characters")
	}
	if n := len(re.Paragraphs); n < 1 || n > 2 {
		v = append(v, fmt.Sprintf("email has %d paragraphs, want 1 or 2", n))
	}

	sentences := 0
	seen := map[string]bool{}
	for i, p := range re.Paragraphs {
		sentences += countSentences(p)
		key := strings.ToLower(strings.TrimSpace(p))
		if seen[key] {
			v = append(v, "a paragraph is repeated")
		}
		seen[key] = true
		v = append(v, checkProseText(fmt.Sprintf("paragraph %d", i+1), p)...)
	}
	if sentences < 3 || sentences > 6 {
		v = append(v, fmt.Sprintf("email has %d sentences, want 3 to 6", sentences))
	}

	v = append(v, checkClosing(re.Closing)...)
	if candidateName != "" && strings.Contains(strings.ToLower(re.Closing), strings.ToLower(candidateName)) {
		v = append(v, "closing contains the candidate's name; the sender appends it")
	}
	v = append(v, checkProseText("subject", re.Subject)...)
	v = append(v, checkProseText("closing", re.Closing)...)
	return v
}

// rewriteBlock turns a violation list into the corrective turn appended for
// the single retry.
func rewriteBlock(violations []string) ContentBlock {
	return ContentBlock{Text: "Your previous draft broke these rules:\n- " +
		strings.Join(violations, "\n- ") +
		"\nRewrite it following every rule in the instructions. Fix only what is listed; keep everything that already follows the rules."}
}
