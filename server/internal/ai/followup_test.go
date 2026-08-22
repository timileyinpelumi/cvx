package ai

import (
	"context"
	"strings"
	"testing"

	"cvx/internal/model"
)

const validFollowUpJSON = `{"subject":"Following up on my application for Backend Engineer",
	"paragraphs":["I applied for the Backend Engineer role a couple of weeks ago and wanted to check in. Since then I have kept the analytical engine work going in Python, which is the closest thing in my background to what this role covers. Is there an update, or anything else that would be useful from me?"],
	"closing":"Best regards,"}`

func TestFollowUpComposesGreetingAndCarriesTheWait(t *testing.T) {
	f := &fakeLLM{out: validFollowUpJSON}
	posting := model.Posting{Usable: true, Title: "Backend Engineer", ContactName: "Jane Doe", Raw: "Backend Engineer"}

	re, err := FollowUp(context.Background(), f, digitizedSample(), posting, 14)
	if err != nil {
		t.Fatal(err)
	}
	// The opener varies per draft; the addressee comes from the posting and
	// must not.
	if !strings.HasSuffix(re.Greeting, " Jane,") {
		t.Fatalf("greeting not composed from the posting: %q", re.Greeting)
	}
	joined := ""
	for _, b := range f.blocks {
		joined += b.Text + "\n"
	}
	if !strings.Contains(joined, "sent 14 days ago") {
		t.Fatalf("the wait was not passed to the model:\n%s", joined)
	}
	if !strings.Contains(f.system, "nudge, not a second application") {
		t.Fatal("follow-up prompt missing")
	}
}

// A nudge that runs to three paragraphs is not a nudge.
func TestFollowUpRejectsAnEssay(t *testing.T) {
	long := `{"subject":"Following up on my application for Backend Engineer",
		"paragraphs":["One. Two.","Three. Four."],"closing":"Best regards,"}`
	f := &fakeLLM{out: long}
	_, err := FollowUp(context.Background(), f, digitizedSample(), model.Posting{Raw: "role"}, 10)
	if err == nil || !strings.Contains(err.Error(), "want exactly 1") {
		t.Fatalf("want the paragraph-count violation, got %v", err)
	}
}

func TestCheckFollowUpAcceptsAShortNudge(t *testing.T) {
	re := model.RecruiterEmail{
		Subject:    "Following up on my application for Backend Engineer",
		Greeting:   "Hello HR,",
		Paragraphs: []string{"I applied two weeks ago. Is there an update?"},
		Closing:    "Best regards,",
	}
	if v := checkFollowUp(re, "Ada Lovelace"); len(v) != 0 {
		t.Fatalf("a clean nudge was flagged: %v", v)
	}
}
