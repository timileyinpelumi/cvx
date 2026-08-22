package store

import (
	"testing"
	"time"
)

func TestLatencyPercentiles(t *testing.T) {
	s := open(t)
	// 1..100ms, so the percentiles are known exactly rather than approximated.
	for i := 1; i <= 100; i++ {
		if err := s.RecordEvent(Event{Kind: EventGenerate, MS: int64(i), OK: true}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.LatencyByKind(time.Now().Add(-time.Hour), []string{EventGenerate, EventPreview})
	if err != nil {
		t.Fatal(err)
	}
	// A kind with no events is left out rather than shown as a row of zeros.
	if len(got) != 1 || got[0].Label != EventGenerate {
		t.Fatalf("got %+v", got)
	}
	l := got[0]
	if l.Count != 100 || l.Max != 100 {
		t.Fatalf("count/max: %+v", l)
	}
	if l.P50 != 50 || l.P95 != 95 {
		t.Fatalf("percentiles wrong: p50=%d p95=%d", l.P50, l.P95)
	}
}

// The averages that say whether the output is any good, not just how much
// of it there was.
func TestQualityReadsGenerateMeta(t *testing.T) {
	s := open(t)
	add := func(fit int, fill float64, trimmed, warnings int) {
		t.Helper()
		if err := s.RecordEvent(Event{Kind: EventGenerate, OK: true, Meta: map[string]any{
			"fit": fit, "fill": fill, "trimmed": trimmed, "warnings": warnings,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	add(80, 0.95, 2, 0)
	add(60, 0.70, 0, 1)

	q, err := s.QualitySince(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if q.Generations != 2 || q.AvgFit != 70 {
		t.Fatalf("quality: %+v", q)
	}
	if q.TrimmedShare != 0.5 || q.WarnedShare != 0.5 || q.ShortPageRate != 0.5 {
		t.Fatalf("shares: %+v", q)
	}
}

func TestSlowRequestsAndProviders(t *testing.T) {
	s := open(t)
	for _, ms := range []int64{100, 9000, 3000} {
		if err := s.RecordEvent(Event{Kind: EventRequest, Target: "POST /api/generate", MS: ms, OK: true,
			Meta: map[string]any{"id": "abc"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordEvent(Event{Kind: EventLLM, Target: "m", MS: 10, OK: true,
		Meta: map[string]any{"provider": "groq"}}); err != nil {
		t.Fatal(err)
	}

	slow, err := s.SlowRequests(time.Now().Add(-time.Hour), 2)
	if err != nil || len(slow) != 2 || slow[0].MS != 9000 {
		t.Fatalf("slowest first: %+v, %v", slow, err)
	}

	providers, err := s.LLMByProvider(time.Now().Add(-time.Hour))
	if err != nil || len(providers) != 1 || providers[0].Label != "groq" {
		t.Fatalf("providers: %+v, %v", providers, err)
	}
}

// A bad day should show as a step, not be averaged into the window.
func TestErrorRateByDayFillsGaps(t *testing.T) {
	s := open(t)
	if err := s.RecordEvent(Event{Kind: EventGenerate, OK: false, Detail: "boom"}); err != nil {
		t.Fatal(err)
	}
	rate, err := s.ErrorRateByDay(5)
	if err != nil || len(rate) != 5 {
		t.Fatalf("want 5 buckets, got %d, %v", len(rate), err)
	}
	if rate[len(rate)-1].Failed != 1 {
		t.Fatalf("today should carry the failure: %+v", rate[len(rate)-1])
	}
}
