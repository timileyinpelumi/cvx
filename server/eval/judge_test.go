package eval

import (
	"context"
	"errors"
	"testing"

	"cvx/internal/ai"
	"cvx/internal/model"
)

type fakeLLM struct {
	out    string
	err    error
	system string
	blocks []ai.ContentBlock
}

func (f *fakeLLM) GenerateJSON(_ context.Context, system string, blocks []ai.ContentBlock, _ map[string]any) ([]byte, error) {
	f.system, f.blocks = system, blocks
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.out), nil
}

func TestJudgeResumeParsesRubric(t *testing.T) {
	f := &fakeLLM{out: validResumeRubricJSON}
	p := model.Profile{Name: "Alex"}
	tr := baseTailored()

	rubric, err := JudgeResume(context.Background(), f, p, "some JD text", tr)
	if err != nil {
		t.Fatalf("JudgeResume: %v", err)
	}
	if rubric.Selection.Score != 8 || rubric.Selection.Rationale != "good picks" {
		t.Fatalf("unexpected selection: %+v", rubric.Selection)
	}
	if rubric.Honesty.Score != 9 {
		t.Fatalf("unexpected honesty: %+v", rubric.Honesty)
	}
	if len(f.blocks) != 3 {
		t.Fatalf("want 3 content blocks (profile, JD, tailored), got %d", len(f.blocks))
	}
	if f.system == "" {
		t.Fatal("want non-empty system prompt")
	}
}

func TestJudgeResumePropagatesLLMError(t *testing.T) {
	f := &fakeLLM{err: errors.New("boom")}
	_, err := JudgeResume(context.Background(), f, model.Profile{}, "jd", model.Tailored{})
	if err == nil {
		t.Fatal("want error propagated")
	}
}

func TestJudgeResumeRejectsMalformedJSON(t *testing.T) {
	f := &fakeLLM{out: `{not json`}
	_, err := JudgeResume(context.Background(), f, model.Profile{}, "jd", model.Tailored{})
	if err == nil {
		t.Fatal("want unmarshal error")
	}
}

func TestJudgeCoverLetterParsesRubric(t *testing.T) {
	f := &fakeLLM{out: validCoverRubricJSON}
	p := model.Profile{Name: "Alex"}
	cl := model.CoverLetter{Greeting: "Dear hiring team,", Paragraphs: []string{"para"}, Closing: "Sincerely"}

	rubric, err := JudgeCoverLetter(context.Background(), f, p, "some JD text", cl)
	if err != nil {
		t.Fatalf("JudgeCoverLetter: %v", err)
	}
	if rubric.Specificity.Score != 7 || rubric.Voice.Score != 8 || rubric.Factuality.Score != 9 {
		t.Fatalf("unexpected rubric: %+v", rubric)
	}
	if len(f.blocks) != 3 {
		t.Fatalf("want 3 content blocks (profile, JD, cover letter), got %d", len(f.blocks))
	}
}

func TestJudgeCoverLetterPropagatesLLMError(t *testing.T) {
	f := &fakeLLM{err: errors.New("boom")}
	_, err := JudgeCoverLetter(context.Background(), f, model.Profile{}, "jd", model.CoverLetter{})
	if err == nil {
		t.Fatal("want error propagated")
	}
}
