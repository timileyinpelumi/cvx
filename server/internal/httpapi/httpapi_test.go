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

func (f fakeLLM) GenerateJSON(_ context.Context, _ string, blocks []ai.ContentBlock, _ map[string]any) ([]byte, error) {
	for _, b := range blocks {
		if b.PDF != nil {
			return []byte(digitizeJSON), nil
		}
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
		Mail:  func(model.Tailored, []byte, string) (bool, error) { return false, nil },
	}
	e := echo.New()
	s.Register(e)
	return s, e
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
	s.Mail = func(model.Tailored, []byte, string) (bool, error) {
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

// TestGenerationsListNilSlicesSerializeAsEmptyArrays seeds a generation with
// nil Gaps/WhatChanged directly via store.SaveGeneration (bypassing the LLM
// entirely, so it exercises the store-layer nil-guard) and checks the
// /api/generations response body never contains null for those fields.
func TestGenerationsListNilSlicesSerializeAsEmptyArrays(t *testing.T) {
	st := newStore(t)
	s := &Server{
		Store: st,
		LLM:   fakeLLM{},
		Mail:  func(model.Tailored, []byte, string) (bool, error) { return false, nil },
	}
	e := echo.New()
	s.Register(e)

	ta := model.Tailored{TargetRole: "X", Gaps: nil, WhatChanged: nil}
	if _, err := st.SaveGeneration(ta, []byte("pdf-bytes"), "x.pdf"); err != nil {
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
