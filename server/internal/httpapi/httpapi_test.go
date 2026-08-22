package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"cvx/internal/ai"
	"cvx/internal/auth"
	"cvx/internal/model"
	"cvx/internal/pdfgen"
	"cvx/internal/store"
)

// fakeLLM returns a canned digitize response when given a PDF block, and a
// canned tailor response otherwise. The tailor fixture cites item-0 /
// item-0-b-0, matching the ids model.AssignIDs deterministically assigns to
// the single item in the digitize fixture, so model.ValidateTailored passes.
// tailorOut overrides the default tailor JSON when set, so tests can probe
// alternate LLM response shapes (e.g. explicit empty arrays).
type fakeLLM struct {
	tailorOut   string
	coverOut    string
	extendOut   string
	digitizeOut string
	classifyOut string
	skillsOut   string
}

const digitizeJSON = `{"isResume":true,"notResumeReason":"","name":"Ada","email":"a@e.com","phone":"","location":"","summary":"","links":[],"skills":["Python","Go"],
	"items":[{"kind":"experience","title":"Engineer","organization":"AE","startDate":"2021-01","endDate":"","bullets":[{"text":"Built engine","skills":["Python"]}]}]}`

const tailorJSON = `{"targetRole":"Python Backend Engineer","headline":"h","summary":"s","selectedSkills":["Python"],
	"sections":[{"title":"Experience","items":[{"sourceId":"item-0","title":"Engineer","organization":"AE","dates":"2021 - Present",
	"bullets":[{"sourceBulletId":"item-0-b-0","text":"Built the engine in Python"}]}]}],
	"gaps":[{"requirement":"Django","evidence":"not in profile","severity":"missing"}],"whatChanged":["led with Python"]}`

// tailorJSONEmptyArrays mirrors tailorJSON but with gaps/whatChanged as
// explicit empty JSON arrays, to check they round-trip as [] (not null).
const tailorJSONEmptyArrays = `{"targetRole":"Python Backend Engineer","headline":"h","summary":"s","selectedSkills":["Python"],
	"sections":[{"title":"Experience","items":[{"sourceId":"item-0","title":"Engineer","organization":"AE","dates":"2021 - Present",
	"bullets":[{"sourceBulletId":"item-0-b-0","text":"Built the engine in Python"}]}]}],
	"gaps":[],"whatChanged":[]}`

const coverJSON = `{"recruiterName":"","company":"","tone":"neutral","paragraphs":["I am applying for the Python Backend Engineer role. At Analytical Engines Co I built the core computation engine in Python, designing the service layer that carried every production workload and cutting batch processing time for the largest datasets.","That work maps directly onto what this role asks for. I wrote the first published algorithm for the engine, owned its correctness under load, and would bring the same care for measurable outcomes to your backend systems."],"closing":"Sincerely,"}`

// extendJSON adds one new skill and one new item, so a happy-path test can
// assert both itemCount and skillCount grow. "Rust" is deliberately not
// already in digitizeJSON's skills (["Python","Go"]) so it isn't deduped away.
const extendJSON = `{"useful":true,"notUsefulReason":"","newSkills":["Rust"],"newItems":[{"kind":"project","title":"Side project","organization":"","startDate":"2024-01","endDate":"","bullets":[{"text":"Built a CLI tool","skills":["Rust"]}]}],"bulletAdditions":[]}`

// extendUnknownIDJSON references an item id that cannot exist in a freshly
// digitized profile (which only ever has item-0), driving the 502 branch of
// postProfileExtend via model.MergeAdditions' guardrail.
const extendUnknownIDJSON = `{"useful":true,"notUsefulReason":"","newSkills":[],"newItems":[],"bulletAdditions":[{"itemId":"item-99","bullets":[{"text":"x","skills":[]}]}]}`

// isExtendSchema reports whether schema is the ai package's profile-additions
// schema (shape-tested: its top-level properties include "newSkills"), as
// opposed to the tailor or cover-letter schemas.
func isExtendSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = props["newSkills"]
	return ok
}

// isCoverLetterSchema reports whether schema is the ai package's cover
// letter schema (shape-tested: paragraphs, and no subject line, which is
// what separates it from the recruiter email), as opposed to the tailor
// schema. fakeLLM uses this to route calls that don't carry a PDF block
// (i.e. everything except Digitize) between the tailor and cover-letter
// fixtures, since GenerateJSON's other parameters don't otherwise
// distinguish the two calls.
func isCoverLetterSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	if _, hasSubject := props["subject"]; hasSubject {
		return false
	}
	_, ok = props["paragraphs"]
	return ok
}

// isProfileSchema reports whether schema is the ai package's profile schema
// (shape-tested: its top-level properties include "isResume").
func isProfileSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = props["isResume"]
	return ok
}

// isClassifySchema reports whether schema is the ai package's job-input
// verdict schema (shape-tested: its top-level properties include "usable").
func isClassifySchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = props["usable"]
	return ok
}

// isSkillsVerdictSchema reports whether schema is the ai package's skill
// verdict schema (shape-tested: its top-level properties include "verdicts").
func isSkillsVerdictSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = props["verdicts"]
	return ok
}

func (f fakeLLM) GenerateJSON(_ context.Context, _ string, blocks []ai.ContentBlock, schema map[string]any) ([]byte, error) {
	for _, b := range blocks {
		if b.PDF != nil {
			if f.digitizeOut != "" {
				return []byte(f.digitizeOut), nil
			}
			return []byte(digitizeJSON), nil
		}
	}
	// The text intake path carries no PDF block, so route on the schema:
	// the digitize fixture is the profile-shaped answer either way.
	if isProfileSchema(schema) {
		if f.digitizeOut != "" {
			return []byte(f.digitizeOut), nil
		}
		return []byte(digitizeJSON), nil
	}
	if isClassifySchema(schema) {
		if f.classifyOut != "" {
			return []byte(f.classifyOut), nil
		}
		return []byte(`{"usable":true,"reason":""}`), nil
	}
	if isSkillsVerdictSchema(schema) {
		if f.skillsOut != "" {
			return []byte(f.skillsOut), nil
		}
		return []byte(`{"verdicts":[]}`), nil
	}
	if isCoverLetterSchema(schema) {
		if f.coverOut != "" {
			return []byte(f.coverOut), nil
		}
		return []byte(coverJSON), nil
	}
	if isExtendSchema(schema) {
		if f.extendOut != "" {
			return []byte(f.extendOut), nil
		}
		return []byte(extendJSON), nil
	}
	if isRecruiterEmailSchema(schema) {
		return []byte(recruiterJSON), nil
	}
	if f.tailorOut != "" {
		return []byte(f.tailorOut), nil
	}
	return []byte(tailorJSON), nil
}

