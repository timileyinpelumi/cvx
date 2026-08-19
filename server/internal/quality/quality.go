// Package quality holds the free, instant input checks that run before any
// LLM call: they reject obvious keyboard mash so the model only ever judges
// text that could plausibly mean something.
package quality

import (
	"strings"
	"unicode"
)

// SkillShaped reports whether s could plausibly be a skill name: 2 to 60
// characters, at least one letter, only characters that appear in real skill
// names (letters, digits, spaces, and the punctuation of names like C++,
// Node.js, CI/CD), and not keyboard mash.
func SkillShaped(s string) bool {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) < 2 || len(runes) > 60 {
		return false
	}
	hasLetter := false
	for _, r := range runes {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r) || r == ' ':
		case strings.ContainsRune("+#./&()'-", r):
		default:
			return false
		}
	}
	if !hasLetter {
		return false
	}
	// Short names like Go, C++, C#, SQL are legitimate below Mash's radar.
	return len(runes) <= 4 || !Mash(s)
}

// Mash reports whether s is obviously meaningless: too few letters to carry
// any information, long repeated-character runs, consonant-only mashing, or
// one token repeated over and over. It is deliberately lenient — fluent-looking
// nonsense passes and is left to the LLM classifier.
func Mash(s string) bool {
	s = strings.TrimSpace(s)

	letters, vowels := 0, 0
	maxRun, run := 1, 1
	var prev rune
	for i, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if strings.ContainsRune("aeiouyAEIOUY", r) {
				vowels++
			}
		}
		if i > 0 && r == prev {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 1
		}
		prev = r
	}

	if letters < 2 {
		return true
	}
	if maxRun >= 5 {
		return true
	}
	if letters >= 10 && vowels*100 < letters*12 {
		return true
	}

	fields := strings.Fields(strings.ToLower(s))
	if len(fields) >= 3 {
		same := true
		for _, f := range fields[1:] {
			if f != fields[0] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}

	return false
}
