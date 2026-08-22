package eval

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"cvx/internal/ai"
	"cvx/internal/model"
)

// queueLLM is a fake ai.LLM that returns canned JSON strings in order, one
// per call. Run makes calls in a fixed, deterministic sequence (tailor,
// [cover letter], per fixture), so a queue is enough to script whole runs.
type queueLLM struct {
	outs []string
	i    int
}

func (q *queueLLM) GenerateJSON(_ context.Context, _ string, _ []ai.ContentBlock, _ map[string]any) ([]byte, error) {
	if q.i >= len(q.outs) {
		return nil, errQueueExhausted
	}
	out := q.outs[q.i]
	q.i++
	return []byte(out), nil
}

var errQueueExhausted = errors.New("queueLLM: no more canned outputs")

// erroringLLM is a fake ai.LLM whose GenerateJSON always fails — it stands
// in for a real infra failure (network, provider outage, ...), as opposed
// to a model that responded but cited a fabricated id.
type erroringLLM struct{ err error }

func (e *erroringLLM) GenerateJSON(_ context.Context, _ string, _ []ai.ContentBlock, _ map[string]any) ([]byte, error) {
	return nil, e.err
}

func validTailoredJSON(nItems int) string {
	// The fixture profile (server/eval/fixtures/profile.json) has 6 items,
	// item-0..item-5, each with at least 3 bullets (item-N-b-0, -b-1 always
	// exist), so any nItems in [1,6] can be satisfied with real ids.
	var items []string
	for i := 0; i < nItems; i++ {
		id := strconv.Itoa(i)
		items = append(items, `{"sourceId":"item-`+id+`","title":"T","organization":"O","dates":"2020",`+
			`"bullets":[{"sourceBulletId":"item-`+id+`-b-0","text":"did a thing"},`+
			`{"sourceBulletId":"item-`+id+`-b-1","text":"did another thing"}]}`)
	}
	return `{"targetRole":"Backend Engineer","headline":"Senior Backend Engineer","summary":"Backend engineer who builds and operates the services other teams depend on, with most of that work in Go and Python behind high-traffic APIs. Owned the computation engine that carried every production workload, took its batch processing time down, and kept it correct under load. Comfortable across PostgreSQL, Docker, and the observability work that keeps a distributed system honest, which is the same ground this role covers.",` +
		`"selectedSkills":["Go","Python","PostgreSQL","Docker","Distributed Systems","Observability"],` +
		`"sections":[{"title":"Experience","items":[` + strings.Join(items, ",") + `]}],` +
		`"gaps":[{"requirement":"Kubernetes at scale","evidence":"only single-cluster experience","severity":"weak"}],` +
		`"whatChanged":["led with backend depth"]}`
}

const validResumeRubricJSON = `{"selection":{"score":8,"rationale":"good picks"},` +
	`"vocabulary":{"score":7,"rationale":"mirrors JD reasonably"},` +
	`"bulletStrength":{"score":8,"rationale":"specific"},` +
	`"honesty":{"score":9,"rationale":"traceable to profile"},` +
	`"gapQuality":{"score":6,"rationale":"real gap"},` +
	`"headlineSummary":{"score":8,"rationale":"apt"}}`

const validCoverLetterJSON = `{"greeting":"Dear hiring team,",` +
	`"paragraphs":["I am applying for the Backend Engineer role. At Analytical Engines Co I built the core computation engine in Go, designing the service layer that carried every production workload and cutting batch processing time for the largest datasets.",` +
	`"That work maps directly onto what this role asks for. I wrote the first published algorithm for the engine, owned its correctness under load, and would bring the same care for measurable outcomes to your backend systems."],` +
	`"closing":"Sincerely,"}`

const validCoverRubricJSON = `{"specificity":{"score":7,"rationale":"specific enough"},` +
	`"voice":{"score":8,"rationale":"natural"},` +
	`"factuality":{"score":9,"rationale":"traceable"}}`