const recruiterJSON = `{"subject":"Application for Python Backend Engineer","paragraphs":["I am applying for the Python Backend Engineer role. At Analytical Engines Co I built the core computation engine in Python and wrote its first published algorithm. My resume and the details are attached."],"closing":"Best regards,"}`

func isRecruiterEmailSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = props["subject"]
	return ok
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cvx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// devAuth builds a dev-mode Auth (auto-authenticates every request as a
// fixed test user), matching how CVX_DEV_USER works in real local dev — the
// data-flow tests in this file aren't testing auth itself, they just need a
// consistently-authenticated caller so the /api group's middleware doesn't
// reject them.
func devAuth(st *store.Store) *auth.Auth {
	return &auth.Auth{Store: st, DevUserEmail: "dev@test.local"}
}

// devUserID upserts (or fetches) the same dev-mode user devAuth's middleware
// would resolve every request to, so tests that write directly to the store
// (bypassing HTTP) can scope those writes to the user the HTTP layer will
// later query as.
func devUserID(t *testing.T, st *store.Store) int64 {
	t.Helper()
	u, _, err := st.UpsertUser("dev", "dev@test.local", "dev@test.local", "dev@test.local")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// newTestServerAs builds a Server+Echo pair sharing st but authenticated (in
// dev mode) as a distinct email, so isolation tests can drive two "users"
// against the same underlying database the way two real signed-in sessions
// would.
func newTestServerAs(t *testing.T, st *store.Store, email string) (*Server, *echo.Echo) {
	t.Helper()
	s := &Server{
		Store:  st,
		LLM:    fakeLLM{},
		Mail:   func(string, model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
		Auth:   &auth.Auth{Store: st, DevUserEmail: email},
		Events: &Recorder{Store: st},
	}
	e := echo.New()
	s.Register(e)
	return s, e
}

func newTestServerWithLLM(t *testing.T, llm ai.LLM) (*Server, *echo.Echo) {
	t.Helper()
	st := newStore(t)
	s := &Server{
		Store:  st,
		LLM:    llm,
		Mail:   func(string, model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
		Auth:   devAuth(st),
		Events: &Recorder{Store: st},
	}
	e := echo.New()
	s.Register(e)
	return s, e
}

func generateRequestBodyWithCover(role string, cover bool) *http.Request {
	body, _ := json.Marshal(generateRequest{RoleInput: role, CoverLetter: cover})
	req := httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return req
}

func newTestServer(t *testing.T) (*Server, *echo.Echo) {
	t.Helper()
	return newTestServerWithLLM(t, fakeLLM{})
}

func uploadRequest(t *testing.T, pdf []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "resume.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(pdf); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/profile", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	return req
}

func extendRequestBody(note string) *http.Request {
	body, _ := json.Marshal(extendRequest{Note: note})
	req := httptest.NewRequest(http.MethodPost, "/api/profile/extend", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return req
}

func generateRequestBody(role string) *http.Request {
	body, _ := json.Marshal(generateRequest{RoleInput: role})
	req := httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return req
}

// TestLogoutUnguardedEvenWithoutValidSession checks the real Register
// wiring (not just the auth package's own unit test) puts /api/logout
// outside the auth-guarded /api group: an OAuth-mode server with no session
// cookie at all must still let logout succeed (204), never 401.
func TestLogoutUnguardedEvenWithoutValidSession(t *testing.T) {
	st := newStore(t)
	s := &Server{
		Store: st,
		LLM:   fakeLLM{},
		Mail:  func(string, model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
		Auth:  &auth.Auth{Store: st, SessionSecret: "test-session-secret-at-least-32-chars-long"}, // OAuth mode, no cookie
	}
	e := echo.New()
	s.Register(e)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/logout", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	// A guarded route in the same server must still 401 without a session,
	// proving this isn't accidentally-open auth, just logout specifically.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for guarded route, got %d: %s", rec.Code, rec.Body)
	}
}

func TestProfileNotFoundThenUpload(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, uploadRequest(t, []byte("%PDF-fake")))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ada" || got.ItemCount != 1 || got.SkillCount != 2 {
		t.Fatalf("got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 after upload, got %d", rec.Code)
	}
}

func TestUploadMissingFile(t *testing.T) {
	_, e := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/profile", strings.NewReader("not multipart"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEMultipartForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestUploadTooLarge(t *testing.T) {
	_, e := newTestServer(t)
	big := bytes.Repeat([]byte("a"), maxUploadBytes+1)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, uploadRequest(t, big))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d: %s", rec.Code, rec.Body)
	}
}

func TestGenerateBeforeProfile(t *testing.T) {
	_, e := newTestServer(t)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body)
	}
}

