// Package jdfetch fetches a job posting page and extracts its readable text,
// so a pasted URL can stand in for a pasted job description.
package jdfetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	fetchTimeout = 15 * time.Second
	maxBodyBytes = 2 << 20 // 2MB
	userAgent    = "cvx/1.0"
)

// skipTags are element subtrees whose text is noise, not job content.
var skipTags = map[string]bool{
	"script":   true,
	"style":    true,
	"nav":      true,
	"footer":   true,
	"noscript": true,
}

// IsURL reports whether s, once trimmed, is a single whitespace-free token
// starting with http:// or https://. Anything else (a role title, a
// multi-line job description, plain text that merely mentions "http") is
// not a URL.
func IsURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return false
	}
	return !strings.ContainsAny(s, " \t\n\r\f\v")
}

// FetchText GETs url and returns its readable text: script/style/nav/footer
// /noscript subtrees are dropped, remaining text nodes are concatenated, and
// whitespace is collapsed. The request is capped at 15s and the body is
// capped at 2MB (a page over the cap is truncated, not rejected).
func FetchText(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}

	doc, err := html.Parse(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	extractText(doc, &sb)

	text := strings.Join(strings.Fields(sb.String()), " ")
	if text == "" {
		return "", fmt.Errorf("no readable text at that link")
	}
	return text, nil
}

func extractText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode && skipTags[n.Data] {
		return
	}
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
		sb.WriteByte(' ')
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, sb)
	}
}
