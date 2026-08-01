package jdfetch

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"http url", "http://example.com/job/123", true},
		{"https url", "https://example.com/job/123", true},
		{"padded url", "  https://example.com/job/123  ", true},
		{"role title", "Senior Python Backend Engineer", false},
		{"multiline jd", "We are hiring a backend engineer.\nRequirements:\n- Python\n- Go", false},
		{"lowercase mention mid-string", "python http engineer", false},
		{"url with embedded space", "https://example.com/job 123", false},
		{"empty", "", false},
		{"just whitespace", "   ", false},
		{"ftp scheme", "ftp://example.com/job", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsURL(c.in); got != c.want {
				t.Errorf("IsURL(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

const htmlFixture = `<!DOCTYPE html>
<html>
<head><style>body { color: red; }</style></head>
<body>
<nav>Home About Contact</nav>
<script>console.log("tracking pixel");</script>
<main>
<h1>Senior Backend Engineer</h1>
<p>We need someone who knows Python and Go.</p>
</main>
<footer>Copyright 2026 Acme</footer>
<noscript>Enable JavaScript to view this page</noscript>
</body>
</html>`

func TestFetchTextCleansNoise(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlFixture))
	}))
	defer srv.Close()

	got, err := FetchText(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchText: %v", err)
	}
	for _, noise := range []string{"tracking pixel", "Home About Contact", "Copyright 2026", "Enable JavaScript", "color: red"} {
		if strings.Contains(got, noise) {
			t.Errorf("want no %q in extracted text, got %q", noise, got)
		}
	}
	for _, want := range []string{"Senior Backend Engineer", "Python and Go"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in extracted text, got %q", want, got)
		}
	}
}

func TestFetchText404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchText(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("want error for 404 response")
	}
}

func TestFetchTextOversizedBodyTruncatesNotErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body><p>"))
		w.Write(bytes.Repeat([]byte("a "), 3<<20)) // > 2MB, forces the LimitReader to cut mid-body
		w.Write([]byte("</p></body></html>"))
	}))
	defer srv.Close()

	got, err := FetchText(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("want no error for an oversized body (should truncate), got %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want non-empty truncated text")
	}
	if len(got) > 3<<20 {
		t.Fatalf("want text capped near the 2MB read limit, got %d bytes", len(got))
	}
}

func TestFetchTextEmptyPageErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><style>body{color:red}</style></head><body><script>x()</script><nav>Menu</nav></body></html>`))
	}))
	defer srv.Close()

	_, err := FetchText(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("want error for a page with no readable text")
	}
	if !strings.Contains(err.Error(), "no readable text at that link") {
		t.Fatalf("want 'no readable text at that link' error, got %v", err)
	}
}