func TestGenerateEmptyRoleInput(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake"))) // seed profile

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("   "))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestGenerateHappyPathAndPDF(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, uploadRequest(t, []byte("%PDF-fake")))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed upload failed: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Filename == "" || got.Emailed != false || len(got.Gaps) == 0 || len(got.WhatChanged) == 0 {
		t.Fatalf("got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var list []store.GenerationMeta
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != got.ID {
		t.Fatalf("got %+v", list)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+got.ID+"/pdf", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); ct != "application/pdf" {
		t.Fatalf("want application/pdf, got %q", ct)
	}
	wantDisp := fmt.Sprintf(`attachment; filename="%s"`, got.Filename)
	if cd := rec.Header().Get(echo.HeaderContentDisposition); cd != wantDisp {
		t.Fatalf("got %q want %q", cd, wantDisp)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty pdf body")
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/nope/pdf", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestGenerateEmailFailureDoesNotFailRequest(t *testing.T) {
	s, e := newTestServer(t)
	s.Mail = func(string, model.Tailored, []byte, string, []byte, string) (bool, error) {
		return false, fmt.Errorf("boom")
	}
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 despite mail error, got %d: %s", rec.Code, rec.Body)
	}
	var got generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Emailed {
		t.Fatal("want emailed=false on mail error")
	}
}

// TestGenerateResponseEmptyArraysStayArrays exercises the LLM returning
// literal "gaps":[] / "whatChanged":[] and checks the /api/generate response
// body serializes them as [] (regression guard alongside the nil case below).
func TestGenerateResponseEmptyArraysStayArrays(t *testing.T) {
	_, e := newTestServerWithLLM(t, fakeLLM{tailorOut: tailorJSONEmptyArrays})
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"gaps":[]`) || !strings.Contains(body, `"whatChanged":[]`) {
		t.Fatalf("want gaps/whatChanged as [], got %s", body)
	}
	if strings.Contains(body, `"gaps":null`) || strings.Contains(body, `"whatChanged":null`) {
		t.Fatalf("gaps/whatChanged must never be null, got %s", body)
	}
}

// failingLLM always returns err from GenerateJSON, regardless of the blocks
// given, so it drives the digitize/tailor 502 branches directly.
type failingLLM struct{ err error }

func (f failingLLM) GenerateJSON(_ context.Context, _ string, _ []ai.ContentBlock, _ map[string]any) ([]byte, error) {
	return nil, f.err
}

// TestUploadDigitizeFailureSurfacesUnderlyingError checks that when the LLM
// fails during digitize, /api/profile returns 502 and the JSON error body
// contains the underlying error text (not a generic message), since the web
// UI now surfaces that text as a diagnostic detail line.
func TestUploadDigitizeFailureSurfacesUnderlyingError(t *testing.T) {
	_, e := newTestServerWithLLM(t, failingLLM{err: fmt.Errorf("rate limited by provider")})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, uploadRequest(t, []byte("%PDF-fake")))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got["error"], "rate limited by provider") {
		t.Fatalf("want error to contain underlying text, got %+v", got)
	}
}

// TestGenerateTailorFailureSurfacesUnderlyingError is the same check for the
// /api/generate tailor branch.
func TestGenerateTailorFailureSurfacesUnderlyingError(t *testing.T) {
	s, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake"))) // seed profile with the happy-path fakeLLM
	s.LLM = failingLLM{err: fmt.Errorf("model returned invalid schema")}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got["error"], "model returned invalid schema") {
		t.Fatalf("want error to contain underlying text, got %+v", got)
	}
}

// TestGenerateWithURLRoleInputFetchesText points roleInput at an httptest
// server serving a job posting page; postGenerate should fetch and use the
// extracted text instead of treating the URL itself as the role input. The
// fakeLLM ignores prompt content for the tailor call, so this only proves
// the pre-step ran without erroring rather than what text it extracted.
func TestGenerateWithURLRoleInputFetchesText(t *testing.T) {
	jobSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>Python Backend Engineer</h1><p>Build things with Python.</p></body></html>`))
	}))
	defer jobSrv.Close()

	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody(jobSrv.URL))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
}

// TestGenerateWithURLRoleInputFetchFailureReturns502 points roleInput at a
// URL that 404s; postGenerate should surface a 502 with a "fetch job
// posting: ..." detail rather than passing the raw URL to the tailor LLM.
func TestGenerateWithURLRoleInputFetchFailureReturns502(t *testing.T) {
	jobSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer jobSrv.Close()

	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody(jobSrv.URL))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got["error"], "fetch job posting: ") {
		t.Fatalf("want error prefixed with 'fetch job posting: ', got %+v", got)
	}
}

// TestGenerationsListNilSlicesSerializeAsEmptyArrays seeds a generation with
// nil Gaps/WhatChanged directly via store.SaveGeneration (bypassing the LLM
// entirely, so it exercises the store-layer nil-guard) and checks the
// /api/generations response body never contains null for those fields.
func TestGenerationsListNilSlicesSerializeAsEmptyArrays(t *testing.T) {
	st := newStore(t)
	s := &Server{
		Store:  st,
		LLM:    fakeLLM{},
		Mail:   func(string, model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
		Auth:   devAuth(st),
		Events: &Recorder{Store: st},
	}
	e := echo.New()
	s.Register(e)

	ta := model.Tailored{TargetRole: "X", Gaps: nil, WhatChanged: nil}
	if _, err := st.SaveGeneration(devUserID(t, st), ta, []byte("pdf-bytes"), "x.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"gaps":[]`) || !strings.Contains(body, `"whatChanged":[]`) {
		t.Fatalf("want gaps/whatChanged as [], got %s", body)
	}
	if strings.Contains(body, `"gaps":null`) || strings.Contains(body, `"whatChanged":null`) {
		t.Fatalf("gaps/whatChanged must never be null, got %s", body)
	}
}

func TestGapsEndpointShape(t *testing.T) {
	st := newStore(t)
	s := &Server{
		Store:  st,
		LLM:    fakeLLM{},
		Mail:   func(string, model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
		Auth:   devAuth(st),
		Events: &Recorder{Store: st},
	}
	e := echo.New()
	s.Register(e)

	userID := devUserID(t, st)
	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }
	if _, err := st.SaveGeneration(userID, mk(model.Gap{Requirement: "Django", Evidence: "e1", Severity: "missing"}), []byte("p"), "a.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveGeneration(userID, mk(model.Gap{Requirement: "django", Evidence: "e2", Severity: "missing"}), []byte("p"), "b.pdf", nil, "", ""); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gaps", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Total  int `json:"total"`
		Trends []struct {
			Requirement  string `json:"requirement"`
			Count        int    `json:"count"`
			Missing      int    `json:"missing"`
			Weak         int    `json:"weak"`
			LastEvidence string `json:"lastEvidence"`
		} `json:"trends"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || len(got.Trends) != 1 {
		t.Fatalf("want total 2, 1 trend, got %+v", got)
	}
	if got.Trends[0].Count != 2 || got.Trends[0].Missing != 2 {
		t.Fatalf("want count 2 missing 2, got %+v", got.Trends[0])
	}
}

func TestGapsEndpointEmptyTrendsIsArrayNotNull(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gaps", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"trends":[]`) {
		t.Fatalf("want trends:[], got %s", body)
	}
	if strings.Contains(body, `"trends":null`) {
		t.Fatalf("trends must never be null, got %s", body)
	}
}

// TestGenerateWithCoverLetterHappyPath checks that coverLetter:true in the
// request produces both a resume and a cover letter: the response reports
// coverLetter=true with a non-empty coverFilename, and the cover download
// route serves a PDF.
func TestGenerateWithCoverLetterHappyPath(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBodyWithCover("Python Backend Engineer", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.CoverLetter || got.CoverFilename == "" {
		t.Fatalf("want cover letter generated, got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+got.ID+"/cover", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); ct != "application/pdf" {
		t.Fatalf("want application/pdf, got %q", ct)
	}
	wantDisp := fmt.Sprintf(`attachment; filename="%s"`, got.CoverFilename)
	if cd := rec.Header().Get(echo.HeaderContentDisposition); cd != wantDisp {
		t.Fatalf("got %q want %q", cd, wantDisp)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty cover pdf body")
	}
}

// TestGenerateWithoutCoverLetterNoCoverRoute checks that a request without
// coverLetter (the default/backward-compatible shape) reports coverLetter:
// false with no coverFilename, and the cover download route 404s for that
// generation id.
func TestGenerateWithoutCoverLetterNoCoverRoute(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CoverLetter || got.CoverFilename != "" {
		t.Fatalf("want no cover letter, got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+got.ID+"/cover", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body)
	}
}

func TestExtendProfileBeforeProfile(t *testing.T) {
	_, e := newTestServer(t)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, extendRequestBody("shipped v2"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body)
	}
}

func TestExtendProfileEmptyNote(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake"))) // seed profile

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, extendRequestBody("   "))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

// TestExtendProfileHappyPath checks that a successful extend grows both
// itemCount (a new item) and skillCount (a new skill), matching the summary
// shape returned by GET /api/profile and POST /api/profile.
func TestExtendProfileHappyPath(t *testing.T) {
	_, e := newTestServer(t)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, uploadRequest(t, []byte("%PDF-fake")))
	var seeded profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &seeded); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, extendRequestBody("Also built a CLI tool in Rust on the side"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ItemCount != seeded.ItemCount+1 {
		t.Fatalf("want itemCount to grow by 1, got %d -> %d", seeded.ItemCount, got.ItemCount)
	}
	if got.SkillCount != seeded.SkillCount+1 {
		t.Fatalf("want skillCount to grow by 1, got %d -> %d", seeded.SkillCount, got.SkillCount)
	}

	// The extend must have persisted: a fresh GET reflects the same counts.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	var reloaded profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded != got {
		t.Fatalf("want persisted profile to match response, got %+v vs %+v", reloaded, got)
	}
}

// TestExtendProfileUnknownItemIDReturns502 drives model.MergeAdditions'
// guardrail (a bulletAddition referencing an item id the profile does not
// have) through the HTTP layer: it must surface as 502, not succeed or panic,
// and the stored profile must be left untouched.
func TestExtendProfileUnknownItemIDReturns502(t *testing.T) {
	_, e := newTestServerWithLLM(t, fakeLLM{extendOut: extendUnknownIDJSON})
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, extendRequestBody("references a role that isn't in the profile"))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got["error"], "item-99") {
		t.Fatalf("want error naming item-99, got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	var afterFailure profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &afterFailure); err != nil {
		t.Fatal(err)
	}
	if afterFailure.ItemCount != 1 || afterFailure.SkillCount != 2 {
		t.Fatalf("want profile unchanged after failed extend, got %+v", afterFailure)
	}
}

