package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// extendFailingLLM always errors, driving ExtendProfile's error-wrapping path.
type extendFailingLLM struct{}

func (extendFailingLLM) GenerateJSON(context.Context, string, []ContentBlock, map[string]any) ([]byte, error) {
	return nil, fmt.Errorf("boom")
}

func TestExtendProfileValid(t *testing.T) {
	p := digitizedSample() // item-0 / item-0-b-0, skills ["Python"]
	f := &fakeLLM{out: `{"useful":true,"notUsefulReason":"","newSkills":["Go"],"newItems":[],"bulletAdditions":[{"itemId":"item-0","bullets":[{"text":"Shipped v2","skills":["Go"]}]}]}`}

	a, err := ExtendProfile(context.Background(), f, p, "Shipped v2 of the engine using Go", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.NewSkills) != 1 || a.NewSkills[0] != "Go" {
		t.Fatalf("newSkills = %v", a.NewSkills)
	}
	if len(a.BulletAdditions) != 1 || a.BulletAdditions[0].ItemID != "item-0" {
		t.Fatalf("bulletAdditions = %+v", a.BulletAdditions)
	}
	if len(a.BulletAdditions[0].Bullets) != 1 || a.BulletAdditions[0].Bullets[0].Text != "Shipped v2" {
		t.Fatalf("bullets = %+v", a.BulletAdditions[0].Bullets)
	}
}

func TestExtendProfileNewItem(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: `{"useful":true,"notUsefulReason":"","newSkills":[],"newItems":[{"kind":"project","title":"Side project","organization":"","startDate":"2024-01","endDate":"","bullets":[{"text":"Built a CLI tool","skills":["Rust"]}]}],"bulletAdditions":[]}`}

	a, err := ExtendProfile(context.Background(), f, p, "I also built a CLI tool in Rust on the side", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.NewItems) != 1 || a.NewItems[0].Title != "Side project" {
		t.Fatalf("newItems = %+v", a.NewItems)
	}
	if len(a.NewItems[0].Bullets) != 1 || a.NewItems[0].Bullets[0].Text != "Built a CLI tool" {
		t.Fatalf("bullets = %+v", a.NewItems[0].Bullets)
	}
}

func TestExtendProfileCallShape(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: `{"useful":true,"notUsefulReason":"","newSkills":[],"newItems":[],"bulletAdditions":[]}`}

	if _, err := ExtendProfile(context.Background(), f, p, "shipped the v2 launch", ""); err != nil {
		t.Fatal(err)
	}

	if f.system == "" {
		t.Fatal("expected a system prompt")
	}
	for _, phrase := range []string{"ONLY facts", "invent", "exact id", "explicitly"} {
		if !strings.Contains(f.system, phrase) {
			t.Fatalf("system prompt missing %q:\n%s", phrase, f.system)
		}
	}

	if len(f.blocks) != 2 {
		t.Fatalf("want 2 blocks (profile JSON + note), got %d", len(f.blocks))
	}
	if !strings.Contains(f.blocks[0].Text, "Ada") {
		t.Fatalf("expected profile JSON in first block, got %q", f.blocks[0].Text)
	}
	if !strings.Contains(f.blocks[1].Text, "shipped the v2 launch") {
		t.Fatalf("expected note in second block, got %q", f.blocks[1].Text)
	}
}

func TestExtendProfileSchemaShape(t *testing.T) {
	req, ok := profileAdditionsSchema["required"].([]string)
	if !ok {
		t.Fatalf("required is not []string: %v", profileAdditionsSchema["required"])
	}
	want := []string{"useful", "notUsefulReason", "newSkills", "newCertifications",
		"newLanguages", "newInterests", "newItems", "bulletAdditions"}
	if len(req) != len(want) {
		t.Fatalf("required = %v, want %v", req, want)
	}
	for i, r := range want {
		if req[i] != r {
			t.Fatalf("required = %v, want %v", req, want)
		}
	}
	if profileAdditionsSchema["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %v, want false", profileAdditionsSchema["additionalProperties"])
	}
}

func TestExtendProfileLLMFailure(t *testing.T) {
	p := digitizedSample()
	if _, err := ExtendProfile(context.Background(), extendFailingLLM{}, p, "note", ""); err == nil {
		t.Fatal("want error")
	}
}

func TestExtendProfileGapContext(t *testing.T) {
	f := &fakeLLM{out: `{"useful":true,"notUsefulReason":"","newSkills":[],"newItems":[],"bulletAdditions":[]}`}
	p := digitizedSample()

	if _, err := ExtendProfile(context.Background(), f, p, "I used Django on one internal tool", "Django experience — not in profile"); err != nil {
		t.Fatal(err)
	}
	if len(f.blocks) != 3 || !strings.Contains(f.blocks[2].Text, "Django experience") {
		t.Fatalf("want gap context as third block, got %d blocks", len(f.blocks))
	}
	if !strings.Contains(f.system, "Gap context") {
		t.Fatal("system prompt must carry the targeting rule")
	}

	f2 := &fakeLLM{out: `{"useful":true,"notUsefulReason":"","newSkills":[],"newItems":[],"bulletAdditions":[]}`}
	if _, err := ExtendProfile(context.Background(), f2, p, "note", ""); err != nil {
		t.Fatal(err)
	}
	if len(f2.blocks) != 2 {
		t.Fatalf("empty context must add no block, got %d", len(f2.blocks))
	}
}