func TestRunHappyPath(t *testing.T) {
	gen := &queueLLM{outs: []string{validTailoredJSON(3)}}
	judge := &queueLLM{outs: []string{validResumeRubricJSON}}

	report, err := Run(context.Background(), gen, judge, "fixtures", []string{"backend-go"}, RunOptions{CoverLetters: false})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Fixtures) != 1 {
		t.Fatalf("want 1 fixture, got %d", len(report.Fixtures))
	}
	fr := report.Fixtures[0]
	if fr.ID != "backend-go" {
		t.Fatalf("want id backend-go, got %q", fr.ID)
	}
	if fr.Error != "" {
		t.Fatalf("unexpected fixture error: %s", fr.Error)
	}
	if fr.Resume == nil || fr.Resume.Selection.Score != 8 {
		t.Fatalf("want resume rubric with selection=8, got %+v", fr.Resume)
	}
	for _, c := range fr.Checks {
		if !c.Pass {
			t.Errorf("check %s unexpectedly failed: %s", c.Name, c.Detail)
		}
	}
}

func TestRunDetectsItemCountViolation(t *testing.T) {
	gen := &queueLLM{outs: []string{validTailoredJSON(6)}}
	judge := &queueLLM{outs: []string{validResumeRubricJSON}}

	report, err := Run(context.Background(), gen, judge, "fixtures", []string{"backend-go"}, RunOptions{CoverLetters: false})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	fr := report.Fixtures[0]

	var itemCount *Check
	for i := range fr.Checks {
		if fr.Checks[i].Name == "itemCount" {
			itemCount = &fr.Checks[i]
		}
	}
	if itemCount == nil {
		t.Fatal("no itemCount check found")
	}
	if itemCount.Pass {
		t.Fatalf("want itemCount check to fail for 6 items, got pass; detail=%s", itemCount.Detail)
	}
	if !strings.Contains(itemCount.Detail, "6") {
		t.Fatalf("want detail to mention 6 items, got %q", itemCount.Detail)
	}

	// Guardrail itself should still pass — all sourceIds/sourceBulletIds are
	// real — this is specifically an item-count violation, not a guardrail one.
	for _, c := range fr.Checks {
		if c.Name == "guardrail" && !c.Pass {
			t.Fatalf("guardrail unexpectedly failed: %s", c.Detail)
		}
	}
}

func TestRunGuardrailFailureSkipsJudge(t *testing.T) {
	fabricated := `{"targetRole":"X","headline":"h","summary":"s","selectedSkills":[],
		"sections":[{"title":"Experience","items":[{"sourceId":"item-99","title":"CTO","organization":"","dates":"",
		"bullets":[{"sourceBulletId":"item-99-b-0","text":"Ran everything"}]}]}],"gaps":[],"whatChanged":[]}`
	gen := &queueLLM{outs: []string{fabricated}}
	judge := &queueLLM{outs: []string{validResumeRubricJSON}} // must never be consumed

	report, err := Run(context.Background(), gen, judge, "fixtures", []string{"backend-go"}, RunOptions{CoverLetters: false})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	fr := report.Fixtures[0]
	if fr.Error == "" {
		t.Fatal("want fixture-level error for guardrail failure")
	}
	if len(fr.Checks) != 1 || fr.Checks[0].Name != "guardrail" || fr.Checks[0].Pass {
		t.Fatalf("want single failing guardrail check, got %+v", fr.Checks)
	}
	if fr.Resume != nil {
		t.Fatal("want no resume rubric when guardrail fails")
	}
	if judge.i != 0 {
		t.Fatalf("want judge never called, got %d calls", judge.i)
	}
}

func TestRunGenerationErrorIsNotGuardrail(t *testing.T) {
	gen := &erroringLLM{err: errors.New("network unreachable")}
	judge := &queueLLM{outs: []string{validResumeRubricJSON}} // must never be consumed

	report, err := Run(context.Background(), gen, judge, "fixtures", []string{"backend-go"}, RunOptions{CoverLetters: false})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	fr := report.Fixtures[0]

	if !fr.GenerationError {
		t.Fatalf("want GenerationError=true for an infra failure, got %+v", fr)
	}
	if len(fr.Checks) != 0 {
		t.Fatalf("want no deterministic checks recorded for a generation error (excluded from pass rate), got %+v", fr.Checks)
	}
	if fr.Error == "" || !strings.Contains(fr.Error, "network unreachable") {
		t.Fatalf("want error detail naming the underlying failure, got %q", fr.Error)
	}
	for _, c := range fr.Checks {
		if c.Name == "guardrail" {
			t.Fatalf("want no guardrail check for an infra failure, got %+v", c)
		}
	}
	if judge.i != 0 {
		t.Fatalf("want judge never called, got %d calls", judge.i)
	}

	if agg := report.Aggregate; agg.ErroredFixtures != 1 || agg.GuardrailFailedFixtures != 0 {
		t.Fatalf("want ErroredFixtures=1, GuardrailFailedFixtures=0, got %+v", agg)
	}
}