// coverFailingLLM behaves like the happy-path fakeLLM for digitize/tailor
// calls but returns an error for the cover-letter-shaped schema, so tests
// can drive the "cover letter failed" branch in postGenerate without
// touching the resume path.
type coverFailingLLM struct{}

func (coverFailingLLM) GenerateJSON(_ context.Context, _ string, blocks []ai.ContentBlock, schema map[string]any) ([]byte, error) {
	for _, b := range blocks {
		if b.PDF != nil {
			return []byte(digitizeJSON), nil
		}
	}
	if isCoverLetterSchema(schema) {
		return nil, fmt.Errorf("cover letter boom")
	}
	if isClassifySchema(schema) {
		return []byte(`{"usable":true,"reason":""}`), nil
	}
	return []byte(tailorJSON), nil
}

// TestGenerateCoverLetterFailureDoesNotFailGenerate checks that when cover
// letter generation fails, /api/generate still succeeds with the resume:
// status 200, a valid generation id, coverLetter=false, and no coverFilename.
func TestGenerateCoverLetterFailureDoesNotFailGenerate(t *testing.T) {
	_, e := newTestServerWithLLM(t, coverFailingLLM{})
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBodyWithCover("Python Backend Engineer", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 despite cover failure, got %d: %s", rec.Code, rec.Body)
	}
	var got generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CoverLetter || got.CoverFilename != "" {
		t.Fatalf("want no cover letter reported on failure, got %+v", got)
	}
	if got.ID == "" || got.Filename == "" {
		t.Fatalf("want resume generation to still succeed, got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+got.ID+"/cover", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for cover pdf after cover failure, got %d", rec.Code)
	}
}

