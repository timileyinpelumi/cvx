package ai

import (
	"context"
	"strings"
	"testing"

	"cvx/internal/model"
)

func goodLetter() model.CoverLetter {
	return model.CoverLetter{
		Greeting: "Dear hiring team,",
		Paragraphs: []string{
			"I am applying for the Python Backend Engineer role. At Analytical Engines Co I built the core computation engine in Python, designing the service layer that carried every production workload and cutting batch processing time for the largest datasets.",
			"That work maps directly onto what this role asks for. I wrote the first published algorithm for the engine, owned its correctness under load, and would bring the same care for measurable outcomes to your backend systems.",
		},
		Closing: "Sincerely,",
	}
}

func TestCheckCoverLetterPassesGoodLetter(t *testing.T) {
	if v := checkCoverLetter(goodLetter()); len(v) != 0 {
		t.Fatalf("good letter flagged: %v", v)
	}
}

func TestCheckCoverLetterCatchesViolations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*model.CoverLetter)
		want   string
	}{
		{"bad greeting", func(cl *model.CoverLetter) { cl.Greeting = "Hello!" }, "greeting"},
		{"em dash", func(cl *model.CoverLetter) { cl.Paragraphs[0] += " Systems — fast ones." }, "em or en dash"},
		{"exclamation", func(cl *model.CoverLetter) { cl.Paragraphs[1] += " I love this!" }, "exclamation"},
		{"placeholder", func(cl *model.CoverLetter) { cl.Paragraphs[0] += " I admire [Company]." }, "placeholder"},
		{"cliche", func(cl *model.CoverLetter) { cl.Paragraphs[0] += " I have a proven track record." }, "banned phrase"},
		{"one paragraph", func(cl *model.CoverLetter) { cl.Paragraphs = cl.Paragraphs[:1] }, "paragraphs"},
		{"too short", func(cl *model.CoverLetter) {
			cl.Paragraphs = []string{"I build systems.", "I would like this job."}
		}, "words"},
		{"long closing", func(cl *model.CoverLetter) { cl.Closing = "With my very warmest regards to all," }, "sign-off"},
		{"no comma", func(cl *model.CoverLetter) { cl.Closing = "Sincerely" }, "comma"},
		{"repeated paragraph", func(cl *model.CoverLetter) { cl.Paragraphs = append(cl.Paragraphs[:1], cl.Paragraphs[0], cl.Paragraphs[0]) }, "repeated"},
	}
	for _, tc := range cases {
		cl := goodLetter()
		tc.mutate(&cl)
		v := checkCoverLetter(cl)
		if len(v) == 0 {
			t.Errorf("%s: no violation flagged", tc.name)
			continue
		}
		if !strings.Contains(strings.Join(v, "; "), tc.want) {
			t.Errorf("%s: want violation mentioning %q, got %v", tc.name, tc.want, v)
		}
	}
}

func goodEmail() model.RecruiterEmail {
	return model.RecruiterEmail{
		Subject: "Application for Backend Engineer",
		Paragraphs: []string{
			"I am applying for the Backend Engineer role. At Analytical Engines Co I built the core analytical engine in Go and wrote its first published algorithm. My resume and the details are attached.",
		},
		Closing: "Best regards,",
	}
}

func TestCheckRecruiterEmailPassesGoodEmail(t *testing.T) {
	if v := checkRecruiterEmail(goodEmail(), "Ada Lovelace"); len(v) != 0 {
		t.Fatalf("good email flagged: %v", v)
	}
}

func TestCheckRecruiterEmailCatchesViolations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*model.RecruiterEmail)
		want   string
	}{
		{"empty subject", func(re *model.RecruiterEmail) { re.Subject = "" }, "subject is empty"},
		{"long subject", func(re *model.RecruiterEmail) { re.Subject = strings.Repeat("Backend ", 12) }, "80 characters"},
		{"too few sentences", func(re *model.RecruiterEmail) { re.Paragraphs = []string{"I want this role."} }, "sentences"},
		{"too many sentences", func(re *model.RecruiterEmail) {
			re.Paragraphs = []string{strings.Repeat("I built a system. ", 7)}
		}, "sentences"},
		{"name in closing", func(re *model.RecruiterEmail) { re.Closing = "Best regards, Ada Lovelace," }, "candidate's name"},
		{"three paragraphs", func(re *model.RecruiterEmail) {
			re.Paragraphs = []string{"One thing done.", "Two things done.", "Three things done."}
		}, "paragraphs"},
	}
	for _, tc := range cases {
		re := goodEmail()
		tc.mutate(&re)
		v := checkRecruiterEmail(re, "Ada Lovelace")
		if len(v) == 0 {
			t.Errorf("%s: no violation flagged", tc.name)
			continue
		}
		if !strings.Contains(strings.Join(v, "; "), tc.want) {
			t.Errorf("%s: want violation mentioning %q, got %v", tc.name, tc.want, v)
		}
	}
}

// queuedLLM returns canned outputs in order, recording how many calls and the
// last blocks, so retry tests can watch the corrective turn go out.
type queuedLLM struct {
	outs   []string
	calls  int
	blocks []ContentBlock
}

func (q *queuedLLM) GenerateJSON(_ context.Context, _ string, blocks []ContentBlock, _ map[string]any) ([]byte, error) {
	q.blocks = blocks
	out := q.outs[min(q.calls, len(q.outs)-1)]
	q.calls++
	return []byte(out), nil
}

const badLetterJSON = `{"greeting":"Hey!","paragraphs":["Short."],"closing":"Sincerely,"}`

func goodLetterJSON() string {
	cl := goodLetter()
	return `{"greeting":"` + cl.Greeting + `","paragraphs":["` + cl.Paragraphs[0] + `","` + cl.Paragraphs[1] + `"],"closing":"` + cl.Closing + `"}`
}

func TestCoverLetterRetriesOnceThenPasses(t *testing.T) {
	p := digitizedSample()
	q := &queuedLLM{outs: []string{badLetterJSON, goodLetterJSON()}}

	cl, err := CoverLetter(context.Background(), q, p, "Backend Engineer")
	if err != nil {
		t.Fatal(err)
	}
	if q.calls != 2 {
		t.Fatalf("want exactly 2 calls, got %d", q.calls)
	}
	if cl.Greeting != "Dear hiring team," {
		t.Fatalf("unexpected letter: %+v", cl)
	}

	joined := ""
	for _, b := range q.blocks {
		joined += b.Text + "\n"
	}
	if !strings.Contains(joined, "broke these rules") {
		t.Fatal("retry did not carry the corrective turn")
	}
}

func TestCoverLetterFailsAfterSecondBadDraft(t *testing.T) {
	p := digitizedSample()
	q := &queuedLLM{outs: []string{badLetterJSON}}

	_, err := CoverLetter(context.Background(), q, p, "Backend Engineer")
	if err == nil || !strings.Contains(err.Error(), "quality checks") {
		t.Fatalf("want quality-check error, got %v", err)
	}
	if q.calls != 2 {
		t.Fatalf("want exactly 2 calls (one retry, never more), got %d", q.calls)
	}
}
