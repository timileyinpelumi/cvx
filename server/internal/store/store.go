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
  created_at TEXT NOT NULL, tailored_json TEXT NOT NULL, pdf BLOB NOT NULL,
  cover_pdf BLOB, cover_filename TEXT
);
`

type Store struct{ db *sql.DB }

type GenerationMeta struct {
	ID             string      `json:"id"`
	TargetRole     string      `json:"targetRole"`
	Filename       string      `json:"filename"`
	CreatedAt      string      `json:"createdAt"`
	Gaps           []model.Gap `json:"gaps"`
	WhatChanged    []string    `json:"whatChanged"`
	HasCoverLetter bool        `json:"hasCoverLetter"`
}

// Open opens the SQLite database at path, applies the schema migration, and
// then runs migrate to backfill any columns added since the database was
// first created. A single connection is used (SetMaxOpenConns(1)) since
// modernc.org/sqlite serializes writes anyway and this avoids SQLITE_BUSY
// errors under concurrent access from this single-user tool.
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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrate adds columns introduced after the initial v1 schema to a
// generations table that predates them, so an existing v1 database (created
// before cover letters existed) picks up the new columns in place without
// losing data. Idempotent: it checks PRAGMA table_info before each ALTER
// TABLE, so running it on every Open (including against a freshly created
// database, which already has the columns via the schema constant above) is
// always safe.
func migrate(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(generations)`)
	if err != nil {
		return err
	}
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if !cols["cover_pdf"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN cover_pdf BLOB`); err != nil {
			return err
		}
	}
	if !cols["cover_filename"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN cover_filename TEXT`); err != nil {
			return err
		}
	}
	return nil
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

// SaveGeneration persists a tailored resume and its rendered PDF, plus an
// optional cover letter PDF alongside it. coverPDF == nil (or coverFilename
// == "") means no cover letter was generated for this run; both are stored
// as SQL NULL in that case rather than empty-but-present values.
func (s *Store) SaveGeneration(t model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (GenerationMeta, error) {
	id := time.Now().UTC().Format("20060102T150405.000") + "-" + slug(t.TargetRole)
	b, err := json.Marshal(t)
	if err != nil {
		return GenerationMeta{}, err
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)

	var coverFilenameArg any
	if coverFilename != "" {
		coverFilenameArg = coverFilename
	}
	var coverPDFArg any
	if coverPDF != nil {
		coverPDFArg = coverPDF
	}

	_, err = s.db.Exec(
		`INSERT INTO generations (id, target_role, filename, created_at, tailored_json, pdf, cover_pdf, cover_filename) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, t.TargetRole, filename, createdAt, string(b), pdf, coverPDFArg, coverFilenameArg,
	)
	if err != nil {
		return GenerationMeta{}, err
	}
	return GenerationMeta{
		ID:             id,
		TargetRole:     t.TargetRole,
		Filename:       filename,
		CreatedAt:      createdAt,
		Gaps:           model.NonNil(t.Gaps),
		WhatChanged:    model.NonNil(t.WhatChanged),
		HasCoverLetter: coverFilename != "",
	}, nil
}

func (s *Store) ListGenerations() ([]GenerationMeta, error) {
	rows, err := s.db.Query(
		`SELECT id, target_role, filename, created_at, tailored_json, cover_filename FROM generations ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GenerationMeta
	for rows.Next() {
		var m GenerationMeta
		var raw string
		var coverFilename sql.NullString
		if err := rows.Scan(&m.ID, &m.TargetRole, &m.Filename, &m.CreatedAt, &raw, &coverFilename); err != nil {
			return nil, err
		}
		var t model.Tailored
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			return nil, err
		}
		m.Gaps = model.NonNil(t.Gaps)
		m.WhatChanged = model.NonNil(t.WhatChanged)
		m.HasCoverLetter = coverFilename.Valid && coverFilename.String != ""
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

// GetGenerationCoverPDF returns the cover letter PDF for id, or (nil, "",
// nil) when the generation has no cover letter (including when id itself
// doesn't exist).
func (s *Store) GetGenerationCoverPDF(id string) (pdf []byte, filename string, err error) {
	var coverPDF []byte
	var coverFilename sql.NullString
	err = s.db.QueryRow(`SELECT cover_pdf, cover_filename FROM generations WHERE id = ?`, id).Scan(&coverPDF, &coverFilename)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if !coverFilename.Valid || coverFilename.String == "" {
		return nil, "", nil
	}
	return coverPDF, coverFilename.String, nil
}