// TestGenerateEmailsBothPDFsWhenCoverExists checks that when coverLetter is
// requested and succeeds, the Mail closure is invoked with both the resume
// pdf and the cover pdf/filename (non-nil, non-empty) so the wiring in
// main.go can attach both to the outgoing email.
func TestGenerateEmailsBothPDFsWhenCoverExists(t *testing.T) {
	s, e := newTestServer(t)
	var gotPDF, gotCoverPDF []byte
	var gotTo, gotFilename, gotCoverFilename string
	s.Mail = func(to string, _ model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error) {
		gotTo, gotPDF, gotFilename, gotCoverPDF, gotCoverFilename = to, pdf, filename, coverPDF, coverFilename
		return true, nil
	}
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBodyWithCover("Python Backend Engineer", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	if len(gotPDF) == 0 || gotFilename == "" {
		t.Fatalf("want resume pdf/filename passed to Mail, got %d bytes, filename %q", len(gotPDF), gotFilename)
	}
	if len(gotCoverPDF) == 0 || gotCoverFilename == "" {
		t.Fatalf("want cover pdf/filename passed to Mail, got %d bytes, filename %q", len(gotCoverPDF), gotCoverFilename)
	}
	if gotTo != "dev@test.local" {
		t.Fatalf("want recipient dev@test.local (the signed-in user's email), got %q", gotTo)
	}
}

// TestDataIsolatedPerUser checks per-user scoping end-to-end at the HTTP
// layer: two dev-mode "users" sharing one store never see each other's
// profile or generations. User A uploads a profile and generates a resume
// (with a cover letter); user B must see no profile (404), an empty
// generations list, empty gaps, and 404s downloading A's PDF/cover PDF by
// id. User B then uploads their own profile and confirms it — not A's — is
// what GET /api/profile returns for them.
func TestDataIsolatedPerUser(t *testing.T) {
	st := newStore(t)
	_, eA := newTestServerAs(t, st, "userA@test.local")
	_, eB := newTestServerAs(t, st, "userB@test.local")

	rec := httptest.NewRecorder()
	eA.ServeHTTP(rec, uploadRequest(t, []byte("%PDF-fake")))
	if rec.Code != http.StatusOK {
		t.Fatalf("userA seed upload failed: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	eA.ServeHTTP(rec, generateRequestBodyWithCover("Python Backend Engineer", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("userA generate failed: %d %s", rec.Code, rec.Body)
	}
	var genA generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &genA); err != nil {
		t.Fatal(err)
	}

	// User B has no profile of their own yet.
	rec = httptest.NewRecorder()
	eB.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for userB's own profile, got %d: %s", rec.Code, rec.Body)
	}

	// User B's generations list must not include userA's generation.
	rec = httptest.NewRecorder()
	eB.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var listB []store.GenerationMeta
	if err := json.Unmarshal(rec.Body.Bytes(), &listB); err != nil {
		t.Fatal(err)
	}
	if len(listB) != 0 {
		t.Fatalf("want empty generations list for userB, got %+v", listB)
	}

	// User B's gaps must be empty even though userA's generation has gaps.
	rec = httptest.NewRecorder()
	eB.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gaps", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"total":0`) || !strings.Contains(body, `"trends":[]`) {
		t.Fatalf("want empty gaps for userB, got %s", body)
	}

	// User B cannot download userA's generation PDF or cover PDF by id.
	rec = httptest.NewRecorder()
	eB.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+genA.ID+"/pdf", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for userB downloading userA's pdf, got %d: %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	eB.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+genA.ID+"/cover", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for userB downloading userA's cover pdf, got %d: %s", rec.Code, rec.Body)
	}

	// User A still sees their own data throughout.
	rec = httptest.NewRecorder()
	eA.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	var listA []store.GenerationMeta
	if err := json.Unmarshal(rec.Body.Bytes(), &listA); err != nil {
		t.Fatal(err)
	}
	if len(listA) != 1 || listA[0].ID != genA.ID {
		t.Fatalf("want userA's own generation still visible, got %+v", listA)
	}

	// User B uploads their own profile: it must be independent of userA's.
	rec = httptest.NewRecorder()
	eB.ServeHTTP(rec, uploadRequest(t, []byte("%PDF-fake")))
	if rec.Code != http.StatusOK {
		t.Fatalf("userB upload failed: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	eB.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for userB's own profile after upload, got %d: %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	eA.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want userA's profile unaffected by userB's upload, got %d: %s", rec.Code, rec.Body)
	}
}

