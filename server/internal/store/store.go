package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
  cover_pdf BLOB, cover_filename TEXT, role_fingerprint TEXT,
  pinned INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT '', status_at TEXT
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT, provider TEXT NOT NULL, provider_id TEXT NOT NULL,
  email TEXT NOT NULL, name TEXT NOT NULL, created_at TEXT NOT NULL,
  UNIQUE(provider, provider_id)
);
CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER PRIMARY KEY,
  resume_style TEXT NOT NULL,
  email_copy INTEGER NOT NULL DEFAULT 1,
  generation TEXT NOT NULL DEFAULT '{}',
  recruiter_auto INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS profile_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL,
  json TEXT NOT NULL, saved_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS llm_cache (
  key TEXT PRIMARY KEY, response BLOB NOT NULL, created_at TEXT NOT NULL
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
	Pinned         bool        `json:"pinned"`
	Status         string      `json:"status"`
	StatusAt       string      `json:"statusAt"`
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
	if !genCols["role_fingerprint"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN role_fingerprint TEXT`); err != nil {
			return err
		}
	}
	if !genCols["pinned"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !genCols["status"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN status TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !genCols["status_at"] {
		if _, err := db.Exec(`ALTER TABLE generations ADD COLUMN status_at TEXT`); err != nil {
			return err
		}
	}

	settingsCols, err := tableColumns(db, "user_settings")
	if err != nil {
		return err
	}
	if !settingsCols["email_copy"] {
		if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN email_copy INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	if !settingsCols["generation"] {
		if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN generation TEXT NOT NULL DEFAULT '{}'`); err != nil {
			return err
		}
	}
	if !settingsCols["recruiter_auto"] {
		if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN recruiter_auto INTEGER NOT NULL DEFAULT 0`); err != nil {
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

// DeleteUser permanently removes the user and everything they own, in one
// transaction. The LLM cache is left alone: it is keyed by request content,
// not user.
func (s *Store) DeleteUser(userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM generations WHERE user_id = ?`,
		`DELETE FROM profile_history WHERE user_id = ?`,
		`DELETE FROM profile WHERE user_id = ?`,
		`DELETE FROM user_settings WHERE user_id = ?`,
		`DELETE FROM users WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// llmCacheTTL bounds how long a cached LLM response is served; PruneLLMCache
// removes older rows on startup.
const llmCacheTTL = 14 * 24 * time.Hour

// GetLLMCache returns the cached response for key, or (nil, false) on a miss
// or an expired row. Errors degrade to a miss: the cache must never take the
// pipeline down.
func (s *Store) GetLLMCache(key string) ([]byte, bool) {
	var resp []byte
	var createdAt string
	err := s.db.QueryRow(`SELECT response, created_at FROM llm_cache WHERE key = ?`, key).Scan(&resp, &createdAt)
	if err != nil {
		return nil, false
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil || time.Since(t) > llmCacheTTL {
		return nil, false
	}
	return resp, true
}

// PutLLMCache stores response under key, replacing any previous row. Errors
// are dropped for the same reason GetLLMCache degrades to a miss.
func (s *Store) PutLLMCache(key string, response []byte) {
	s.db.Exec(
		`INSERT INTO llm_cache (key, response, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET response = excluded.response, created_at = excluded.created_at`,
		key, response, time.Now().UTC().Format(time.RFC3339),
	)
}

// PruneLLMCache deletes rows past the TTL. Called once from main on startup.
func (s *Store) PruneLLMCache() {
	cutoff := time.Now().UTC().Add(-llmCacheTTL).Format(time.RFC3339)
	s.db.Exec(`DELETE FROM llm_cache WHERE created_at < ?`, cutoff)
}

// profileHistoryCap bounds snapshots per user; older ones are dropped.
const profileHistoryCap = 20

func (s *Store) SaveProfile(userID int64, p model.Profile) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}

	// Snapshot the outgoing profile first so any write (upload, extend,
	// skills edit) can be undone with RestoreProfile.
	var current string
	if err := s.db.QueryRow(`SELECT json FROM profile WHERE user_id = ?`, userID).Scan(&current); err == nil {
		if err := s.snapshotProfile(userID, current); err != nil {
			return err
		}
	} else if err != sql.ErrNoRows {
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO profile (user_id, json, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET json = excluded.json, updated_at = excluded.updated_at`,
		userID, string(b), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) snapshotProfile(userID int64, profileJSON string) error {
	// Millisecond precision so rapid successive writes keep their order.
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	if _, err := s.db.Exec(
		`INSERT INTO profile_history (user_id, json, saved_at) VALUES (?, ?, ?)`,
		userID, profileJSON, now,
	); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`DELETE FROM profile_history WHERE user_id = ? AND id NOT IN
		 (SELECT id FROM profile_history WHERE user_id = ? ORDER BY id DESC LIMIT ?)`,
		userID, userID, profileHistoryCap,
	)
	return err
}

