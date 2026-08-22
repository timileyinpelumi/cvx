package mail

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cvx/internal/model"
)

func fixture() model.Tailored {
	return model.Tailored{
		TargetRole:  "Python Backend Engineer",
		WhatChanged: []string{"Reordered skills to lead with Python"},
		Gaps: []model.Gap{
			{Requirement: "5+ years Kubernetes", Evidence: "No direct Kubernetes experience found", Severity: "major"},
		},
	}
}

func TestSend_EnvUnset_NoRequest(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unexpected request when env is unset")
	}))
	defer srv.Close()

	sent, err := Send("candidate@example.com", fixture(), []byte("%PDF-1.4 fake"), "resume.pdf", srv.URL)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sent {
		t.Fatal("expected sent=false when env is unset")
	}
}

func TestSend_Configured_PostsExpectedPayload(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key-123")

	pdf := []byte("%PDF-1.4 fake pdf bytes")

	var gotMethod, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"abc123"}`))
	}))
	defer srv.Close()

	sent, err := Send("candidate@example.com", fixture(), pdf, "resume.pdf", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sent {
		t.Fatal("expected sent=true")
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotAuth != "Bearer test-key-123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key-123")
	}

	to, _ := gotBody["to"]
	toStr, _ := json.Marshal(to)
	if !strings.Contains(string(toStr), "candidate@example.com") {
		t.Errorf("to field = %v, want to contain candidate@example.com", to)
	}

	subject, _ := gotBody["subject"].(string)
	if subject != "Your resume for Python Backend Engineer is ready" {
		t.Errorf("subject = %q, want %q", subject, "Your resume for Python Backend Engineer is ready")
	}

	attachments, ok := gotBody["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("attachments = %v, want one attachment", gotBody["attachments"])
	}
	att, ok := attachments[0].(map[string]any)
	if !ok {
		t.Fatalf("attachment element not an object: %v", attachments[0])
	}
	if att["filename"] != "resume.pdf" {
		t.Errorf("attachment filename = %v, want resume.pdf", att["filename"])
	}
	contentStr, _ := att["content"].(string)
	decoded, err := base64.StdEncoding.DecodeString(contentStr)
	if err != nil {
		t.Fatalf("attachment content is not valid base64: %v", err)
	}
	if string(decoded) != string(pdf) {
		t.Errorf("decoded attachment content = %q, want %q", decoded, pdf)
	}

	html, _ := gotBody["html"].(string)
	if !strings.Contains(html, "5+ years Kubernetes") {
		t.Errorf("html body missing gap requirement, got: %s", html)
	}
	if !strings.Contains(html, "Reordered skills to lead with Python") {
		t.Errorf("html body missing whatChanged entry, got: %s", html)
	}
}

func TestSend_WithCoverLetter_IncludesBothAttachments(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key-123")

	pdf := []byte("%PDF-1.4 resume")
	cover := []byte("%PDF-1.4 cover letter")

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"abc123"}`))
	}))
	defer srv.Close()

	sent, err := Send("candidate@example.com", fixture(), pdf, "resume.pdf", srv.URL, Attachment{Filename: "cover.pdf", Content: cover})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sent {
		t.Fatal("expected sent=true")
	}

	attachments, ok := gotBody["attachments"].([]any)
	if !ok || len(attachments) != 2 {
		t.Fatalf("attachments = %v, want two attachments", gotBody["attachments"])
	}

	first, ok := attachments[0].(map[string]any)
	if !ok || first["filename"] != "resume.pdf" {
		t.Fatalf("first attachment = %v, want filename resume.pdf", attachments[0])
	}
	firstDecoded, err := base64.StdEncoding.DecodeString(first["content"].(string))
	if err != nil || string(firstDecoded) != string(pdf) {
		t.Fatalf("first attachment content mismatch: decoded=%q err=%v", firstDecoded, err)
	}

	second, ok := attachments[1].(map[string]any)
	if !ok || second["filename"] != "cover.pdf" {
		t.Fatalf("second attachment = %v, want filename cover.pdf", attachments[1])
	}
	secondDecoded, err := base64.StdEncoding.DecodeString(second["content"].(string))
	if err != nil || string(secondDecoded) != string(cover) {
		t.Fatalf("second attachment content mismatch: decoded=%q err=%v", secondDecoded, err)
	}
}

func TestSend_NonSuccessStatus_ReturnsError(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key-123")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"invalid from address"}`))
	}))
	defer srv.Close()

	sent, err := Send("candidate@example.com", fixture(), []byte("%PDF-1.4 fake"), "resume.pdf", srv.URL)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if sent {
		t.Fatal("expected sent=false on error")
	}
}

