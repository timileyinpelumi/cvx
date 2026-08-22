package store

import "time"

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
