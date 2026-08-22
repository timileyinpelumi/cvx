package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Event kinds. One vocabulary, used by the recorder and by every admin
// query, so a rename cannot silently split a metric in two.
const (
	EventSignup        = "signup"
	EventSignin        = "signin"
	EventAccountClose  = "account.close"
	EventProfileUpload = "profile.upload"
	EventProfileEdit   = "profile.edit"
	EventProfileExtend = "profile.extend"
	EventGenerate      = "generate"
	EventCoverLetter   = "cover_letter"
	EventRecruiterMail = "recruiter_email"
	EventFollowUp      = "followup"
	EventBulletRewrite = "bullet.rewrite"
	EventPreview       = "preview"
	EventDownload      = "download"
	EventStatusChange  = "status.change"
	EventLLM           = "llm"
	EventMail          = "mail"
	EventRequest       = "request"
)

// Event is one thing that happened. Deliberately flat: an admin panel that
// has to join four tables to answer "what is this costing me" is a panel
// nobody opens.
type Event struct {
	ID     int64  `json:"id"`
	At     string `json:"at"`
	UserID int64  `json:"userId,omitempty"`
	Kind   string `json:"kind"`
	// Target is the kind-specific subject: a model name for an llm event, a
	// route for a request, a provider for mail.
	Target string `json:"target,omitempty"`
	MS     int64  `json:"ms,omitempty"`
	OK     bool   `json:"ok"`
	// Detail is the failure reason, empty when OK.
	Detail string `json:"detail,omitempty"`
	// Meta carries the few numbers a kind needs: tokens, cost, counts.
	Meta map[string]any `json:"meta,omitempty"`
}

// eventRetention bounds the table. Events are small and the disk is not,
// but an append-only table with no ceiling is a slow leak.
const eventRetention = 90 * 24 * time.Hour

