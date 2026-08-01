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
	f := &fakeLLM{out: `{"name":"Ada","email":"a@e.com","phone":"","location":"","summary":"","links":[],"skills":["Python"],
		"items":[{"kind":"experience","title":"Engineer","organization":"AE","startDate":"2021-01","endDate":"","bullets":[{"text":"Built engine","skills":["Python"]}]}]}`}
	p, err := Digitize(context.Background(), f, []byte("%PDF"))
	if err != nil {
		panic(err)
	}
	return p
}

func TestDigitize(t *testing.T) {
	f := &fakeLLM{out: `{"name":"Ada","email":"a@e.com","phone":"","location":"","summary":"","links":[],"skills":["Python"],
		"items":[{"kind":"experience","title":"Engineer","organization":"AE","startDate":"2021-01","endDate":"","bullets":[{"text":"Built engine","skills":["Python"]}]}]}`}
	p, err := Digitize(context.Background(), f, []byte("%PDF"))
	if err != nil || p.Items[0].ID != "item-0" || p.Items[0].Bullets[0].ID != "item-0-b-0" {
		t.Fatalf("%+v %v", p, err)
	}
	if f.blocks[0].PDF == nil {
		t.Fatal("expected PDF block sent to LLM")
	}
}

func TestTailorValid(t *testing.T) {
	p := digitizedSample() // helper reusing TestDigitize fixture
	f := &fakeLLM{out: `{"targetRole":"Python Backend Engineer","headline":"h","summary":"s","selectedSkills":["Python"],
		"sections":[{"title":"Experience","items":[{"sourceId":"item-0","title":"Engineer","organization":"AE","dates":"2021 – Present",
		"bullets":[{"sourceBulletId":"item-0-b-0","text":"Built the engine in Python"}]}]}],
		"gaps":[{"requirement":"Django","evidence":"not in profile","severity":"missing"}],"whatChanged":["led with Python"]}`}
	ta, err := Tailor(context.Background(), f, p, "Python Backend Engineer")
	if err != nil || ta.TargetRole != "Python Backend Engineer" {
		t.Fatalf("%+v %v", ta, err)
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