func TestRunWithCoverLetters(t *testing.T) {
	gen := &queueLLM{outs: []string{validTailoredJSON(3), validCoverLetterJSON}}
	judge := &queueLLM{outs: []string{validResumeRubricJSON, validCoverRubricJSON}}

	report, err := Run(context.Background(), gen, judge, "fixtures", []string{"backend-go"}, RunOptions{CoverLetters: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	fr := report.Fixtures[0]
	if fr.Cover == nil {
		t.Fatal("want cover rubric")
	}
	if fr.Cover.Specificity.Score != 7 || fr.Cover.Voice.Score != 8 || fr.Cover.Factuality.Score != 9 {
		t.Fatalf("unexpected cover rubric: %+v", fr.Cover)
	}
}

func TestIsGuardrailError(t *testing.T) {
	guardrail := errors.New("tailored output references unknown profile item: item-99")
	if !isGuardrailError(guardrail) {
		t.Errorf("want unwrapped ValidateTailored error classified as guardrail")
	}

	infra := errors.New("tailor: request timed out")
	if isGuardrailError(infra) {
		t.Errorf("want \"tailor: \"-prefixed error classified as infra, not guardrail")
	}
}

func TestRunUnknownFixtureID(t *testing.T) {
	gen := &queueLLM{}
	judge := &queueLLM{}
	if _, err := Run(context.Background(), gen, judge, "fixtures", []string{"nonexistent"}, RunOptions{CoverLetters: false}); err == nil || !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("want error naming unknown id, got %v", err)
	}
}

func TestRunSequentialOrder(t *testing.T) {
	// Two fixtures, no cover letters: gen/judge should each be called
	// exactly twice, once per fixture, strictly in sequence.
	gen := &queueLLM{outs: []string{validTailoredJSON(3), validTailoredJSON(3)}}
	judge := &queueLLM{outs: []string{validResumeRubricJSON, validResumeRubricJSON}}

	report, err := Run(context.Background(), gen, judge, "fixtures", []string{"backend-go", "backend-python"}, RunOptions{CoverLetters: false})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Fixtures) != 2 {
		t.Fatalf("want 2 fixtures, got %d", len(report.Fixtures))
	}
	if report.Fixtures[0].ID != "backend-go" || report.Fixtures[1].ID != "backend-python" {
		t.Fatalf("want fixtures in index order, got %s, %s", report.Fixtures[0].ID, report.Fixtures[1].ID)
	}
	if gen.i != 2 || judge.i != 2 {
		t.Fatalf("want 2 gen calls and 2 judge calls, got gen=%d judge=%d", gen.i, judge.i)
	}
}

// --- individual deterministic check unit tests ---

func baseTailored() model.Tailored {
	return model.Tailored{
		Headline: "Senior Backend Engineer",
		Summary:  "A concise summary under sixty words.",
		Sections: []model.TSection{{
			Title: "Experience",
			Items: []model.TItem{{
				SourceID: "item-0",
				Bullets: []model.TBullet{
					{SourceBulletID: "item-0-b-0", Text: "a"},
					{SourceBulletID: "item-0-b-1", Text: "b"},
				},
			}},
		}},
		Gaps:           []model.Gap{{Requirement: "Kubernetes", Evidence: "thin", Severity: "weak"}},
		SelectedSkills: []string{"Go"},
	}
}

func TestItemCountCheckPass(t *testing.T) {
	c := itemCountCheck(baseTailored())
	if !c.Pass {
		t.Fatalf("want pass, got %+v", c)
	}
}

func TestItemCountCheckFail(t *testing.T) {
	tr := baseTailored()
	for i := 0; i < 5; i++ {
		tr.Sections[0].Items = append(tr.Sections[0].Items, model.TItem{SourceID: "item-x"})
	}
	c := itemCountCheck(tr)
	if c.Pass {
		t.Fatal("want fail for 6 items")
	}
}

func TestBulletsPerItemCheckFailTooFew(t *testing.T) {
	tr := baseTailored()
	tr.Sections[0].Items[0].Bullets = tr.Sections[0].Items[0].Bullets[:1]
	c := bulletsPerItemCheck(tr)
	if c.Pass {
		t.Fatal("want fail for 1 bullet")
	}
}