func TestSendRecruiter(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key")
	var got sendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header: %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	re := model.RecruiterEmail{
		Subject:    "Application for Backend Engineer",
		Greeting:   "Hello Jane,",
		Paragraphs: []string{"I am applying for the Backend Engineer role."},
		Closing:    "Best regards,",
	}
	ok, err := SendRecruiter("me@example.com", re, "Ada Example",
		[]byte("pdf"), "resume.pdf", srv.URL,
		Attachment{Filename: "cover.pdf", Content: []byte("cover")})
	if err != nil || !ok {
		t.Fatalf("got %v, %v", ok, err)
	}
	if got.Subject != "Application for Backend Engineer" {
		t.Fatalf("subject %q", got.Subject)
	}
	if !strings.Contains(got.HTML, "applying for the Backend Engineer role") || !strings.Contains(got.HTML, "Ada Example") {
		t.Fatalf("body: %s", got.HTML)
	}
	if !strings.HasPrefix(stripTags(got.HTML), "Hello Jane,") {
		t.Fatalf("email does not open with the greeting: %s", got.HTML)
	}
	if strings.Contains(got.HTML, "Gaps") || strings.Contains(got.HTML, "What changed") {
		t.Fatalf("notification content leaked into recruiter email: %s", got.HTML)
	}
	if len(got.Attachments) != 2 {
		t.Fatalf("want 2 attachments, got %d", len(got.Attachments))
	}
}

func TestSendRecruiterGate(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	ok, err := SendRecruiter("me@example.com", model.RecruiterEmail{Subject: "s"}, "n", nil, "r.pdf", "")
	if err != nil || ok {
		t.Fatalf("want false,nil got %v,%v", ok, err)
	}
}

func TestRenderNotificationStructure(t *testing.T) {
	ta := model.Tailored{
		TargetRole:  "Backend <Engineer>",
		WhatChanged: []string{"Led with Go & systems work"},
		Gaps:        []model.Gap{{Requirement: "Kubernetes", Severity: "missing", Evidence: "not in profile"}},
	}
	out := renderNotification(ta, []string{"ADA_Backend.pdf", "ADA_Backend_Cover.pdf"})

	for _, want := range []string{
		"Your resume is ready",
		"Backend &lt;Engineer&gt;",
		"Led with Go &amp; systems work",
		"ADA_Backend.pdf",
		"ADA_Backend_Cover.pdf",
		"Kubernetes",
		"cvx sent this because you generated a resume",
		"#2244d9",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("notification missing %q", want)
		}
	}
	if strings.Contains(out, "<Engineer>") {
		t.Error("role was not HTML-escaped")
	}
}

func TestRenderRecruiterIsUnbranded(t *testing.T) {
	out := renderRecruiter(model.RecruiterEmail{
		Greeting:   "Hello HR,",
		Paragraphs: []string{"I am applying for the role."},
		Closing:    "Best regards,",
	}, "Ada Lovelace")

	for _, want := range []string{"Hello HR,", "I am applying for the role.", "Best regards,", "Ada Lovelace"} {
		if !strings.Contains(out, want) {
			t.Errorf("recruiter email missing %q", want)
		}
	}
	// Forwardable: nothing may point back at cvx.
	if strings.Contains(strings.ToLower(out), "cvx") {
		t.Error("recruiter email must not carry cvx branding")
	}
}

// An email that reached the sender without a greeting still opens with one:
// the greeting is the guarantee, not a field the caller may forget.
func TestRenderRecruiterAlwaysGreets(t *testing.T) {
	out := renderRecruiter(model.RecruiterEmail{Paragraphs: []string{"I am applying."}}, "Ada")
	if !strings.HasPrefix(stripTags(out), "Hello HR,") {
		t.Fatalf("missing fallback greeting: %s", out)
	}
}

// stripTags reduces the rendered HTML to its text so a test can assert on
// reading order rather than on markup.
func stripTags(h string) string {
	var b strings.Builder
	depth := 0
	for _, r := range h {
		switch {
		case r == '<':
			depth++
		case r == '>':
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func TestSendWelcomeAndFarewell(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key")
	t.Setenv("CVX_BASE_URL", "https://cvx.example.com")

	var got sendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if ok, err := SendWelcome("me@example.com", "Ada Lovelace", srv.URL); err != nil || !ok {
		t.Fatalf("welcome: %v, %v", ok, err)
	}
	if got.Subject != "Welcome to cvx, Ada" {
		t.Fatalf("subject: %q", got.Subject)
	}
	// Every email carries a text part: without one it reads as spam.
	if got.Text == "" || !strings.Contains(got.Text, "cvx.example.com") {
		t.Fatalf("text part: %q", got.Text)
	}
	if !strings.Contains(got.HTML, "https://cvx.example.com/account") {
		t.Fatal("welcome email has no way back into the app")
	}
	if len(got.Attachments) != 0 {
		t.Fatal("a welcome email should carry no attachments")
	}

	if ok, err := SendFarewell("me@example.com", "Ada Lovelace", srv.URL); err != nil || !ok {
		t.Fatalf("farewell: %v, %v", ok, err)
	}
	if got.Subject != "Your cvx account is closed" {
		t.Fatalf("subject: %q", got.Subject)
	}
	if !strings.Contains(got.Text, "new, empty account") {
		t.Fatalf("farewell should say what happens next: %q", got.Text)
	}
}

// No API key means no network call, for every sender.
func TestLifecycleMailIsGated(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	if ok, err := SendWelcome("me@example.com", "Ada", "http://127.0.0.1:1"); err != nil || ok {
		t.Fatalf("want false,nil got %v,%v", ok, err)
	}
	if ok, err := SendFarewell("me@example.com", "Ada", "http://127.0.0.1:1"); err != nil || ok {
		t.Fatalf("want false,nil got %v,%v", ok, err)
	}
}
