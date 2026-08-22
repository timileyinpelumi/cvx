package ai

import (
	"context"
	"strings"
	"testing"

	"cvx/internal/model"
)

type fakeLLM struct {
	out    string
	system string
	blocks []ContentBlock
}

func (f *fakeLLM) GenerateJSON(_ context.Context, system string, blocks []ContentBlock, _ map[string]any) ([]byte, error) {
	f.system, f.blocks = system, blocks
	return []byte(f.out), nil
}

func digitizedSample() model.Profile {
	f := &fakeLLM{out: `{"isResume":true,"notResumeReason":"","name":"Ada","email":"a@e.com","phone":"","location":"","summary":"","links":[],"skills":["Python"],
		"items":[{"kind":"experience","title":"Engineer","organization":"AE","startDate":"2021-01","endDate":"","bullets":[{"text":"Built engine","skills":["Python"]}]}]}`}
	p, err := Digitize(context.Background(), f, []byte("%PDF"))
	if err != nil {
		panic(err)
	}
	return p
}

func TestDigitize(t *testing.T) {
	f := &fakeLLM{out: `{"isResume":true,"notResumeReason":"","name":"Ada","email":"a@e.com","phone":"","location":"","summary":"","links":[],"skills":["Python"],
		"items":[{"kind":"experience","title":"Engineer","organization":"AE","startDate":"2021-01","endDate":"","bullets":[{"text":"Built engine","skills":["Python"]}]}]}`}
	p, err := Digitize(context.Background(), f, []byte("%PDF"))
	if err != nil || p.Items[0].ID != "item-0" || p.Items[0].Bullets[0].ID != "item-0-b-0" {
		t.Fatalf("%+v %v", p, err)
	}
	if f.blocks[0].PDF == nil {
		t.Fatal("expected PDF block sent to LLM")
	}
}

// tailoredSample is a minimal valid Tailor response for digitizedSample's
// profile.
const tailoredSample = `{"targetRole":"Python Backend Engineer","headline":"h","summary":"Backend engineer who builds the services other teams depend on, most of it in Python. Built the computation engine at AE and owned its correctness under load, from the service layer down to the batch jobs that carried production traffic. That is the same ground this role covers, and the reason the fit is close enough to be worth a conversation.","selectedSkills":["Python"],
	"sections":[{"title":"Experience","items":[{"sourceId":"item-0","title":"Engineer","organization":"AE","dates":"2021 – Present",
	"bullets":[{"sourceBulletId":"item-0-b-0","text":"Built the engine in Python"}]}]}],
	"gaps":[{"requirement":"Django","evidence":"not in profile","severity":"missing"}],"whatChanged":["led with Python"]}`

func TestTailorValid(t *testing.T) {
	p := digitizedSample() // helper reusing TestDigitize fixture
	f := &fakeLLM{out: tailoredSample}
	ta, err := Tailor(context.Background(), f, p, testPosting("Python Backend Engineer"))
	if err != nil || ta.TargetRole != "Python Backend Engineer" {
		t.Fatalf("%+v %v", ta, err)
	}
}

func TestTailorCallShape(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: tailoredSample}

	if _, err := Tailor(context.Background(), f, p, testPosting("Python Backend Engineer")); err != nil {
		t.Fatal(err)
	}

	// The load-bearing instructions of the v2 prompt: the guardrail, the
	// selection/anti-force-fit rule, the anti-stuffing vocabulary rule, and
	// the verbatim-skills rule that backs the selectedSkillsSubset check.
	for _, phrase := range []string{
		"sourceBulletId",
		"Select 3 to 5 items",
		"2 to 4 bullets per item",
		"Never force-fit",
		"Anti-stuffing",
		"copied verbatim from the",
		`"skills" array`,
		"whatChanged",
	} {
		if !strings.Contains(f.system, phrase) {
			t.Fatalf("system prompt missing %q:\n%s", phrase, f.system)
		}
	}

	if len(f.blocks) != 3 {
		t.Fatalf("want 3 blocks (profile JSON + structured posting + raw posting), got %d", len(f.blocks))
	}
	if !strings.Contains(f.blocks[1].Text, "already read and structured") {
		t.Fatalf("want the parsed posting in block 2, got %q", f.blocks[1].Text)
	}
}

func TestTailorRejectsFabrication(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: `{"targetRole":"X","headline":"h","summary":"Backend engineer who builds the services other teams depend on, most of it in Python. Built the computation engine at AE and owned its correctness under load, from the service layer down to the batch jobs that carried production traffic. That is the same ground this role covers, and the reason the fit is close enough to be worth a conversation.","selectedSkills":[],
		"sections":[{"title":"Experience","items":[{"sourceId":"item-7","title":"CTO","organization":"","dates":"",
		"bullets":[{"sourceBulletId":"item-7-b-0","text":"Ran everything"}]}]}],"gaps":[],"whatChanged":[]}`}
	if _, err := Tailor(context.Background(), f, p, testPosting("X")); err == nil || !strings.Contains(err.Error(), "item-7") {
		t.Fatalf("want fabrication error, got %v", err)
	}
}

func TestTailorOptionsInstructions(t *testing.T) {
	if got := (TailorOptions{}).instructions(); got != "" {
		t.Fatalf("zero options must add nothing, got %q", got)
	}
	if got := (TailorOptions{Tone: "plain", Summary: "standard", Bullets: "full"}).instructions(); got != "" {
		t.Fatalf("named defaults must add nothing, got %q", got)
	}
	got := TailorOptions{Tone: "confident", Summary: "none", Bullets: "lean"}.instructions()
	for _, want := range []string{"assertive", "empty string", "2 or 3 bullets"} {
		if !strings.Contains(got, want) {
			t.Fatalf("instructions missing %q: %q", want, got)
		}
	}
}

func TestTailorWithOptionsAppendsBlock(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: tailoredSample}
	if _, err := TailorWithOptions(context.Background(), f, p, testPosting("role"), TailorOptions{Tone: "confident"}); err != nil {
		t.Fatal(err)
	}
	if len(f.blocks) != 4 {
		t.Fatalf("want profile + structured posting + raw posting + adjustments blocks, got %d", len(f.blocks))
	}

	f2 := &fakeLLM{out: tailoredSample}
	if _, err := Tailor(context.Background(), f2, p, testPosting("role")); err != nil {
		t.Fatal(err)
	}
	if len(f2.blocks) != 3 {
		t.Fatalf("default Tailor must send exactly profile + structured posting + raw posting, got %d", len(f2.blocks))
	}
}

// testPosting is the fixture stand-in for a parsed posting: usable, titled,
// and carrying the same text as its raw body.
func testPosting(text string) model.Posting {
	return model.Posting{Usable: true, Title: text, Raw: text}
}