func TestBulletsPerItemCheckFailTooMany(t *testing.T) {
	tr := baseTailored()
	tr.Sections[0].Items[0].Bullets = append(tr.Sections[0].Items[0].Bullets,
		model.TBullet{SourceBulletID: "item-0-b-2", Text: "c"},
		model.TBullet{SourceBulletID: "item-0-b-3", Text: "d"},
		model.TBullet{SourceBulletID: "item-0-b-4", Text: "e"})
	c := bulletsPerItemCheck(tr)
	if c.Pass {
		t.Fatal("want fail for 5 bullets")
	}
}

func TestSummaryCheckEmptyFails(t *testing.T) {
	tr := baseTailored()
	tr.Summary = "   "
	c := summaryCheck(tr)
	if c.Pass {
		t.Fatal("want fail for empty summary")
	}
}

func TestSummaryCheckTooShortFails(t *testing.T) {
	tr := baseTailored()
	words := make([]string, model.MinSummaryWords-1)
	for i := range words {
		words[i] = "word"
	}
	tr.Summary = strings.Join(words, " ")
	if c := summaryCheck(tr); c.Pass {
		t.Fatalf("want fail for %d-word summary", len(words))
	}
}

func TestSummaryCheckTooLongFails(t *testing.T) {
	tr := baseTailored()
	words := make([]string, model.MaxSummaryWords+1)
	for i := range words {
		words[i] = "word"
	}
	tr.Summary = strings.Join(words, " ")
	if c := summaryCheck(tr); c.Pass {
		t.Fatalf("want fail for %d-word summary", len(words))
	}
}

func TestHeadlineCheckEmptyFails(t *testing.T) {
	tr := baseTailored()
	tr.Headline = ""
	c := headlineCheck(tr)
	if c.Pass {
		t.Fatal("want fail for empty headline")
	}
}

func TestGapsCheckBadSeverityFails(t *testing.T) {
	tr := baseTailored()
	tr.Gaps = []model.Gap{{Requirement: "X", Evidence: "y", Severity: "critical"}}
	c := gapsCheck(tr)
	if c.Pass {
		t.Fatal("want fail for invalid severity")
	}
}

func TestGapsCheckEmptyRequirementFails(t *testing.T) {
	tr := baseTailored()
	tr.Gaps = []model.Gap{{Requirement: "", Evidence: "y", Severity: "missing"}}
	c := gapsCheck(tr)
	if c.Pass {
		t.Fatal("want fail for empty requirement")
	}
}

func TestSelectedSkillsCheckSubsetPasses(t *testing.T) {
	p := model.Profile{Skills: []string{"Go", "PostgreSQL"}}
	tr := baseTailored()
	tr.SelectedSkills = []string{"go", "POSTGRESQL"} // case-insensitive match
	c := selectedSkillsCheck(p, tr)
	if !c.Pass {
		t.Fatalf("want pass, got %+v", c)
	}
}

func TestSelectedSkillsCheckNotInProfileFails(t *testing.T) {
	p := model.Profile{Skills: []string{"Go"}}
	tr := baseTailored()
	tr.SelectedSkills = []string{"Go", "Rust"}
	c := selectedSkillsCheck(p, tr)
	if c.Pass {
		t.Fatal("want fail: Rust not in profile skills")
	}
	if !strings.Contains(c.Detail, "Rust") {
		t.Fatalf("want detail to name Rust, got %q", c.Detail)
	}
}

func TestVoiceCheckCatchesNarratorSummary(t *testing.T) {
	p := model.Profile{Name: "Ada Lovelace"}
	tr := baseTailored()
	tr.Summary = "Ada builds analytical engines. She wrote the first published algorithm and owns the engine's correctness under load."
	if c := voiceCheck(p, tr); c.Pass {
		t.Fatal("want fail for a summary written about the candidate")
	}
}

func TestVoiceCheckPassesImpliedFirstPerson(t *testing.T) {
	p := model.Profile{Name: "Ada Lovelace"}
	tr := baseTailored()
	tr.Summary = "Mathematician and engineer who builds analytical engines, with the first published algorithm to the name and ownership of the engine's correctness under load."
	if c := voiceCheck(p, tr); !c.Pass {
		t.Fatalf("clean summary flagged: %s", c.Detail)
	}
}
