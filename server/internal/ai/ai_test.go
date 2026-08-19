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
const tailoredSample = `{"targetRole":"Python Backend Engineer","headline":"h","summary":"s","selectedSkills":["Python"],
	"sections":[{"title":"Experience","items":[{"sourceId":"item-0","title":"Engineer","organization":"AE","dates":"2021 – Present",
	"bullets":[{"sourceBulletId":"item-0-b-0","text":"Built the engine in Python"}]}]}],
	"gaps":[{"requirement":"Django","evidence":"not in profile","severity":"missing"}],"whatChanged":["led with Python"]}`

func TestTailorValid(t *testing.T) {
	p := digitizedSample() // helper reusing TestDigitize fixture
	f := &fakeLLM{out: tailoredSample}
	ta, err := Tailor(context.Background(), f, p, "Python Backend Engineer")
	if err != nil || ta.TargetRole != "Python Backend Engineer" {
		t.Fatalf("%+v %v", ta, err)
	}
}

func TestTailorCallShape(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: tailoredSample}

	if _, err := Tailor(context.Background(), f, p, "Python Backend Engineer"); err != nil {
		t.Fatal(err)
	}

	// The load-bearing instructions of the v2 prompt: the guardrail, the
	// selection/anti-force-fit rule, the anti-stuffing vocabulary rule, and
	// the verbatim-skills rule that backs the selectedSkillsSubset check.
	for _, phrase := range []string{
		"sourceBulletId",
		"at most 5 items",
		"2-4 bullets per item",
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

	if len(f.blocks) != 2 {
		t.Fatalf("want 2 blocks (profile JSON + role), got %d", len(f.blocks))
	}
}

func TestTailorRejectsFabrication(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: `{"targetRole":"X","headline":"h","summary":"s","selectedSkills":[],
		"sections":[{"title":"Experience","items":[{"sourceId":"item-7","title":"CTO","organization":"","dates":"",
		"bullets":[{"sourceBulletId":"item-7-b-0","text":"Ran everything"}]}]}],"gaps":[],"whatChanged":[]}`}
	if _, err := Tailor(context.Background(), f, p, "X"); err == nil || !strings.Contains(err.Error(), "item-7") {
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
	if _, err := TailorWithOptions(context.Background(), f, p, "role", TailorOptions{Tone: "confident"}); err != nil {
		t.Fatal(err)
	}
	if len(f.blocks) != 3 {
		t.Fatalf("want profile+role+adjustments blocks, got %d", len(f.blocks))
	}

	f2 := &fakeLLM{out: tailoredSample}
	if _, err := Tailor(context.Background(), f2, p, "role"); err != nil {
		t.Fatal(err)
	}
	if len(f2.blocks) != 2 {
		t.Fatalf("default Tailor must send exactly profile+role, got %d", len(f2.blocks))
	}
}
