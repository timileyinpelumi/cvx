package ai

import (
	"context"
	"strings"
	"testing"

	"cvx/internal/model"
)

func rewriteFixture() (model.Bullet, model.Posting) {
	return model.Bullet{ID: "item-0-b-0", Text: "Built the payment service in Go"},
		model.Posting{Usable: true, Title: "Backend Engineer", Raw: "Backend Engineer"}
}

func TestRewriteBulletReturnsTheNewLine(t *testing.T) {
	f := &fakeLLM{out: `{"text":"Built the payment service in Go and owned it in production"}`}
	source, posting := rewriteFixture()

	got, err := RewriteBullet(context.Background(), f, source, "Engineer", "AE", posting, "Did payments", "make it concrete")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Built the payment service in Go and owned it in production" {
		t.Fatalf("got %q", got)
	}
	joined := ""
	for _, b := range f.blocks {
		joined += b.Text + "\n"
	}
	for _, want := range []string{"Built the payment service in Go", "Engineer at AE", "Backend Engineer", "make it concrete"} {
		if !strings.Contains(joined, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// The rewrite is held to the same prose standard as everything else, so a
// bad line fails loudly instead of landing on the page.
func TestRewriteBulletRejectsBadOutput(t *testing.T) {
	source, posting := rewriteFixture()
	cases := map[string]string{
		"empty":            `{"text":"   "}`,
		"em dash":          `{"text":"Built the payment service — in Go"}`,
		"placeholder":      `{"text":"Built the [SYSTEM] in Go"}`,
		"exclamation":      `{"text":"Built the payment service in Go!"}`,
		"runs to an essay": `{"text":"` + strings.Repeat("word ", 40) + `"}`,
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeLLM{out: out}
			if _, err := RewriteBullet(context.Background(), f, source, "Engineer", "AE", posting, "Did payments", ""); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// No instruction means no instruction block: the prompt should not carry an
// empty "what the candidate asked for" section.
func TestRewriteBulletOmitsEmptyInstruction(t *testing.T) {
	f := &fakeLLM{out: `{"text":"Built the payment service in Go"}`}
	source, posting := rewriteFixture()
	if _, err := RewriteBullet(context.Background(), f, source, "Engineer", "AE", posting, "Did payments", "  "); err != nil {
		t.Fatal(err)
	}
	for _, b := range f.blocks {
		if strings.Contains(b.Text, "candidate asked for") {
			t.Fatal("empty instruction was sent anyway")
		}
	}
}
