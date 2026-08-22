package store

import (
	"testing"
	"time"
)

func TestRecordAndListEvents(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "events-user")

	must := func(e Event) {
		t.Helper()
		if err := s.RecordEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	must(Event{UserID: userID, Kind: EventGenerate, Target: "Backend Engineer", MS: 900, OK: true})
	must(Event{UserID: userID, Kind: EventGenerate, Target: "Data Engineer", MS: 1200, OK: false, Detail: "tailor failed"})
	must(Event{Kind: EventLLM, Target: "gpt-oss-120b", MS: 700, OK: true,
		Meta: map[string]any{"tokens": 1500, "cost": 0.02}})

	all, err := s.ListEvents(EventFilter{})
	if err != nil || len(all) != 3 {
		t.Fatalf("want 3 events, got %d, %v", len(all), err)
	}
	// Newest first: an admin tail that reads oldest first is unusable.
	if all[0].Kind != EventLLM {
		t.Fatalf("ordering wrong: %+v", all[0])
	}
	if all[0].Meta["tokens"] != float64(1500) {
		t.Fatalf("meta lost: %+v", all[0].Meta)
	}

	bad, err := s.ListEvents(EventFilter{OnlyBad: true})
	if err != nil || len(bad) != 1 || bad[0].Detail != "tailor failed" {
		t.Fatalf("failure filter: %+v, %v", bad, err)
	}

	byKind, err := s.ListEvents(EventFilter{Kind: EventGenerate})
	if err != nil || len(byKind) != 2 {
		t.Fatalf("kind filter: %d, %v", len(byKind), err)
	}

	byUser, err := s.ListEvents(EventFilter{UserID: userID})
	if err != nil || len(byUser) != 2 {
		t.Fatalf("user filter: %d, %v", len(byUser), err)
	}
}

func TestAggregatesAnswerThePanelsQuestions(t *testing.T) {
	s := open(t)
	since := time.Now().Add(-time.Hour)

	for i := 0; i < 3; i++ {
		if err := s.RecordEvent(Event{Kind: EventGenerate, MS: 1000, OK: i != 2}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordEvent(Event{Kind: EventLLM, Target: "gpt-oss-120b", MS: 500, OK: true,
		Meta: map[string]any{"tokens": 1000, "cost": 0.01}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEvent(Event{Kind: EventLLM, Target: "gpt-oss-120b", MS: 700, OK: true,
		Meta: map[string]any{"tokens": 2000, "cost": 0.03}}); err != nil {
		t.Fatal(err)
	}

	kinds, err := s.CountsByKind(since)
	if err != nil {
		t.Fatal(err)
	}
	var generate Count
	for _, k := range kinds {
		if k.Label == EventGenerate {
			generate = k
		}
	}
	if generate.Total != 3 || generate.Failed != 1 || generate.MS != 1000 {
		t.Fatalf("counts by kind: %+v", generate)
	}

	usage, err := s.TokenUsage(since)
	if err != nil || len(usage) != 1 {
		t.Fatalf("token usage: %+v, %v", usage, err)
	}
	if usage[0].Tokens != 3000 || usage[0].Cost < 0.039 || usage[0].Cost > 0.041 {
		t.Fatalf("token usage totals wrong: %+v", usage[0])
	}

	failures, err := s.FailureGroups(since, 10)
	if err != nil || len(failures) != 1 || failures[0].Total != 1 {
		t.Fatalf("failure groups: %+v, %v", failures, err)
	}
}

// A day with nothing in it is a zero bar, not a missing one: a sparkline
// with gaps silently rescales and lies about the trend.
func TestDailyCountsFillsEmptyDays(t *testing.T) {
	s := open(t)
	if err := s.RecordEvent(Event{Kind: EventGenerate, OK: true}); err != nil {
		t.Fatal(err)
	}
	daily, err := s.DailyCounts(EventGenerate, 7)
	if err != nil || len(daily) != 7 {
		t.Fatalf("want 7 buckets, got %d, %v", len(daily), err)
	}
	if daily[len(daily)-1].Total != 1 {
		t.Fatalf("today should hold the event: %+v", daily[len(daily)-1])
	}
	if daily[0].Total != 0 {
		t.Fatalf("an empty day should be zero: %+v", daily[0])
	}
}

func TestAdminTotalsAndFunnel(t *testing.T) {
	s := open(t)
	userID := testUser(t, s, "google", "funnel-user")

	for _, kind := range []string{EventSignup, EventProfileUpload, EventGenerate, EventGenerate, EventDownload} {
		if err := s.RecordEvent(Event{UserID: userID, Kind: kind, OK: true}); err != nil {
			t.Fatal(err)
		}
	}

	f, err := s.FunnelSince(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if f.Signups != 1 || f.Uploads != 1 || f.Generations != 2 || f.Downloads != 1 || f.Sent != 0 {
		t.Fatalf("funnel: %+v", f)
	}

	totals, err := s.AdminTotals()
	if err != nil {
		t.Fatal(err)
	}
	if totals.Users != 1 || totals.ActiveUsers != 1 || totals.Events != 5 || totals.DBBytes == 0 {
		t.Fatalf("totals: %+v", totals)
	}

	users, err := s.ListAdminUsers(10)
	if err != nil || len(users) != 1 || users[0].LastSeen == "" {
		t.Fatalf("admin users: %+v, %v", users, err)
	}
}
