package httpapi

import (
	"bytes"
	"context"
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
	"cvx/internal/model"
	"cvx/internal/store"
)

// fakeLLM returns a canned digitize response when given a PDF block, and a
// canned tailor response otherwise. The tailor fixture cites item-0 /
// item-0-b-0, matching the ids model.AssignIDs deterministically assigns to
// the single item in the digitize fixture, so model.ValidateTailored passes.
// tailorOut overrides the default tailor JSON when set, so tests can probe
// alternate LLM response shapes (e.g. explicit empty arrays).
type fakeLLM struct {
	tailorOut string
	coverOut  string
	extendOut string
}

const digitizeJSON = `{"name":"Ada","email":"a@e.com","phone":"","location":"","summary":"","links":[],"skills":["Python","Go"],
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

const coverJSON = `{"greeting":"Dear hiring team,","paragraphs":["I am excited to apply for this role.","My experience aligns well with what you need."],"closing":"Sincerely,"}`

// extendJSON adds one new skill and one new item, so a happy-path test can
// assert both itemCount and skillCount grow. "Rust" is deliberately not
// already in digitizeJSON's skills (["Python","Go"]) so it isn't deduped away.
const extendJSON = `{"newSkills":["Rust"],"newItems":[{"kind":"project","title":"Side project","organization":"","startDate":"2024-01","endDate":"","bullets":[{"text":"Built a CLI tool","skills":["Rust"]}]}],"bulletAdditions":[]}`

// extendUnknownIDJSON references an item id that cannot exist in a freshly
// digitized profile (which only ever has item-0), driving the 502 branch of
// postProfileExtend via model.MergeAdditions' guardrail.
const extendUnknownIDJSON = `{"newSkills":[],"newItems":[],"bulletAdditions":[{"itemId":"item-99","bullets":[{"text":"x","skills":[]}]}]}`

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
// letter schema (shape-tested: its top-level properties include "greeting"),
// as opposed to the tailor schema. fakeLLM uses this to route calls that
// don't carry a PDF block (i.e. everything except Digitize) between the
// tailor and cover-letter fixtures, since GenerateJSON's other parameters
// don't otherwise distinguish the two calls.
func isCoverLetterSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = props["greeting"]
	return ok
}

func (f fakeLLM) GenerateJSON(_ context.Context, _ string, blocks []ai.ContentBlock, schema map[string]any) ([]byte, error) {
	for _, b := range blocks {
		if b.PDF != nil {
			return []byte(digitizeJSON), nil
		}
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
	if f.tailorOut != "" {
		return []byte(f.tailorOut), nil
	}
	return []byte(tailorJSON), nil
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

func newTestServerWithLLM(t *testing.T, llm ai.LLM) (*Server, *echo.Echo) {
	t.Helper()
	s := &Server{
		Store: newStore(t),
		LLM:   llm,
		Mail:  func(model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
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
	s.Mail = func(model.Tailored, []byte, string, []byte, string) (bool, error) {
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
		Store: st,
		LLM:   fakeLLM{},
		Mail:  func(model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
	}
	e := echo.New()
	s.Register(e)

	ta := model.Tailored{TargetRole: "X", Gaps: nil, WhatChanged: nil}
	if _, err := st.SaveGeneration(ta, []byte("pdf-bytes"), "x.pdf", nil, ""); err != nil {
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
		Store: st,
		LLM:   fakeLLM{},
		Mail:  func(model.Tailored, []byte, string, []byte, string) (bool, error) { return false, nil },
	}
	e := echo.New()
	s.Register(e)

	mk := func(gaps ...model.Gap) model.Tailored { return model.Tailored{TargetRole: "X", Gaps: gaps} }
	if _, err := st.SaveGeneration(mk(model.Gap{Requirement: "Django", Evidence: "e1", Severity: "missing"}), []byte("p"), "a.pdf", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveGeneration(mk(model.Gap{Requirement: "django", Evidence: "e2", Severity: "missing"}), []byte("p"), "b.pdf", nil, ""); err != nil {
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
	var gotFilename, gotCoverFilename string
	s.Mail = func(_ model.Tailored, pdf []byte, filename string, coverPDF []byte, coverFilename string) (bool, error) {
		gotPDF, gotFilename, gotCoverPDF, gotCoverFilename = pdf, filename, coverPDF, coverFilename
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
}
