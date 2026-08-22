package ai

import (
	"context"
	"strings"
	"testing"

	"cvx/internal/model"
)

func TestAuditGroundingReportsUnsupportedClaims(t *testing.T) {
	f := &fakeLLM{out: `{"unsupported":[{"artifact":"application email","claim":"I cut latency by half at AE.","profileSays":"the profile gives no latency number"}]}`}
	found, err := AuditGrounding(context.Background(), f, digitizedSample(), map[string]string{
		ArtifactEmail: "I cut latency by half at AE.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Artifact != ArtifactEmail {
		t.Fatalf("got %+v", found)
	}
	if !strings.Contains(f.blocks[1].Text, "cut latency by half") {
		t.Fatalf("prose not sent for audit: %q", f.blocks[1].Text)
	}
}

// No prose means no call: the audit costs a request, and an empty one buys
// nothing.
func TestAuditGroundingSkipsEmptyInput(t *testing.T) {
	f := &fakeLLM{out: `{"unsupported":[]}`}
	found, err := AuditGrounding(context.Background(), f, model.Profile{}, map[string]string{
		ArtifactCoverLetter: "   ",
	})
	if err != nil || found != nil {
		t.Fatalf("got %v, %v", found, err)
	}
	if f.system != "" {
		t.Fatal("the audit called the model with nothing to audit")
	}
}

// Two artifacts always reach the prompt in the same order, so identical
// inputs produce an identical request and can be served from cache.
func TestAuditGroundingOrdersArtifactsStably(t *testing.T) {
	var seen string
	for i := 0; i < 5; i++ {
		f := &fakeLLM{out: `{"unsupported":[]}`}
		if _, err := AuditGrounding(context.Background(), f, model.Profile{}, map[string]string{
			ArtifactEmail:       "email body",
			ArtifactCoverLetter: "letter body",
		}); err != nil {
			t.Fatal(err)
		}
		if seen == "" {
			seen = f.blocks[1].Text
		} else if f.blocks[1].Text != seen {
			t.Fatal("artifact order changed between runs")
		}
	}
}
