package ai

import (
	"context"
	"strings"
	"testing"

	"cvx/internal/model"
)

func TestRecruiterEmail(t *testing.T) {
	f := &fakeLLM{out: `{"subject":"Application for Backend Engineer","paragraphs":["I am applying for the Backend Engineer role. At Analytical Engines Co I built the core analytical engine in Go and wrote its first published algorithm. My resume and the details are attached."],"closing":"Best regards,"}`}
	p := digitizedSample()

	re, err := RecruiterEmail(context.Background(), f, p, testPosting("Backend Engineer"))
	if err != nil {
		t.Fatal(err)
	}
	if re.Subject != "Application for Backend Engineer" || len(re.Paragraphs) != 1 || re.Closing != "Best regards," {
		t.Fatalf("got %+v", re)
	}
	if len(f.blocks) != 4 || !strings.Contains(f.blocks[0].Text, "Profile JSON") ||
		!strings.Contains(f.blocks[1].Text, "Backend Engineer") ||
		!strings.Contains(f.blocks[3].Text, "Angle for this draft") {
		t.Fatalf("blocks wrong: %+v", f.blocks)
	}
	if !strings.Contains(f.system, "email, not a cover letter") {
		t.Fatal("system prompt missing")
	}
}

// The email has to say what it is before it says anything clever, and it has
// to point at what is attached. Both are prompt-level contracts, so this
// asserts the prompt still carries them.
func TestRecruiterEmailPromptRequiresIntroAndAttachments(t *testing.T) {
	f := &fakeLLM{out: `{"subject":"Application for Backend Engineer","recruiterName":"","company":"","tone":"neutral","paragraphs":["I am applying for the Backend Engineer role. At Analytical Engines Co I built the core analytical engine in Go and wrote its first published algorithm. My resume is attached."],"closing":"Best regards,"}`}
	if _, err := RecruiterEmail(context.Background(), f, digitizedSample(), testPosting("Backend Engineer")); err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		"FIRST sentence of paragraph 1",
		"LAST sentence of the email points at what is attached",
		"greeting is composed for you from the posting",
	} {
		if !strings.Contains(f.system, phrase) {
			t.Fatalf("system prompt missing %q", phrase)
		}
	}
}

// The greeting is composed from the parsed posting, so the same posting
// greets the same person the same way in the email and the letter.
func TestRecruiterEmailComposesGreetingFromPosting(t *testing.T) {
	f := &fakeLLM{out: `{"subject":"Application for Backend Engineer","paragraphs":["I am applying for the Backend Engineer role at Venix. At Analytical Engines Co I built the core analytical engine in Go and wrote its first published algorithm. My resume is attached."],"closing":"Best regards,"}`}
	posting := model.Posting{
		Usable: true, Title: "Backend Engineer", Company: "Venix Inc",
		ContactName: "Ms. Jane Doe", Tone: model.ToneFormal, Raw: "Backend Engineer at Venix",
	}
	re, err := RecruiterEmail(context.Background(), f, digitizedSample(), posting)
	if err != nil {
		t.Fatal(err)
	}
	if re.Greeting != "Dear Jane," {
		t.Fatalf("want the named contact and the posting's register, got %q", re.Greeting)
	}
}