// ProfileHistoryInfo reports how many snapshots the user has and when the
// newest one was taken ("" when none).
func (s *Store) ProfileHistoryInfo(userID int64) (int, string, error) {
	var count int
	var last sql.NullString
	err := s.db.QueryRow(
		`SELECT COUNT(*), MAX(saved_at) FROM profile_history WHERE user_id = ?`, userID,
	).Scan(&count, &last)
	if err != nil {
		return 0, "", err
	}
	return count, last.String, nil
}

// RestoreProfile swaps the newest snapshot back in as the live profile. The
// replaced profile is snapshotted first, so restoring twice toggles between
// the two most recent states rather than walking further into history.
func (s *Store) RestoreProfile(userID int64) (*model.Profile, error) {
	var histID int64
	var histJSON string
	err := s.db.QueryRow(
		`SELECT id, json FROM profile_history WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID,
	).Scan(&histID, &histJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var p model.Profile
	if err := json.Unmarshal([]byte(histJSON), &p); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM profile_history WHERE id = ?`, histID); err != nil {
		return nil, err
	}
	// SaveProfile snapshots the current live profile before overwriting it.
	if err := s.SaveProfile(userID, p); err != nil {
		return nil, err
	}
	return &p, nil
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

const suffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func idSuffix() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("0405.000")
	}
	for i := range b {
		b[i] = suffixAlphabet[int(b[i])%len(suffixAlphabet)]
	}
	return string(b)
}

// generationID is the user-visible handle (it appears in URLs and filenames),
// so it reads as the role, not a timestamp: "backend-engineer-x7k2".
func generationID(role string) string {
	s := slug(role)
	if s == "" {
		s = "resume"
	}
	return s + "-" + idSuffix()
}