// RecordEvent appends one event. Errors are returned but callers are
// expected to log and continue: telemetry must never fail the thing it is
// measuring.
func (s *Store) RecordEvent(e Event) error {
	var meta any
	if len(e.Meta) > 0 {
		b, err := json.Marshal(e.Meta)
		if err != nil {
			return err
		}
		meta = string(b)
	}
	var userID any
	if e.UserID != 0 {
		userID = e.UserID
	}
	_, err := s.db.Exec(
		`INSERT INTO events (at, user_id, kind, target, ms, ok, detail, meta)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano), userID, e.Kind, e.Target,
		e.MS, boolToInt(e.OK), e.Detail, meta,
	)
	return err
}

// PruneEvents drops everything past the retention window. Called once from
// main on startup, like PruneLLMCache.
func (s *Store) PruneEvents() {
	cutoff := time.Now().UTC().Add(-eventRetention).Format(time.RFC3339Nano)
	s.db.Exec(`DELETE FROM events WHERE at < ?`, cutoff)
}

// EventFilter narrows a listing. Zero value means "everything, newest
// first".
type EventFilter struct {
	Kind    string
	UserID  int64
	OnlyBad bool
	Since   time.Time
	Limit   int
}

// ListEvents returns the newest events matching f.
func (s *Store) ListEvents(f EventFilter) ([]Event, error) {
	where := []string{"1 = 1"}
	var args []any
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.UserID != 0 {
		where = append(where, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.OnlyBad {
		where = append(where, "ok = 0")
	}
	if !f.Since.IsZero() {
		where = append(where, "at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)

	rows, err := s.db.Query(
		`SELECT id, at, COALESCE(user_id, 0), kind, target, ms, ok, detail, COALESCE(meta, '')
		 FROM events WHERE `+strings.Join(where, " AND ")+
			` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var e Event
		var ok int
		var meta string
		if err := rows.Scan(&e.ID, &e.At, &e.UserID, &e.Kind, &e.Target, &e.MS, &ok, &e.Detail, &meta); err != nil {
			return nil, err
		}
		e.OK = ok != 0
		if meta != "" {
			json.Unmarshal([]byte(meta), &e.Meta)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

/* ------------------------------ aggregates ------------------------------ */

// Count is one bucket of an aggregate: a label and its numbers. Shared by
// every admin query so the panel renders one shape.
type Count struct {
	Label  string  `json:"label"`
	Total  int     `json:"total"`
	Failed int     `json:"failed"`
	MS     int64   `json:"ms,omitempty"`
	Tokens int64   `json:"tokens,omitempty"`
	Cost   float64 `json:"cost,omitempty"`
}

// CountsByKind summarizes activity since a point in time: how much of each
// kind happened, how much of it failed, and how slow it was.
func (s *Store) CountsByKind(since time.Time) ([]Count, error) {
	rows, err := s.db.Query(
		`SELECT kind, COUNT(*), SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END), COALESCE(CAST(AVG(ms) AS INTEGER), 0)
		 FROM events WHERE at >= ? GROUP BY kind ORDER BY COUNT(*) DESC`,
		since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCounts(rows)
}

// DailyCounts returns one bucket per day for a kind, oldest first, so the
// panel can draw a sparkline without a charting library.
func (s *Store) DailyCounts(kind string, days int) ([]Count, error) {
	if days <= 0 || days > 90 {
		days = 14
	}
	since := time.Now().UTC().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)

	args := []any{since.Format(time.RFC3339Nano)}
	kindClause := ""
	if kind != "" {
		kindClause = " AND kind = ?"
		args = append(args, kind)
	}

	rows, err := s.db.Query(
		`SELECT substr(at, 1, 10) AS day, COUNT(*), SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END), 0
		 FROM events WHERE at >= ?`+kindClause+
			` GROUP BY day ORDER BY day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byDay := map[string]Count{}
	found, err := scanCounts(rows)
	if err != nil {
		return nil, err
	}
	for _, c := range found {
		byDay[c.Label] = c
	}

	// Fill the gaps: a day with nothing in it is a zero, not a missing bar.
	out := make([]Count, 0, days)
	for i := 0; i < days; i++ {
		day := since.AddDate(0, 0, i).Format("2006-01-02")
		if c, ok := byDay[day]; ok {
			out = append(out, c)
			continue
		}
		out = append(out, Count{Label: day})
	}
	return out, nil
}

// TokenUsage totals what the LLM calls have cost, grouped by model.
func (s *Store) TokenUsage(since time.Time) ([]Count, error) {
	rows, err := s.db.Query(
		`SELECT target,
		        COUNT(*),
		        SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END),
		        COALESCE(CAST(AVG(ms) AS INTEGER), 0),
		        COALESCE(SUM(json_extract(meta, '$.tokens')), 0),
		        COALESCE(SUM(json_extract(meta, '$.cost')), 0)
		 FROM events WHERE kind = ? AND at >= ?
		 GROUP BY target ORDER BY SUM(json_extract(meta, '$.tokens')) DESC`,
		EventLLM, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Count{}
	for rows.Next() {
		var c Count
		if err := rows.Scan(&c.Label, &c.Total, &c.Failed, &c.MS, &c.Tokens, &c.Cost); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FailureGroups returns recent failures grouped by kind and reason, most
// frequent first, each carrying its last message. A count on its own is not
// actionable; a count with the sentence that caused it is.
func (s *Store) FailureGroups(since time.Time, limit int) ([]Count, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT kind || ': ' || COALESCE(NULLIF(detail, ''), 'no reason recorded') AS label,
		        COUNT(*), COUNT(*), 0
		 FROM events WHERE ok = 0 AND at >= ?
		 GROUP BY label ORDER BY COUNT(*) DESC LIMIT ?`,
		since.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCounts(rows)
}

func scanCounts(rows *sql.Rows) ([]Count, error) {
	out := []Count{}
	for rows.Next() {
		var c Count
		if err := rows.Scan(&c.Label, &c.Total, &c.Failed, &c.MS); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DatabaseSize reports the file size in bytes, so the panel can show the
// one number that decides when the volume needs attention.
func (s *Store) DatabaseSize() (int64, error) {
	var pageCount, pageSize int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		return 0, err
	}
	if err := s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) eventTableCheck() error {
	var name string
	if err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='events'`).Scan(&name); err != nil {
		return fmt.Errorf("store: events table missing: %w", err)
	}
	return nil
}
