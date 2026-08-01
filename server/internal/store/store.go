package store

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"cvx/internal/model"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS profile (id INTEGER PRIMARY KEY CHECK (id = 1), json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS generations (
  id TEXT PRIMARY KEY, target_role TEXT NOT NULL, filename TEXT NOT NULL,
  created_at TEXT NOT NULL, tailored_json TEXT NOT NULL, pdf BLOB NOT NULL
);
`

type Store struct{ db *sql.DB }

type GenerationMeta struct {
	ID          string
	TargetRole  string
	Filename    string
	CreatedAt   string
	Gaps        []model.Gap
	WhatChanged []string
}

// Open opens the SQLite database at path and applies the schema migration.
// A single connection is used (SetMaxOpenConns(1)) since modernc.org/sqlite
// serializes writes anyway and this avoids SQLITE_BUSY errors under
// concurrent access from this single-user tool.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveProfile(p model.Profile) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO profile (id, json, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET json = excluded.json, updated_at = excluded.updated_at`,
		string(b), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) LoadProfile() (*model.Profile, error) {
	var raw string
	err := s.db.QueryRow(`SELECT json FROM profile WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p model.Profile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slug(role string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(role), "-"), "-")
}

func (s *Store) SaveGeneration(t model.Tailored, pdf []byte, filename string) (GenerationMeta, error) {
	id := time.Now().UTC().Format("20060102T150405.000") + "-" + slug(t.TargetRole)
	b, err := json.Marshal(t)
	if err != nil {
		return GenerationMeta{}, err
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`INSERT INTO generations (id, target_role, filename, created_at, tailored_json, pdf) VALUES (?, ?, ?, ?, ?, ?)`,
		id, t.TargetRole, filename, createdAt, string(b), pdf,
	)
	if err != nil {
		return GenerationMeta{}, err
	}
	return GenerationMeta{
		ID:          id,
		TargetRole:  t.TargetRole,
		Filename:    filename,
		CreatedAt:   createdAt,
		Gaps:        t.Gaps,
		WhatChanged: t.WhatChanged,
	}, nil
}

func (s *Store) ListGenerations() ([]GenerationMeta, error) {
	rows, err := s.db.Query(
		`SELECT id, target_role, filename, created_at, tailored_json FROM generations ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GenerationMeta
	for rows.Next() {
		var m GenerationMeta
		var raw string
		if err := rows.Scan(&m.ID, &m.TargetRole, &m.Filename, &m.CreatedAt, &raw); err != nil {
			return nil, err
		}
		var t model.Tailored
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			return nil, err
		}
		m.Gaps = t.Gaps
		m.WhatChanged = t.WhatChanged
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetGenerationPDF(id string) (pdf []byte, filename string, err error) {
	err = s.db.QueryRow(`SELECT pdf, filename FROM generations WHERE id = ?`, id).Scan(&pdf, &filename)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return pdf, filename, nil
}
