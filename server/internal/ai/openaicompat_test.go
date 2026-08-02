package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return m
}

func TestOpenAICompatRequestShape(t *testing.T) {
	var gotAuth string
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer srv.Close()

	c := newOpenAICompat(srv.URL, "test-key", "test-model", true, maxTokensFieldLegacy)
	schema := map[string]any{"type": "object"}
	out, err := c.GenerateJSON(context.Background(), "be helpful", []ContentBlock{{Text: "hello world"}}, schema)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if string(out) != `{"ok":true}` {
		t.Fatalf("want {\"ok\":true}, got %s", out)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("want Bearer auth, got %q", gotAuth)
	}
	if body["model"] != "test-model" {
		t.Fatalf("want model test-model, got %v", body["model"])
	}
	if got, want := body["max_tokens"], float64(maxOutputTokens); got != want {
		t.Fatalf("want max_tokens=%v, got %v", want, got)
	}
	if _, present := body["max_completion_tokens"]; present {
		t.Fatalf("want no max_completion_tokens field when maxTokensField is legacy, got %v", body["max_completion_tokens"])
	}

	rf, ok := body["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("missing response_format: %v", body)
	}
	if rf["type"] != "json_schema" {
		t.Fatalf("want json_schema type, got %v", rf["type"])
	}
	js, ok := rf["json_schema"].(map[string]any)
	if !ok {
		t.Fatalf("missing json_schema: %v", rf)
	}
	if js["strict"] != true {
		t.Fatalf("want strict true, got %v", js["strict"])
	}
	if js["schema"] == nil {
		t.Fatalf("want schema present, got nil")
	}

	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("want 2 messages (system + user), got %v", body["messages"])
	}
	sysMsg := messages[0].(map[string]any)
	if sysMsg["role"] != "system" || sysMsg["content"] != "be helpful" {
		t.Fatalf("want system message, got %v", sysMsg)
	}
	userMsg := messages[1].(map[string]any)
	if userMsg["role"] != "user" {
		t.Fatalf("want user role, got %v", userMsg["role"])
	}
	parts, ok := userMsg["content"].([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("want 1 content part, got %v", userMsg["content"])
	}
	part := parts[0].(map[string]any)
	if part["type"] != "text" || part["text"] != "hello world" {
		t.Fatalf("want text part with hello world, got %v", part)
	}
}

func TestOpenAICompatNoSystemMessage(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer srv.Close()

	c := newOpenAICompat(srv.URL, "k", "m", true, maxTokensFieldLegacy)
	if _, err := c.GenerateJSON(context.Background(), "", []ContentBlock{{Text: "hi"}}, map[string]any{}); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("want only user message when system is empty, got %v", messages)
	}
}

func TestOpenAICompatPDFNativeTrue(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer srv.Close()

	pdfBytes := makeTestPDF(t, "Golang Engineer")
	c := newOpenAICompat(srv.URL, "k", "m", true, maxTokensFieldLegacy)
	if _, err := c.GenerateJSON(context.Background(), "", []ContentBlock{{PDF: pdfBytes}}, map[string]any{}); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	messages := body["messages"].([]any)
	userMsg := messages[0].(map[string]any)
	parts := userMsg["content"].([]any)
	if len(parts) != 1 {
		t.Fatalf("want exactly 1 part for pdfNative, got %d: %v", len(parts), parts)
	}
	part := parts[0].(map[string]any)
	if part["type"] != "file" {
		t.Fatalf("want file part, got %v", part)
	}
	file, ok := part["file"].(map[string]any)
	if !ok {
		t.Fatalf("missing file object: %v", part)
	}
	if file["filename"] != "resume.pdf" {
		t.Fatalf("want filename resume.pdf, got %v", file["filename"])
	}
	fd, _ := file["file_data"].(string)
	if !strings.HasPrefix(fd, "data:application/pdf;base64,") {
		t.Fatalf("want data:application/pdf;base64 prefix, got %q", fd[:min(40, len(fd))])
	}
	b64 := strings.TrimPrefix(fd, "data:application/pdf;base64,")
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if string(decoded) != string(pdfBytes) {
		t.Fatalf("decoded pdf bytes mismatch")
	}
}

func TestOpenAICompatPDFNativeFalseExtractsText(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer srv.Close()

	pdfBytes := makeTestPDF(t, "Golang Engineer")
	c := newOpenAICompat(srv.URL, "k", "m", false, maxTokensFieldLegacy)
	if _, err := c.GenerateJSON(context.Background(), "", []ContentBlock{{PDF: pdfBytes}}, map[string]any{}); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	messages := body["messages"].([]any)
	userMsg := messages[0].(map[string]any)
	parts := userMsg["content"].([]any)
	if len(parts) != 1 {
		t.Fatalf("want exactly 1 part for pdfNative=false, got %d: %v", len(parts), parts)
	}
	part := parts[0].(map[string]any)
	if part["type"] != "text" {
		t.Fatalf("want text part (extracted), got %v", part)
	}
	text, _ := part["text"].(string)
	if !strings.Contains(text, "Golang Engineer") {
		t.Fatalf("want extracted text to contain Golang Engineer, got %q", text)
	}
	if !strings.HasPrefix(text, "RESUME TEXT (extracted from PDF):\n") {
		t.Fatalf("want RESUME TEXT prefix, got %q", text[:min(60, len(text))])
	}
	if _, hasFile := part["file"]; hasFile {
		t.Fatalf("want no file part when pdfNative=false")
	}
}

func TestOpenAICompatNon2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	c := newOpenAICompat(srv.URL, "bad-key", "m", true, maxTokensFieldLegacy)
	_, err := c.GenerateJSON(context.Background(), "", []ContentBlock{{Text: "hi"}}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("want error to contain status 401, got %v", err)
	}
}

func TestOpenAICompatEmptyChoicesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[],"error":{"message":"model overloaded"}}`))
	}))
	defer srv.Close()

	c := newOpenAICompat(srv.URL, "k", "m", true, maxTokensFieldLegacy)
	_, err := c.GenerateJSON(context.Background(), "", []ContentBlock{{Text: "hi"}}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "model overloaded") {
		t.Fatalf("want error to include upstream message, got %v", err)
	}
}

func TestOpenAICompatMaxTokensFieldLegacySendsMaxTokens(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer srv.Close()

	c := newOpenAICompat(srv.URL, "k", "m", true, maxTokensFieldLegacy)
	if _, err := c.GenerateJSON(context.Background(), "", []ContentBlock{{Text: "hi"}}, map[string]any{}); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	if got, want := body["max_tokens"], float64(maxOutputTokens); got != want {
		t.Fatalf("want max_tokens=%v, got %v", want, got)
	}
	if _, present := body["max_completion_tokens"]; present {
		t.Fatalf("want no max_completion_tokens field, got %v", body["max_completion_tokens"])
	}
}

func TestOpenAICompatMaxTokensFieldModernSendsMaxCompletionTokens(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer srv.Close()

	c := newOpenAICompat(srv.URL, "k", "m", true, maxTokensFieldModern)
	if _, err := c.GenerateJSON(context.Background(), "", []ContentBlock{{Text: "hi"}}, map[string]any{}); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	if got, want := body["max_completion_tokens"], float64(maxOutputTokens); got != want {
		t.Fatalf("want max_completion_tokens=%v, got %v", want, got)
	}
	if _, present := body["max_tokens"]; present {
		t.Fatalf("want no max_tokens field, got %v", body["max_tokens"])
	}
}