// TestHandlersGuardMissingUserIDContext exercises the defensive 401 branch
// each handler carries for the case where it runs without the auth
// middleware having set a userID in context — a path that should be
// unreachable through Register's real routing (every /api/* route sits
// behind Auth.Middleware) but is guarded anyway. Calling the handler
// directly, bypassing Register/Middleware entirely, is the only way to
// exercise it.
func TestHandlersGuardMissingUserIDContext(t *testing.T) {
	st := newStore(t)
	s := &Server{
		Store:  st,
		LLM:    fakeLLM{},
		Mail:   func(string, model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
		Auth:   devAuth(st),
		Events: &Recorder{Store: st},
	}
	e := echo.New()

	newContext := func(req *http.Request) (echo.Context, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		return e.NewContext(req, rec), rec
	}

	c, rec := newContext(httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if err := s.getProfile(c); err != nil {
		t.Fatalf("want handler to write the 401 response directly, got error %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", rec.Code, rec.Body)
	}

	c, rec = newContext(httptest.NewRequest(http.MethodGet, "/api/generations/some-id/pdf", nil))
	c.SetParamNames("id")
	c.SetParamValues("some-id")
	if err := s.getGenerationPDF(c); err != nil {
		t.Fatalf("want handler to write the 401 response directly, got error %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", rec.Code, rec.Body)
	}
}

func TestDeleteGenerationRoute(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("generate: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/generations/"+got.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+got.ID+"/pdf", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("pdf after delete: want 404, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/generations/"+got.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: want 404, got %d", rec.Code)
	}
}

func TestSettingsRoutes(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get defaults: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		ResumeStyle pdfgen.Style `json:"resumeStyle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ResumeStyle != pdfgen.DefaultStyle() {
		t.Fatalf("want defaults, got %+v", got.ResumeStyle)
	}

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	if rec := put(`{"resumeStyle":{"theme":"neon","accent":"#2244D9","density":"normal","skillsFirst":false}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad theme: want 400, got %d: %s", rec.Code, rec.Body)
	}

	if rec := put(`{"resumeStyle":{"theme":"compact","accent":"#B07818","density":"tight","skillsFirst":true}}`); rec.Code != http.StatusOK {
		t.Fatalf("valid put: want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := pdfgen.Style{Theme: "compact", Accent: "#B07818", Density: "tight", SkillsFirst: true}
	if got.ResumeStyle != want {
		t.Fatalf("round trip: want %+v, got %+v", want, got.ResumeStyle)
	}
}

func TestSettingsPreview(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/preview", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("preview without profile: want 404, got %d", rec.Code)
	}

	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/preview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: want 200, got %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); ct != "application/pdf" {
		t.Fatalf("want application/pdf, got %q", ct)
	}
	if cd := rec.Header().Get(echo.HeaderContentDisposition); !strings.Contains(cd, "inline") {
		t.Fatalf("want inline disposition, got %q", cd)
	}
	if rec.Body.Len() < 1000 {
		t.Fatalf("implausible preview pdf: %d bytes", rec.Body.Len())
	}
}

func TestExtendWithContext(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	body, _ := json.Marshal(extendRequest{Note: "I used Django on an internal tool", Context: "Django experience — not in profile"})
	req := httptest.NewRequest(http.MethodPost, "/api/profile/extend", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("extend with context: want 200, got %d: %s", rec.Code, rec.Body)
	}
}

func TestGenerateRecruiterEmail(t *testing.T) {
	s, e := newTestServer(t)
	var gotSubject, gotTo string
	notified := false
	s.Mail = func(to string, t model.Tailored, pdf []byte, fn string, cp []byte, cf string) (bool, error) {
		notified = true
		return true, nil
	}
	s.RecruiterMail = func(to string, re model.RecruiterEmail, name string, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error) {
		gotTo, gotSubject = to, re.Subject
		return true, nil
	}
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	body, _ := json.Marshal(generateRequest{RoleInput: "Python Backend Engineer", RecruiterEmail: true})
	req := httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Emailed        bool `json:"emailed"`
		RecruiterEmail bool `json:"recruiterEmail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Emailed || !resp.RecruiterEmail {
		t.Fatalf("want emailed+recruiterEmail true, got %+v", resp)
	}
	if notified {
		t.Fatal("notification must not send when recruiter email requested")
	}
	if gotTo == "" || gotSubject != "Application for Python Backend Engineer" {
		t.Fatalf("recruiter mail call: to=%q subject=%q", gotTo, gotSubject)
	}
}

func TestGenerateWithoutRecruiterFlagUsesNotification(t *testing.T) {
	s, e := newTestServer(t)
	notified, recruited := false, false
	s.Mail = func(to string, t model.Tailored, pdf []byte, fn string, cp []byte, cf string) (bool, error) {
		notified = true
		return true, nil
	}
	s.RecruiterMail = func(to string, re model.RecruiterEmail, name string, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error) {
		recruited = true
		return true, nil
	}
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	if !notified || recruited {
		t.Fatalf("want notification only: notified=%v recruited=%v", notified, recruited)
	}
}

// TestGenerateRejectsMashInput drives the heuristic gate: obvious keyboard
// mash gets 422 with the fixed job-input message and never reaches the LLM.
func TestGenerateRejectsMashInput(t *testing.T) {
	_, e := newTestServerWithLLM(t, failingLLM{})
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("wgfwjhfkjhsdfkjh"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for mash input, got %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "job ad or a role") {
		t.Fatalf("want fixed job-input copy, got %s", rec.Body)
	}
}

// TestGenerateRejectsNonJobInput drives the classifier gate: fluent text the
// model judges unusable gets 422 with the fixed copy, not a resume.
func TestGenerateRejectsNonJobInput(t *testing.T) {
	_, e := newTestServerWithLLM(t, fakeLLM{classifyOut: `{"usable":false,"reason":"grocery list"}`})
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("eggs milk bread and a dozen apples"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for non-job input, got %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "job ad or a role") {
		t.Fatalf("want fixed job-input copy, got %s", rec.Body)
	}
}

// TestUploadRejectsNonResumePDF drives the digitize gate: a PDF the model
// says is not a resume gets 422 and no profile is saved.
func TestUploadRejectsNonResumePDF(t *testing.T) {
	notResume := `{"isResume":false,"notResumeReason":"it is an invoice","name":"","email":"","phone":"","location":"","summary":"","links":[],"skills":[],"items":[]}`
	_, e := newTestServerWithLLM(t, fakeLLM{digitizeOut: notResume})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, uploadRequest(t, []byte("%PDF-fake")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for non-resume PDF, got %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "read like a resume") {
		t.Fatalf("want fixed resume copy, got %s", rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want no profile saved after rejected upload, got %d", rec.Code)
	}
}

// TestExtendRejectsMashNote drives the heuristic gate on the extend note.
func TestExtendRejectsMashNote(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, extendRequestBody("qqqqqqqq"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for mash note, got %d: %s", rec.Code, rec.Body)
	}
}

// TestExtendRejectsUselessNote drives the LLM gate: a note the model marks
// not useful gets 422 and the profile is left untouched.
func TestExtendRejectsUselessNote(t *testing.T) {
	useless := `{"useful":false,"notUsefulReason":"just a greeting","newSkills":[],"newItems":[],"bulletAdditions":[]}`
	_, e := newTestServerWithLLM(t, fakeLLM{extendOut: useless})
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, extendRequestBody("hello there how is it going"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for useless note, got %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "about your experience") {
		t.Fatalf("want fixed note copy, got %s", rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	var after profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.ItemCount != 1 || after.SkillCount != 2 {
		t.Fatalf("want profile unchanged after rejected note, got %+v", after)
	}
}

// TestSkillsRoundTrip covers GET/PUT /api/profile/skills: read after upload,
// replace with a reordered+deduped list, and reject when no profile exists.
func TestSkillsRoundTrip(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile/skills", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 before profile, got %d", rec.Code)
	}

	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile/skills", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Python") {
		t.Fatalf("want skills from digitized profile, got %d: %s", rec.Code, rec.Body)
	}

	put := httptest.NewRequest(http.MethodPut, "/api/profile/skills",
		strings.NewReader(`{"skills":["Go","Rust","  ","go","TypeScript"]}`))
	put.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, put)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 on put, got %d: %s", rec.Code, rec.Body)
	}
	var got map[string][]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"Go", "Rust", "TypeScript"}
	if len(got["skills"]) != len(want) {
		t.Fatalf("want %v, got %v", want, got["skills"])
	}
	for i, s := range want {
		if got["skills"][i] != s {
			t.Fatalf("want %v, got %v", want, got["skills"])
		}
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	var after profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.SkillCount != 3 {
		t.Fatalf("want skillCount 3 after put, got %+v", after)
	}
}

// TestSkillsRejectsOversizedSkill drives putSkills' length validation.
func TestSkillsRejectsOversizedSkill(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	long := strings.Repeat("x", 61)
	put := httptest.NewRequest(http.MethodPut, "/api/profile/skills",
		strings.NewReader(`{"skills":["`+long+`"]}`))
	put.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, put)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for oversized skill, got %d: %s", rec.Code, rec.Body)
	}
}

// TestSkillsRejectsGibberishAddition: a fluent-looking non-skill the model
// rejects gets 422 naming it, and the profile keeps its old list.
func TestSkillsRejectsGibberishAddition(t *testing.T) {
	_, e := newTestServerWithLLM(t, fakeLLM{skillsOut: `{"verdicts":[{"skill":"ewigiuwegf","valid":false}]}`})
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	put := httptest.NewRequest(http.MethodPut, "/api/profile/skills",
		strings.NewReader(`{"skills":["Python","Go","ewigiuwegf"]}`))
	put.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, put)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for gibberish skill, got %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "ewigiuwegf") {
		t.Fatalf("want message naming the skill, got %s", rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	var after profileSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.SkillCount != 2 {
		t.Fatalf("want profile unchanged after rejected skill, got %+v", after)
	}
}

// TestSkillsRejectsMalformedAddition drives the heuristic layer: bad
// characters get 422 before any LLM call (failingLLM would 502 otherwise).
func TestSkillsRejectsMalformedAddition(t *testing.T) {
	st := newStore(t)
	s := &Server{Store: st, LLM: failingLLM{}, Auth: devAuth(st)}
	e := echo.New()
	s.Register(e)
	if err := st.SaveProfile(devUserID(t, st), model.Profile{Skills: []string{"Python"}}); err != nil {
		t.Fatal(err)
	}

	put := httptest.NewRequest(http.MethodPut, "/api/profile/skills",
		strings.NewReader(`{"skills":["Python","@@@@"]}`))
	put.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, put)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for malformed skill, got %d: %s", rec.Code, rec.Body)
	}
}

// TestSkillsReorderSkipsLLM: reordering existing skills must never call the
// model — failingLLM would 502 if it did.
func TestSkillsReorderSkipsLLM(t *testing.T) {
	st := newStore(t)
	s := &Server{Store: st, LLM: failingLLM{}, Auth: devAuth(st)}
	e := echo.New()
	s.Register(e)
	if err := st.SaveProfile(devUserID(t, st), model.Profile{Skills: []string{"Python", "Go"}}); err != nil {
		t.Fatal(err)
	}

	put := httptest.NewRequest(http.MethodPut, "/api/profile/skills",
		strings.NewReader(`{"skills":["Go","Python"]}`))
	put.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, put)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for reorder without LLM, got %d: %s", rec.Code, rec.Body)
	}
}

// TestGenerationStatus covers the status endpoint: set, list, clear, reject.
func TestGenerationStatus(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))
	e.ServeHTTP(httptest.NewRecorder(), generateRequestBody("Python Backend Engineer"))

	var rows []store.GenerationMeta
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("want one generation, got %s (%v)", rec.Body, err)
	}
	id := rows[0].ID

	putStatus := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/generations/"+id+"/status", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	if rec := putStatus(`{"status":"interviewing"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	json.Unmarshal(rec.Body.Bytes(), &rows)
	if rows[0].Status != "interviewing" || rows[0].StatusAt == "" {
		t.Fatalf("want status recorded with timestamp, got %+v", rows[0])
	}

	if rec := putStatus(`{"status":"ghosted"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unknown status, got %d", rec.Code)
	}
	if rec := putStatus(`{"status":""}`); rec.Code != http.StatusNoContent {
		t.Fatalf("want clearing to succeed, got %d", rec.Code)
	}
}

// TestProfileRestore: an extend changes the profile; restore brings the
// previous version back, and restoring again toggles forward.
func TestProfileRestore(t *testing.T) {
	_, e := newTestServer(t)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/profile/restore", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 with no history, got %d", rec.Code)
	}

	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))
	e.ServeHTTP(httptest.NewRecorder(), extendRequestBody("I shipped a CLI tool in Rust at my last role"))

	summary := func() profileSummary {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
		var s profileSummary
		if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := summary(); got.SkillCount != 3 {
		t.Fatalf("want extended profile (3 skills), got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/profile/restore", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: want 200, got %d: %s", rec.Code, rec.Body)
	}
	if got := summary(); got.SkillCount != 2 {
		t.Fatalf("want pre-extend profile back (2 skills), got %+v", got)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/profile/restore", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("second restore: want 200, got %d", rec.Code)
	}
	if got := summary(); got.SkillCount != 3 {
		t.Fatalf("want restore to toggle forward (3 skills), got %+v", got)
	}
}

// TestEditGeneration: edits round-trip through validate/normalize/re-render;
// fabricated ids are rejected.
func TestEditGeneration(t *testing.T) {
	_, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))
	e.ServeHTTP(httptest.NewRecorder(), generateRequestBody("Python Backend Engineer"))

	var rows []store.GenerationMeta
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	json.Unmarshal(rec.Body.Bytes(), &rows)
	id := rows[0].ID

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+id+"/tailored", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get tailored: want 200, got %d", rec.Code)
	}
	var ta model.Tailored
	if err := json.Unmarshal(rec.Body.Bytes(), &ta); err != nil {
		t.Fatal(err)
	}

	ta.Headline = "Edited headline"
	ta.Sections[0].Items[0].Bullets[0].Text = "Rebuilt the engine end to end in Python"
	body, _ := json.Marshal(ta)
	req := httptest.NewRequest(http.MethodPut, "/api/generations/"+id, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit: want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+id+"/tailored", nil))
	var after model.Tailored
	json.Unmarshal(rec.Body.Bytes(), &after)
	if after.Headline != "Edited headline" || after.Sections[0].Items[0].Bullets[0].Text != "Rebuilt the engine end to end in Python" {
		t.Fatalf("edit did not persist: %+v", after)
	}

	// The guardrail still holds: citing an id the profile doesn't have is 422.
	ta.Sections[0].Items[0].SourceID = "item-99"
	body, _ = json.Marshal(ta)
	req = httptest.NewRequest(http.MethodPut, "/api/generations/"+id, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for fabricated id, got %d: %s", rec.Code, rec.Body)
	}
}

// TestDeleteAccount wipes everything the user owns and ends the session;
// another user's data is untouched.
func TestDeleteAccount(t *testing.T) {
	st := newStore(t)
	_, e := newTestServerAs(t, st, "a@test.local")
	_, other := newTestServerAs(t, st, "b@test.local")

	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))
	e.ServeHTTP(httptest.NewRecorder(), generateRequestBody("Python Backend Engineer"))
	other.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/account", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	// Dev-mode auth re-creates the user on the next request, but their data
	// must be gone.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want profile gone after delete, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations", nil))
	if !strings.Contains(rec.Body.String(), "[]") {
		t.Fatalf("want no generations after delete, got %s", rec.Body)
	}

	rec = httptest.NewRecorder()
	other.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("other user's profile must survive, got %d", rec.Code)
	}
}

func TestGenerationProvenanceTracesEveryBulletBackToTheProfile(t *testing.T) {
	s, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	if rec.Code != http.StatusOK {
		t.Fatalf("generate: %d %s", rec.Code, rec.Body)
	}
	var gen generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &gen); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+gen.ID+"/provenance", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("provenance: %d %s", rec.Code, rec.Body)
	}
	var got map[string]provenanceEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	entry, ok := got["item-0-b-0"]
	if !ok {
		t.Fatalf("cited bullet missing from provenance: %v", got)
	}
	if entry.Original == "" || entry.ItemTitle == "" {
		t.Fatalf("provenance entry is empty: %+v", entry)
	}
	_ = s
}

// The follow-up flow is gated by the tracker, not by the user's patience.
func TestFollowUpRefusesUntilTheApplicationIsSentAndOldEnough(t *testing.T) {
	s, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	var gen generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &gen); err != nil {
		t.Fatal(err)
	}

	followUp := func() int {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/generations/"+gen.ID+"/followup", nil))
		return rec.Code
	}

	// Not sent at all.
	if code := followUp(); code != http.StatusConflict {
		t.Fatalf("want 409 for an application that was never sent, got %d", code)
	}

	// Sent just now: still too soon.
	if _, err := s.Store.SetGenerationStatus(devUserID(t, s.Store), gen.ID, store.StatusSent); err != nil {
		t.Fatal(err)
	}
	if code := followUp(); code != http.StatusConflict {
		t.Fatalf("want 409 for an application sent today, got %d", code)
	}
}

func TestPreviewRendersWithoutSaving(t *testing.T) {
	s, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	var gen generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &gen); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+gen.ID+"/tailored", nil))
	var tailored model.Tailored
	if err := json.Unmarshal(rec.Body.Bytes(), &tailored); err != nil {
		t.Fatal(err)
	}

	edited := tailored.Clone()
	edited.Headline = "A headline only the preview has seen"
	body, _ := json.Marshal(edited)
	req := httptest.NewRequest(http.MethodPost, "/api/generations/"+gen.ID+"/preview", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}

	var preview previewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	pdf, err := base64.StdEncoding.DecodeString(preview.PDF)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("preview did not return a PDF: %v", err)
	}
	if preview.Fill <= 0 || preview.Fill > 1.2 {
		t.Fatalf("implausible fill: %v", preview.Fill)
	}

	// Nothing was stored: the saved copy still has the old headline.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+gen.ID+"/tailored", nil))
	var after model.Tailored
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Headline == edited.Headline {
		t.Fatal("preview wrote to the stored generation")
	}
	_ = s
}

func TestAvailableContentOffersBackWhatTheResumeDoesNotUse(t *testing.T) {
	s, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, generateRequestBody("Python Backend Engineer"))
	var gen generateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &gen); err != nil {
		t.Fatal(err)
	}

	// Strip the resume down to one bullet, so the rest of the profile is
	// unused and must be offered back.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+gen.ID+"/tailored", nil))
	var tailored model.Tailored
	if err := json.Unmarshal(rec.Body.Bytes(), &tailored); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generations/"+gen.ID+"/available", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("available: %d %s", rec.Code, rec.Body)
	}
	var avail availableResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &avail); err != nil {
		t.Fatal(err)
	}
	if len(avail.Skills) == 0 {
		t.Fatal("the profile's skills should be offered for the skills row")
	}
	for _, item := range avail.Items {
		if item.OnResume && len(item.Bullets) == 0 {
			t.Fatalf("an item with nothing left to add should not be listed: %+v", item)
		}
	}
	_ = s
}

func TestPutProfileEditsTheFactsAndLeavesHistoryAlone(t *testing.T) {
	s, e := newTestServer(t)
	e.ServeHTTP(httptest.NewRecorder(), uploadRequest(t, []byte("%PDF-fake")))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile/edits", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("profile edits: %d %s", rec.Code, rec.Body)
	}
	var edits model.ProfileEdits
	if err := json.Unmarshal(rec.Body.Bytes(), &edits); err != nil {
		t.Fatal(err)
	}

	before, err := s.Store.LoadProfile(devUserID(t, s.Store))
	if err != nil || before == nil {
		t.Fatal(err)
	}

	edits.Name = "Ada Byron"
	edits.LinkedIn = "https://linkedin.com/in/ada"
	edits.Languages = []string{"English"}
	edits.Certifications = []model.Certification{{Name: "AWS Solutions Architect"}}

	body, _ := json.Marshal(edits)
	req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put profile: %d %s", rec.Code, rec.Body)
	}

	after, err := s.Store.LoadProfile(devUserID(t, s.Store))
	if err != nil || after == nil {
		t.Fatal(err)
	}
	if after.Name != "Ada Byron" || len(after.Languages) != 1 || len(after.Certifications) != 1 {
		t.Fatalf("edits lost: %+v", after)
	}
	if len(after.Items) != len(before.Items) {
		t.Fatalf("history changed: %d items, was %d", len(after.Items), len(before.Items))
	}

	// A profile with no name is not a profile.
	edits.Name = "  "
	body, _ = json.Marshal(edits)
	req = httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a nameless profile, got %d", rec.Code)
	}
}

// The path for people who have never had a CV: prose in, profile out.
func TestProfileFromTextAndFollowUpQuestions(t *testing.T) {
	s, e := newTestServer(t)

	body, _ := json.Marshal(map[string]string{
		"text": "I have been a backend engineer at Venix for the last two years, mostly Python and Postgres, and I built their payment service.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/profile/text", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile from text: %d %s", rec.Code, rec.Body)
	}

	stored, err := s.Store.LoadProfile(devUserID(t, s.Store))
	if err != nil || stored == nil {
		t.Fatalf("no profile saved: %v", err)
	}
	if len(stored.Items) == 0 {
		t.Fatal("the text produced no items")
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile/questions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("questions: %d %s", rec.Code, rec.Body)
	}
	var got struct {
		Questions []model.Question `json:"questions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// A profile this thin has holes worth asking about.
	if len(got.Questions) == 0 {
		t.Fatalf("want follow-up questions for a thin profile: %+v", stored)
	}
}

// Too little to work with is refused before it reaches the model. Longer
// nonsense is deliberately left to the model: quality.Mash is lenient by
// design, and a fluent-looking string is not something a regex should judge.
func TestProfileFromTextRefusesNothing(t *testing.T) {
	_, e := newTestServer(t)
	for _, text := range []string{"", "hi", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		body, _ := json.Marshal(map[string]string{"text": text})
		req := httptest.NewRequest(http.MethodPost, "/api/profile/text", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("%q should not have produced a profile", text)
		}
	}
}

// Text the model judges to carry no career content comes back as a 422 with
// copy a person can act on, not a stack trace.
func TestProfileFromTextRefusesNonCareerText(t *testing.T) {
	st := newStore(t)
	s := &Server{
		Store:  st,
		LLM:    fakeLLM{digitizeOut: `{"isResume":false,"notResumeReason":"this is a recipe","name":"","email":"","phone":"","location":"","summary":"","links":[],"certifications":[],"languages":[],"interests":[],"skills":[],"items":[]}`},
		Auth:   devAuth(st),
		Events: &Recorder{Store: st},
	}
	e := echo.New()
	s.Register(e)

	body, _ := json.Marshal(map[string]string{
		"text": "Combine the flour and the butter, then bake for forty minutes at one eighty.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/profile/text", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "your work") {
		t.Fatalf("the message should tell them what is wrong: %s", rec.Body)
	}
}

// Questions never fail on a signed-in account with no profile yet.
func TestProfileQuestionsWithoutAProfile(t *testing.T) {
	_, e := newTestServer(t)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/profile/questions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
}
