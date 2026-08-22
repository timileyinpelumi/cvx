package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cvx/internal/model"
)

const parsedPostingJSON = `{"usable":true,"reason":"job ad","title":"Backend Engineer","company":"Venix Inc",
	"contactName":"Jane Doe","location":"Remote","seniority":"senior","tone":"formal",
	"mustHaves":["Go","payment systems"],"niceToHaves":["Kubernetes"],
	"keywords":["Go","PostgreSQL","Docker","go"]}`

func TestParsePostingStructuresTheAd(t *testing.T) {
	f := &fakeLLM{out: parsedPostingJSON}
	p, err := ParsePosting(context.Background(), f, "Backend Engineer at Venix")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Backend Engineer" || p.Company != "Venix Inc" || p.ContactName != "Jane Doe" {
		t.Fatalf("got %+v", p)
	}
	if p.Raw != "Backend Engineer at Venix" {
		t.Fatalf("raw text not kept: %q", p.Raw)
	}
	// "Go" and "go" are the same keyword.
	if len(p.Keywords) != 3 {
		t.Fatalf("keywords not deduped: %v", p.Keywords)
	}
}

// The parse subsumes the old usability gate, so an unusable input still comes
// back as the same error the handler maps to 422.
func TestParsePostingRejectsUnusableInput(t *testing.T) {
	f := &fakeLLM{out: `{"usable":false,"reason":"this is a receipt","title":"","company":"","contactName":"","location":"","seniority":"unknown","tone":"neutral","mustHaves":[],"niceToHaves":[],"keywords":[]}`}
	_, err := ParsePosting(context.Background(), f, "TOTAL: $14.20")
	if !errors.Is(err, ErrNotJobInput) {
		t.Fatalf("want ErrNotJobInput, got %v", err)
	}
}

func TestParsePostingNormalizesEnums(t *testing.T) {
	f := &fakeLLM{out: `{"usable":true,"reason":"ok","title":" Engineer ","company":"","contactName":"","location":"","seniority":"wizard","tone":"shouty","mustHaves":[],"niceToHaves":[],"keywords":[]}`}
	p, err := ParsePosting(context.Background(), f, "Engineer")
	if err != nil {
		t.Fatal(err)
	}
	if p.Seniority != model.SeniorityUnknown || p.Tone != model.ToneNeutral || p.Title != "Engineer" {
		t.Fatalf("not normalized: %+v", p)
	}
}

// Every generator reads the same structured block, which is what stops the
// resume, the letter, and the email disagreeing about who is hiring.
func TestPostingBlocksCarryTheStructuredRead(t *testing.T) {
	blocks := postingBlocks(model.Posting{
		Usable: true, Title: "Backend Engineer", Company: "Venix",
		MustHaves: []string{"Go"}, Keywords: []string{"Go", "Docker"}, Raw: "the full ad",
	})
	if len(blocks) != 2 {
		t.Fatalf("want a structured block and a raw block, got %d", len(blocks))
	}
	for _, want := range []string{"Backend Engineer", "Venix", "Go", "Docker"} {
		if !strings.Contains(blocks[0].Text, want) {
			t.Errorf("structured block missing %q", want)
		}
	}
	if !strings.Contains(blocks[1].Text, "the full ad") {
		t.Error("raw posting not passed through")
	}
}
