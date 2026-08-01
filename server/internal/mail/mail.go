// Package mail sends the tailored resume PDF via the Resend API. It is
// env-gated: absent RESEND_API_KEY or CVX_EMAIL_TO, Send is a no-op.
package mail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cvx/internal/model"
)

const (
	defaultEndpoint = "https://api.resend.com/emails"
	defaultFrom     = "cvx <onboarding@resend.dev>"
	requestTimeout  = 15 * time.Second
)

type attachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type sendRequest struct {
	From        string       `json:"from"`
	To          string       `json:"to"`
	Subject     string       `json:"subject"`
	HTML        string       `json:"html"`
	Attachments []attachment `json:"attachments"`
}

// Send posts the tailored resume PDF to Resend as an email attachment.
// It returns (false, nil) without making a network call when RESEND_API_KEY
// or CVX_EMAIL_TO is unset or empty. endpoint == "" defaults to the Resend
// emails endpoint; a non-empty value is used as-is (for tests).
func Send(t model.Tailored, pdf []byte, filename string, endpoint string) (bool, error) {
	apiKey := os.Getenv("RESEND_API_KEY")
	to := os.Getenv("CVX_EMAIL_TO")
	if apiKey == "" || to == "" {
		return false, nil
	}

	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	from := os.Getenv("CVX_EMAIL_FROM")
	if from == "" {
		from = defaultFrom
	}

	reqBody := sendRequest{
		From:    from,
		To:      to,
		Subject: fmt.Sprintf("Resume: %s", t.TargetRole),
		HTML:    renderHTML(t),
		Attachments: []attachment{
			{Filename: filename, Content: base64.StdEncoding.EncodeToString(pdf)},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return false, fmt.Errorf("mail: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("mail: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("mail: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("mail: resend returned status %d: %s", resp.StatusCode, string(body))
	}

	return true, nil
}

func renderHTML(t model.Tailored) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<p>Your tailored resume for <b>%s</b> is attached.</p>", html.EscapeString(t.TargetRole))

	if len(t.WhatChanged) > 0 {
		b.WriteString("<h3>What changed</h3><ul>")
		for _, change := range t.WhatChanged {
			fmt.Fprintf(&b, "<li>%s</li>", html.EscapeString(change))
		}
		b.WriteString("</ul>")
	}

	if len(t.Gaps) > 0 {
		b.WriteString("<h3>Gaps for this role</h3><ul>")
		for _, g := range t.Gaps {
			fmt.Fprintf(&b, "<li><b>%s</b> (%s): %s</li>",
				html.EscapeString(g.Requirement), html.EscapeString(g.Severity), html.EscapeString(g.Evidence))
		}
		b.WriteString("</ul>")
	}

	return b.String()
}