// RoleFingerprint normalizes a raw role input (the pasted text or link,
// before any fetching) into a dedupe key: URLs are canonicalized (fragment
// and trailing slash dropped), text is lowercased with whitespace collapsed.
func RoleFingerprint(input string) string {
	s := strings.TrimSpace(input)
	if strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://") {
		if i := strings.IndexByte(s, '#'); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimRight(s, "/")
	} else {
		s = strings.Join(strings.Fields(strings.ToLower(s)), " ")
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// SaveGeneration persists a tailored resume and its rendered PDF, plus an
// optional cover letter PDF alongside it. coverPDF == nil (or coverFilename
// == "") means no cover letter was generated for this run; both are stored
// as SQL NULL in that case rather than empty-but-present values.
//
// A non-empty fingerprint dedupes by job input: when the user already has a
// generation for the same fingerprint, that row is refreshed in place (same
// id, so history links keep working) instead of a new row piling up.
func (s *Store) SaveGeneration(userID int64, t model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string, fingerprint string) (GenerationMeta, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return GenerationMeta{}, err
	}
	// Millisecond precision: created_at is the sort key (ids are random-suffixed
	// slugs), and same-second saves must still order.
	createdAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")

	var coverFilenameArg any
	if coverFilename != "" {
		coverFilenameArg = coverFilename
	}
	var coverPDFArg any
	if coverPDF != nil {
		coverPDFArg = coverPDF
	}
	meta := func(id string) GenerationMeta {
		return GenerationMeta{
			ID:             id,
			TargetRole:     t.TargetRole,
			Filename:       filename,
			CreatedAt:      createdAt,
			Gaps:           model.NonNil(t.Gaps),
			WhatChanged:    model.NonNil(t.WhatChanged),
			HasCoverLetter: coverFilename != "",
		}
	}

	if fingerprint != "" {
		var existingID string
		err := s.db.QueryRow(
			`SELECT id FROM generations WHERE user_id = ? AND role_fingerprint = ?`,
			userID, fingerprint,
		).Scan(&existingID)
		if err == nil {
			_, err = s.db.Exec(
				`UPDATE generations SET target_role = ?, filename = ?, created_at = ?, tailored_json = ?, pdf = ?, cover_pdf = ?, cover_filename = ? WHERE id = ?`,
				t.TargetRole, filename, createdAt, string(b), pdf, coverPDFArg, coverFilenameArg, existingID,
			)
			if err != nil {
				return GenerationMeta{}, err
			}
			return meta(existingID), nil
		}
		if err != sql.ErrNoRows {
			return GenerationMeta{}, err
		}
	}

	id := generationID(t.TargetRole)
	var fingerprintArg any
	if fingerprint != "" {
		fingerprintArg = fingerprint
	}
	// The 4-char suffix can collide on the same role slug; regenerate and retry.
	for attempt := 0; ; attempt++ {
		_, err = s.db.Exec(
			`INSERT INTO generations (id, target_role, filename, created_at, tailored_json, pdf, cover_pdf, cover_filename, user_id, role_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, t.TargetRole, filename, createdAt, string(b), pdf, coverPDFArg, coverFilenameArg, userID, fingerprintArg,
		)
		if err == nil {
			break
		}
		if attempt < 3 && strings.Contains(err.Error(), "UNIQUE constraint failed: generations.id") {
			id = generationID(t.TargetRole)
			continue
		}
		return GenerationMeta{}, err
	}
	return meta(id), nil
}

func (s *Store) ListGenerations(userID int64) ([]GenerationMeta, error) {
	rows, err := s.db.Query(
		`SELECT id, target_role, filename, created_at, tailored_json, cover_filename, pinned, status, status_at FROM generations WHERE user_id = ? ORDER BY pinned DESC, created_at DESC, id DESC`,
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
		var coverFilename, statusAt sql.NullString
		var pinned int
		if err := rows.Scan(&m.ID, &m.TargetRole, &m.Filename, &m.CreatedAt, &raw, &coverFilename, &pinned, &m.Status, &statusAt); err != nil {
			return nil, err
		}
		m.Pinned = pinned != 0
		m.StatusAt = statusAt.String
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

// GetGenerationParams returns the user's saved writing knobs JSON, or
// (nil, nil) when they have never saved settings.
func (s *Store) GetGenerationParams(userID int64) ([]byte, error) {
	var raw string
	err := s.db.QueryRow(`SELECT generation FROM user_settings WHERE user_id = ?`, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

func (s *Store) SaveGenerationParams(userID int64, params []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO user_settings (user_id, resume_style, generation) VALUES (?, '{}', ?)
		 ON CONFLICT(user_id) DO UPDATE SET generation = excluded.generation`,
		userID, string(params),
	)
	return err
}

// GetRecruiterAuto reports whether every compose should request the
// forwardable recruiter email by default. Defaults to false.
func (s *Store) GetRecruiterAuto(userID int64) (bool, error) {
	var v int
	err := s.db.QueryRow(`SELECT recruiter_auto FROM user_settings WHERE user_id = ?`, userID).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

func (s *Store) SaveRecruiterAuto(userID int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO user_settings (user_id, resume_style, recruiter_auto) VALUES (?, '{}', ?)
		 ON CONFLICT(user_id) DO UPDATE SET recruiter_auto = excluded.recruiter_auto`,
		userID, v,
	)
	return err
}

// GetEmailCopy reports whether the user wants each generated resume emailed
// to them. Defaults to true for users who have never saved settings.
func (s *Store) GetEmailCopy(userID int64) (bool, error) {
	var v int
	err := s.db.QueryRow(`SELECT email_copy FROM user_settings WHERE user_id = ?`, userID).Scan(&v)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

// SaveEmailCopy stores the email-copy preference, creating the settings row
// (with the default style JSON) when the user has never saved settings.
func (s *Store) SaveEmailCopy(userID int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO user_settings (user_id, resume_style, email_copy) VALUES (?, '{}', ?)
		 ON CONFLICT(user_id) DO UPDATE SET email_copy = excluded.email_copy`,
		userID, v,
	)
	return err
}

// GenerationStatuses is the set SetGenerationStatus accepts; "" clears the
// status.
var GenerationStatuses = map[string]bool{"": true, "sent": true, "interviewing": true, "rejected": true, "offer": true}

// GetGenerationTailored returns the stored tailored content for one
// generation owned by userID; ok=false when no such row.
func (s *Store) GetGenerationTailored(userID int64, id string) (model.Tailored, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT tailored_json FROM generations WHERE id = ? AND user_id = ?`, id, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return model.Tailored{}, false, nil
	}
	if err != nil {
		return model.Tailored{}, false, err
	}
	var t model.Tailored
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return model.Tailored{}, false, err
	}
	return t, true, nil
}

// UpdateGenerationContent replaces one generation's tailored content and
// rendered PDF after an edit, keeping id, filename, cover letter, pin, and
// status. Reports whether a row matched.
func (s *Store) UpdateGenerationContent(userID int64, id string, t model.Tailored, pdf []byte) (bool, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return false, err
	}
	res, err := s.db.Exec(
		`UPDATE generations SET target_role = ?, tailored_json = ?, pdf = ? WHERE id = ? AND user_id = ?`,
		t.TargetRole, string(b), pdf, id, userID,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetGenerationStatus records where the application stands, stamping when the
// status changed ("" clears both). Reports whether a row matched.
func (s *Store) SetGenerationStatus(userID int64, id string, status string) (bool, error) {
	var statusAt any
	if status != "" {
		statusAt = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.Exec(`UPDATE generations SET status = ?, status_at = ? WHERE id = ? AND user_id = ?`, status, statusAt, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetGenerationPinned pins or unpins one generation owned by userID,
// reporting whether a row matched.
func (s *Store) SetGenerationPinned(userID int64, id string, pinned bool) (bool, error) {
	v := 0
	if pinned {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE generations SET pinned = ? WHERE id = ? AND user_id = ?`, v, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
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
	rows, err := s.db.Query(`SELECT tailored_json FROM generations WHERE user_id = ? ORDER BY created_at ASC, id ASC`, userID)
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
