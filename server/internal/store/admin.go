package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// AdminUser is one account as the panel shows it: who they are, when they
// were last seen, and what they have cost.
type AdminUser struct {
	ID          int64   `json:"id"`
	Email       string  `json:"email"`
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	CreatedAt   string  `json:"createdAt"`
	LastSeen    string  `json:"lastSeen,omitempty"`
	Deleted     bool    `json:"deleted"`
	Generations int     `json:"generations"`
	Tokens      int64   `json:"tokens"`
	Cost        float64 `json:"cost"`
}

// ListAdminUsers returns every account, newest first, with the numbers that
// decide whether one of them is a problem.
func (s *Store) ListAdminUsers(limit int) ([]AdminUser, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT u.id, u.email, u.name, u.provider, u.created_at, u.deleted_at IS NOT NULL,
		        (SELECT COUNT(*) FROM generations g WHERE g.user_id = u.id),
		        (SELECT COALESCE(MAX(e.at), '') FROM events e WHERE e.user_id = u.id),
		        (SELECT COALESCE(SUM(json_extract(e.meta, '$.tokens')), 0) FROM events e
		          WHERE e.user_id = u.id AND e.kind = 'llm'),
		        (SELECT COALESCE(SUM(json_extract(e.meta, '$.cost')), 0) FROM events e
		          WHERE e.user_id = u.id AND e.kind = 'llm')
		 FROM users u ORDER BY u.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AdminUser{}
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.CreatedAt,
			&u.Deleted, &u.Generations, &u.LastSeen, &u.Tokens, &u.Cost); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Funnel is where people fall out: how many reached each step in a window.
// The interesting number is never the total, it is the gap between two
// adjacent steps.
type Funnel struct {
	Signups     int `json:"signups"`
	Uploads     int `json:"uploads"`
	Generations int `json:"generations"`
	Downloads   int `json:"downloads"`
	Sent        int `json:"sent"`
}

func (s *Store) FunnelSince(since time.Time) (Funnel, error) {
	stamp := since.UTC().Format(time.RFC3339Nano)
	count := func(kind string) (int, error) {
		var n int
		err := s.db.QueryRow(
			`SELECT COUNT(*) FROM events WHERE kind = ? AND ok = 1 AND at >= ?`, kind, stamp).Scan(&n)
		return n, err
	}

	var f Funnel
	var err error
	if f.Signups, err = count(EventSignup); err != nil {
		return f, err
	}
	if f.Uploads, err = count(EventProfileUpload); err != nil {
		return f, err
	}
	if f.Generations, err = count(EventGenerate); err != nil {
		return f, err
	}
	if f.Downloads, err = count(EventDownload); err != nil {
		return f, err
	}
	if f.Sent, err = count(EventStatusChange); err != nil {
		return f, err
	}
	return f, nil
}

// Totals are the all-time counts the panel shows as its top row.
type Totals struct {
	Users       int   `json:"users"`
	ActiveUsers int   `json:"activeUsers"`
	Profiles    int   `json:"profiles"`
	Generations int   `json:"generations"`
	Events      int   `json:"events"`
	DBBytes     int64 `json:"dbBytes"`
}

func (s *Store) AdminTotals() (Totals, error) {
	var t Totals
	one := func(q string, dest any) error { return s.db.QueryRow(q).Scan(dest) }

	if err := one(`SELECT COUNT(*) FROM users`, &t.Users); err != nil {
		return t, err
	}
	if err := one(`SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`, &t.ActiveUsers); err != nil {
		return t, err
	}
	if err := one(`SELECT COUNT(*) FROM profile`, &t.Profiles); err != nil {
		return t, err
	}
	if err := one(`SELECT COUNT(*) FROM generations`, &t.Generations); err != nil {
		return t, err
	}
	if err := one(`SELECT COUNT(*) FROM events`, &t.Events); err != nil {
		return t, err
	}
	size, err := s.DatabaseSize()
	if err != nil {
		return t, err
	}
	t.DBBytes = size
	return t, nil
}

// Latency is the distribution of one thing's duration. An average hides the
// tail, and the tail is what people actually complain about.
type Latency struct {
	Label string `json:"label"`
	Count int    `json:"count"`
	P50   int64  `json:"p50"`
	P95   int64  `json:"p95"`
	Max   int64  `json:"max"`
}

