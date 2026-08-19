package ai

import (
	"context"
	"strings"
	"testing"
)

func TestRecruiterEmail(t *testing.T) {
	f := &fakeLLM{out: `{"subject":"Application for Backend Engineer","paragraphs":["I am applying for the Backend Engineer role. At Analytical Engines Co I built the core analytical engine in Go and wrote its first published algorithm. My resume and the details are attached."],"closing":"Best regards,"}`}
	p := digitizedSample()

	re, err := RecruiterEmail(context.Background(), f, p, "Backend Engineer")
	if err != nil {
		t.Fatal(err)
	}
	if re.Subject != "Application for Backend Engineer" || len(re.Paragraphs) != 1 || re.Closing != "Best regards," {
		t.Fatalf("got %+v", re)
	}
	if len(f.blocks) != 2 || !strings.Contains(f.blocks[0].Text, "Profile JSON") || !strings.Contains(f.blocks[1].Text, "Backend Engineer") {
		t.Fatalf("blocks wrong: %+v", f.blocks)
	}
	if !strings.Contains(f.system, "email, not a cover letter") {
		t.Fatal("system prompt missing")
	}
}
