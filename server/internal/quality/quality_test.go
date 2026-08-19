package quality

import (
	"strings"
	"testing"
)

func TestMash(t *testing.T) {
	mash := []string{
		"",
		"...",
		"12345 67890",
		"a",
		"aaaaaaaa",
		"wgfwjhfkjhsdfkjh",
		"test test test test",
	}
	for _, s := range mash {
		if !Mash(s) {
			t.Errorf("Mash(%q) = false, want true", s)
		}
	}

	fine := []string{
		"Backend Engineer",
		"Go",
		"Senior iOS developer, fintech",
		"I led the migration to Kubernetes at Acme in 2023.",
		"We are hiring a backend engineer to build payment infrastructure with Go and Postgres.",
		"ewigiuwegf weiofhweihf",
	}
	for _, s := range fine {
		if Mash(s) {
			t.Errorf("Mash(%q) = true, want false", s)
		}
	}
}

func TestSkillShaped(t *testing.T) {
	good := []string{"Go", "SQL", "C++", "Node.js", "CI/CD", "Team leadership", "C#", ".NET"}
	for _, s := range good {
		if !SkillShaped(s) {
			t.Errorf("SkillShaped(%q) = false, want true", s)
		}
	}
	bad := []string{"", "x", "@@@@", "skill<script>", "asdf;drop", strings.Repeat("x", 61), "aaaaaaa", "12345"}
	for _, s := range bad {
		if SkillShaped(s) {
			t.Errorf("SkillShaped(%q) = true, want false", s)
		}
	}
}
