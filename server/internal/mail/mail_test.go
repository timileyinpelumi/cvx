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
	t.Setenv("CVX_EMAIL_TO", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unexpected request when env is unset")
	}))
	defer srv.Close()

	sent, err := Send(fixture(), []byte("%PDF-1.4 fake"), "resume.pdf", srv.URL)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sent {
		t.Fatal("expected sent=false when env is unset")
	}
}

func TestSend_Configured_PostsExpectedPayload(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key-123")
	t.Setenv("CVX_EMAIL_TO", "candidate@example.com")

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

	sent, err := Send(fixture(), pdf, "resume.pdf", srv.URL)
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
	if subject != "Resume: Python Backend Engineer" {
		t.Errorf("subject = %q, want %q", subject, "Resume: Python Backend Engineer")
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

func TestSend_NonSuccessStatus_ReturnsError(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key-123")
	t.Setenv("CVX_EMAIL_TO", "candidate@example.com")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"invalid from address"}`))
	}))
	defer srv.Close()

	sent, err := Send(fixture(), []byte("%PDF-1.4 fake"), "resume.pdf", srv.URL)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if sent {
		t.Fatal("expected sent=false on error")
	}
}
