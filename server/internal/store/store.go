package store

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"cvx/internal/model"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS profile (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER, json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS generations (
  id TEXT PRIMARY KEY, target_role TEXT NOT NULL, filename TEXT NOT NULL,
  created_at TEXT NOT NULL, tailored_json TEXT NOT NULL, pdf BLOB NOT NULL,
  cover_pdf BLOB, cover_filename TEXT
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT, provider TEXT NOT NULL, provider_id TEXT NOT NULL,
  email TEXT NOT NULL, name TEXT NOT NULL, created_at TEXT NOT NULL,
  UNIQUE(provider, provider_id)
);
CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER PRIMARY KEY,
  resume_style TEXT NOT NULL
);
`

type Store struct{ db *sql.DB }

// User is one authenticated identity, tied to a single OAuth provider
// account (or the "dev" pseudo-provider in CVX_DEV_USER mode).
type User struct {
	ID         int64  `json:"id"`
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}

type GenerationMeta struct {
	ID             string      `json:"id"`
	TargetRole     string      `json:"targetRole"`
	Filename       string      `json:"filename"`
	CreatedAt      string      `json:"createdAt"`
	Gaps           []model.Gap `json:"gaps"`
	WhatChanged    []string    `json:"whatChanged"`
	HasCoverLetter bool        `json:"hasCoverLetter"`
}

// GapTrend is one requirement that has come up as a gap across two or more
// generations, aggregated from every generation's tailored_json.
type GapTrend struct {
	Requirement  string `json:"requirement"`
	Count        int    `json:"count"`
	Missing      int    `json:"missing"`
	Weak         int    `json:"weak"`
	LastEvidence string `json:"lastEvidence"`
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

// tableColumns returns the set of column names on table, via PRAGMA
// table_info, so migrate can decide which ALTER TABLE statements are still
// needed.
func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

// migrate adds columns introduced after the initial v1 schema to tables that
// predate them, so an existing database picks up new columns in place
// without losing data. Idempotent: it checks PRAGMA table_info before each
// ALTER TABLE, so running it on every Open (including against a freshly
// created database, which already has the columns via the schema constant
// above) is always safe.
func migrate(db *sql.DB) error {
	genCols, err := tableColumns(db, "generations")
	if err != nil {
		return err
	}
	if !genCols["cover_pdf"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN cover_pdf BLOB`); err != nil {
			return err
		}
	}
	if !genCols["cover_filename"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN cover_filename TEXT`); err != nil {
			return err
		}
	}
	if !genCols["user_id"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN user_id INTEGER`); err != nil {
			return err
		}
	}

	profileCols, err := tableColumns(db, "profile")
	if err != nil {
		return err
	}
	if !profileCols["user_id"] {
		if _, err := db.Exec(`ALTER TABLE profile ADD COLUMN user_id INTEGER`); err != nil {
			return err
		}
	}

	if err := migrateProfileDropSingleRowCheck(db); err != nil {
		return err
	}

	// A UNIQUE index (rather than UNIQUE(user_id) inline on the column) is
	// what makes ON CONFLICT(user_id) in SaveProfile's upsert legal; "IF NOT
	// EXISTS" makes this safe to run on every Open, including against a
	// database that already has it (fresh installs get it from schema
	// creation implicitly via this same call).
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_profile_user_id ON profile(user_id)`); err != nil {
		return err
	}

	return nil
}

// migrateProfileDropSingleRowCheck rebuilds the profile table on databases
// that predate per-user profiles: the original schema pinned id to a single
// row via CHECK (id = 1), which is incompatible with storing one profile row
// per user. Detected via sqlite_master's stored CREATE TABLE text, so this is
// a no-op — safe to run on every Open — once a database has been rebuilt or
// was created fresh with the current schema (which has no CHECK).
func migrateProfileDropSingleRowCheck(db *sql.DB) error {
	var createSQL string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'profile'`).Scan(&createSQL); err != nil {
		return err
	}
	if !strings.Contains(createSQL, "CHECK") {
		return nil
	}
	_, err := db.Exec(`
		CREATE TABLE profile_new (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER, json TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO profile_new (id, user_id, json, updated_at) SELECT id, user_id, json, updated_at FROM profile;
		DROP TABLE profile;
		ALTER TABLE profile_new RENAME TO profile;
	`)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveProfile(userID int64, p model.Profile) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO profile (user_id, json, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET json = excluded.json, updated_at = excluded.updated_at`,
		userID, string(b), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) LoadProfile(userID int64) (*model.Profile, error) {
	var raw string
	err := s.db.QueryRow(`SELECT json FROM profile WHERE user_id = ?`, userID).Scan(&raw)
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
func (s *Store) SaveGeneration(userID int64, t model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (GenerationMeta, error) {
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
		`INSERT INTO generations (id, target_role, filename, created_at, tailored_json, pdf, cover_pdf, cover_filename, user_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, t.TargetRole, filename, createdAt, string(b), pdf, coverPDFArg, coverFilenameArg, userID,
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

func (s *Store) ListGenerations(userID int64) ([]GenerationMeta, error) {
	rows, err := s.db.Query(
		`SELECT id, target_role, filename, created_at, tailored_json, cover_filename FROM generations WHERE user_id = ? ORDER BY id DESC`,
		userID,
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

// GetResumeStyle returns the user's saved resume style JSON, or (nil, nil)
// when they have never saved one.
func (s *Store) GetResumeStyle(userID int64) ([]byte, error) {
	var raw string
	err := s.db.QueryRow(`SELECT resume_style FROM user_settings WHERE user_id = ?`, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

func (s *Store) SaveResumeStyle(userID int64, style []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO user_settings (user_id, resume_style) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET resume_style = excluded.resume_style`,
		userID, string(style),
	)
	return err
}

// DeleteGeneration removes one generation owned by userID, reporting
// whether a row was actually deleted (false for unknown ids and other
// users' generations alike).
func (s *Store) DeleteGeneration(userID int64, id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM generations WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Store) GetGenerationPDF(userID int64, id string) (pdf []byte, filename string, err error) {
	err = s.db.QueryRow(`SELECT pdf, filename FROM generations WHERE id = ? AND user_id = ?`, id, userID).Scan(&pdf, &filename)
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
// doesn't exist, or belongs to a different user).
func (s *Store) GetGenerationCoverPDF(userID int64, id string) (pdf []byte, filename string, err error) {
	var coverPDF []byte
	var coverFilename sql.NullString
	err = s.db.QueryRow(`SELECT cover_pdf, cover_filename FROM generations WHERE id = ? AND user_id = ?`, id, userID).Scan(&coverPDF, &coverFilename)
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

var innerWhitespace = regexp.MustCompile(`\s+`)

// normalizeRequirement is the grouping key for a gap requirement: lowercase,
// trimmed, with runs of inner whitespace collapsed to one space, so gaps
// that differ only in casing or incidental spacing (e.g. "Django" vs
// "django" vs "  Django ") are counted as the same recurring gap.
func normalizeRequirement(s string) string {
	return innerWhitespace.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), " ")
}

// GapSummary aggregates gaps across every stored generation's tailored_json,
// grouped by normalized requirement. It returns the total number of
// generations on record and one GapTrend per requirement that has come up
// as a gap at least twice (a single occurrence is noise, not a trend),
// sorted by Count descending then Requirement ascending. Requirement and
// LastEvidence are taken from the most recent generation in each group.
func (s *Store) GapSummary(userID int64) ([]GapTrend, int, error) {
	rows, err := s.db.Query(`SELECT tailored_json FROM generations WHERE user_id = ? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	type group struct {
		requirement  string
		count        int
		missing      int
		weak         int
		lastEvidence string
	}
	groups := map[string]*group{}
	var order []string
	total := 0

	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		total++

		var t model.Tailored
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			return nil, 0, err
		}
		// A generation whose tailored output lists the same requirement twice
		// (e.g. differing only in casing) must still count as one gap for
		// that generation, so Count never exceeds the number of generations
		// that actually raised it. seenInGen tracks keys already tallied for
		// the current row; severity is taken from the first occurrence.
		seenInGen := map[string]bool{}
		for _, g := range t.Gaps {
			key := normalizeRequirement(g.Requirement)
			if key == "" {
				continue
			}
			gr, ok := groups[key]
			if !ok {
				gr = &group{}
				groups[key] = gr
				order = append(order, key)
			}
			if !seenInGen[key] {
				seenInGen[key] = true
				gr.count++
				switch g.Severity {
				case "missing":
					gr.missing++
				case "weak":
					gr.weak++
				}
			}
			// Iterating oldest to newest, so the last write for this key
			// leaves the requirement casing and evidence from the newest
			// generation that raised it.
			gr.requirement = g.Requirement
			gr.lastEvidence = g.Evidence
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var out []GapTrend
	for _, key := range order {
		gr := groups[key]
		if gr.count < 2 {
			continue
		}
		out = append(out, GapTrend{
			Requirement:  gr.requirement,
			Count:        gr.count,
			Missing:      gr.missing,
			Weak:         gr.weak,
			LastEvidence: gr.lastEvidence,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Requirement < out[j].Requirement
	})

	return out, total, nil
}

// UpsertUser fetches the user identified by (provider, providerID), or
// creates one if none exists yet. Email/name passed on a repeat call for an
// existing user are ignored — the stored identity from first sign-in wins.
//
// The very first user ever created (i.e. the users table was empty before
// this insert) adopts every legacy profile/generations row that predates
// per-user data (user_id IS NULL), so an existing single-user install keeps
// its data when it upgrades to auth. Later users never trigger this: rows
// adopted by the first user are no longer NULL, so the WHERE clause matches
// nothing for anyone after them.
func (s *Store) UpsertUser(provider, providerID, email, name string) (User, error) {
	var existing User
	err := s.db.QueryRow(
		`SELECT id, provider, provider_id, email, name FROM users WHERE provider = ? AND provider_id = ?`,
		provider, providerID,
	).Scan(&existing.ID, &existing.Provider, &existing.ProviderID, &existing.Email, &existing.Name)
	if err == nil {
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return User{}, err
	}

	var userCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return User{}, err
	}
	isFirstUser := userCount == 0

	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO users (provider, provider_id, email, name, created_at) VALUES (?, ?, ?, ?, ?)`,
		provider, providerID, email, name, createdAt,
	)
	if err != nil {
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}

	if isFirstUser {
		if _, err := s.db.Exec(`UPDATE profile SET user_id = ? WHERE user_id IS NULL`, id); err != nil {
			return User{}, err
		}
		if _, err := s.db.Exec(`UPDATE generations SET user_id = ? WHERE user_id IS NULL`, id); err != nil {
			return User{}, err
		}
	}

	return User{ID: id, Provider: provider, ProviderID: providerID, Email: email, Name: name}, nil
}

// GetUser returns the user with the given id, or (nil, nil) if no such user
// exists.
func (s *Store) GetUser(id int64) (*User, error) {
	var u User
	err := s.db.QueryRow(
		`SELECT id, provider, provider_id, email, name FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Provider, &u.ProviderID, &u.Email, &u.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