// LatencyByKind computes percentiles per event kind. SQLite has no
// percentile function, so it is done with an offset into an ordered window,
// which is exact rather than an approximation.
func (s *Store) LatencyByKind(since time.Time, kinds []string) ([]Latency, error) {
	stamp := since.UTC().Format(time.RFC3339Nano)
	out := []Latency{}

	for _, kind := range kinds {
		var l Latency
		l.Label = kind
		if err := s.db.QueryRow(
			`SELECT COUNT(*), COALESCE(MAX(ms), 0) FROM events WHERE kind = ? AND ms > 0 AND at >= ?`,
			kind, stamp).Scan(&l.Count, &l.Max); err != nil {
			return nil, err
		}
		if l.Count == 0 {
			continue
		}
		percentile := func(p float64) (int64, error) {
			offset := int(float64(l.Count-1) * p)
			var ms int64
			err := s.db.QueryRow(
				`SELECT ms FROM events WHERE kind = ? AND ms > 0 AND at >= ?
				 ORDER BY ms LIMIT 1 OFFSET ?`, kind, stamp, offset).Scan(&ms)
			return ms, err
		}
		var err error
		if l.P50, err = percentile(0.50); err != nil {
			return nil, err
		}
		if l.P95, err = percentile(0.95); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// Quality is how good the output has been, not just how much of it there
// was. Read from the meta the generate events already carry.
type Quality struct {
	Generations   int     `json:"generations"`
	AvgFit        float64 `json:"avgFit"`
	AvgFill       float64 `json:"avgFill"`
	TrimmedShare  float64 `json:"trimmedShare"`
	WarnedShare   float64 `json:"warnedShare"`
	ShortPageRate float64 `json:"shortPageRate"`
}

func (s *Store) QualitySince(since time.Time) (Quality, error) {
	var q Quality
	var avgFit, avgFill, trimmed, warned, short sql.NullFloat64

	err := s.db.QueryRow(
		`SELECT COUNT(*),
		        AVG(json_extract(meta, '$.fit')),
		        AVG(json_extract(meta, '$.fill')),
		        AVG(CASE WHEN json_extract(meta, '$.trimmed') > 0 THEN 1.0 ELSE 0.0 END),
		        AVG(CASE WHEN json_extract(meta, '$.warnings') > 0 THEN 1.0 ELSE 0.0 END),
		        AVG(CASE WHEN json_extract(meta, '$.fill') < 0.9 THEN 1.0 ELSE 0.0 END)
		 FROM events WHERE kind = ? AND ok = 1 AND at >= ?`,
		EventGenerate, since.UTC().Format(time.RFC3339Nano),
	).Scan(&q.Generations, &avgFit, &avgFill, &trimmed, &warned, &short)
	if err != nil {
		return q, err
	}
	q.AvgFit = avgFit.Float64
	q.AvgFill = avgFill.Float64
	q.TrimmedShare = trimmed.Float64
	q.WarnedShare = warned.Float64
	q.ShortPageRate = short.Float64
	return q, nil
}

// SlowRequests returns the slowest recent requests with their ids, so a
// complaint about "it hung" can be matched to a specific line in the log.
func (s *Store) SlowRequests(since time.Time, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(
		`SELECT id, at, COALESCE(user_id, 0), kind, target, ms, ok, detail, COALESCE(meta, '')
		 FROM events WHERE kind = ? AND at >= ? ORDER BY ms DESC LIMIT ?`,
		EventRequest, since.UTC().Format(time.RFC3339Nano), limit)
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

// LLMByProvider groups model calls by who served them, which is the number
// that matters when a provider is swapped or a fallback fires.
func (s *Store) LLMByProvider(since time.Time) ([]Count, error) {
	rows, err := s.db.Query(
		`SELECT COALESCE(json_extract(meta, '$.provider'), 'unknown'),
		        COUNT(*), SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END),
		        COALESCE(CAST(AVG(ms) AS INTEGER), 0)
		 FROM events WHERE kind = ? AND at >= ? GROUP BY 1 ORDER BY COUNT(*) DESC`,
		EventLLM, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCounts(rows)
}

// ErrorRateByDay is the failure share per day, so a bad deploy shows as a
// step rather than being averaged away across the window.
func (s *Store) ErrorRateByDay(days int) ([]Count, error) {
	if days <= 0 || days > 90 {
		days = 14
	}
	since := time.Now().UTC().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)

	rows, err := s.db.Query(
		`SELECT substr(at, 1, 10) AS day, COUNT(*), SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END), 0
		 FROM events WHERE at >= ? GROUP BY day ORDER BY day`,
		since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found, err := scanCounts(rows)
	if err != nil {
		return nil, err
	}
	byDay := map[string]Count{}
	for _, c := range found {
		byDay[c.Label] = c
	}

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
