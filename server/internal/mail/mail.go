// Package mail sends the tailored resume PDF via the Resend API. It is
// gated on RESEND_API_KEY and a non-empty recipient (the signed-in user's
// email); absent either, Send is a no-op.
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
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	// Text is the plain-text alternative. Every email carries one: it is
	// what a screen reader and a text-only client get, and a message with
	// no text part looks like spam to most filters.
	Text        string       `json:"text,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
}

// post sends one prepared email. Shared by every sender so the gate (no API
// key, no recipient, no call) and the error handling live in one place.
func post(endpoint string, req sendRequest) (bool, error) {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" || req.To == "" {
		return false, nil
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	if req.From == "" {
		if req.From = os.Getenv("CVX_EMAIL_FROM"); req.From == "" {
			req.From = defaultFrom
		}
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return false, fmt.Errorf("mail: marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("mail: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(httpReq)
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

// Attachment is an additional file to send alongside the primary resume PDF
// (currently used for an optional cover letter PDF).
type Attachment struct {
	Filename string
	Content  []byte
}

// Send posts the tailored resume PDF to Resend as an email attachment, plus
// any extra attachments (e.g. a cover letter PDF), addressed to the given
// recipient (the signed-in user's email). It returns (false, nil) without
// making a network call when RESEND_API_KEY is unset or the recipient is
// empty. endpoint == "" defaults to the Resend emails endpoint; a non-empty
// value is used as-is (for tests).
func Send(to string, t model.Tailored, pdf []byte, filename string, endpoint string, extra ...Attachment) (bool, error) {
	apiKey := os.Getenv("RESEND_API_KEY")
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

	attachments := []attachment{
		{Filename: filename, Content: base64.StdEncoding.EncodeToString(pdf)},
	}
	for _, a := range extra {
		attachments = append(attachments, attachment{Filename: a.Filename, Content: base64.StdEncoding.EncodeToString(a.Content)})
	}

	names := []string{filename}
	for _, a := range extra {
		names = append(names, a.Filename)
	}

	return post(endpoint, sendRequest{
		From:        from,
		To:          to,
		Subject:     fmt.Sprintf("Your resume for %s is ready", t.TargetRole),
		HTML:        renderNotification(t, names),
		Text:        notificationText(t, names),
		Attachments: attachments,
	})
}

// SendRecruiter posts the forwardable recruiter-facing email: just the
// application note and the attachments, none of the notification content.
// Same gate and transport as Send.
func SendRecruiter(to string, re model.RecruiterEmail, name string, pdf []byte, filename string, endpoint string, extra ...Attachment) (bool, error) {
	apiKey := os.Getenv("RESEND_API_KEY")
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

	attachments := []attachment{
		{Filename: filename, Content: base64.StdEncoding.EncodeToString(pdf)},
	}
	for _, a := range extra {
		attachments = append(attachments, attachment{Filename: a.Filename, Content: base64.StdEncoding.EncodeToString(a.Content)})
	}

	return post(endpoint, sendRequest{
		From:        from,
		To:          to,
		Subject:     re.Subject,
		HTML:        renderRecruiter(re, name),
		Text:        recruiterText(re, name),
		Attachments: attachments,
	})
}

// Email palette: the app's machine-and-paper theme in its light form. Email
// clients ignore stylesheets, so everything is inline and table-based.
const (
	mailFont    = "-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif"
	mailSurface = "#f5f6f8"
	mailCard    = "#ffffff"
	mailLine    = "#dee2e8"
	mailFg      = "#12161c"
	mailMuted   = "#5c6672"
	mailFaint   = "#8a94a2"
	mailInk     = "#2244d9"
	mailMissing = "#c4342a"
	mailWeak    = "#9a6510"
)

// shell wraps body in the cvx-branded email frame: wordmark band, white card
// with title and subtitle, and a quiet footer explaining why the email came.
func shell(title, subtitle, body, footer string) string {
	var b strings.Builder

	// Tell the client this design is light-only, so it tints rather than
	// inverts, and hide the preheader that would otherwise show as a naked
	// line of text in the inbox list.
	b.WriteString(`<meta name="color-scheme" content="light">`)
	fmt.Fprintf(&b,
		`<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">%s</div>`,
		html.EscapeString(subtitle))

	fmt.Fprintf(&b, `<div style="background:%s;padding:32px 16px;font-family:%s;">`, mailSurface, mailFont)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">`)
	b.WriteString(`<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;">`)

	fmt.Fprintf(&b, `<tr><td style="padding:0 4px 14px;font-size:17px;font-weight:700;letter-spacing:-0.02em;color:%s;">cvx</td></tr>`, mailInk)

	fmt.Fprintf(&b, `<tr><td style="background:%s;border:1px solid %s;border-radius:12px;padding:28px;">`, mailCard, mailLine)
	fmt.Fprintf(&b, `<h1 style="margin:0;font-size:20px;line-height:1.3;letter-spacing:-0.02em;color:%s;">%s</h1>`, mailFg, html.EscapeString(title))
	if subtitle != "" {
		fmt.Fprintf(&b, `<p style="margin:6px 0 0;font-size:13.5px;line-height:1.5;color:%s;">%s</p>`, mailMuted, html.EscapeString(subtitle))
	}
	b.WriteString(body)
	b.WriteString(`</td></tr>`)

	fmt.Fprintf(&b,
		`<tr><td style="padding:14px 4px 0;font-size:12px;line-height:1.5;color:%s;">%s<br>`+
			`<a href="%s" style="color:%s;text-decoration:none;">%s</a></td></tr>`,
		mailFaint, html.EscapeString(footer), html.EscapeString(appURL()), mailFaint,
		html.EscapeString(strings.TrimPrefix(strings.TrimPrefix(appURL(), "https://"), "http://")))
	b.WriteString(`</table></td></tr></table></div>`)
	return b.String()
}

// sectionTitle renders the small uppercase label the app uses for regions.
func sectionTitle(label string) string {
	return fmt.Sprintf(`<p style="margin:24px 0 8px;font-size:11px;font-weight:600;letter-spacing:0.09em;text-transform:uppercase;color:%s;">%s</p>`, mailFaint, html.EscapeString(label))
}

// renderNotification builds the "your resume is ready" email: what is
// attached, what changed, and the gaps the role exposed.
func renderNotification(t model.Tailored, attachmentNames []string) string {
	var b strings.Builder

	b.WriteString(sectionTitle("Attached"))
	for _, name := range attachmentNames {
		fmt.Fprintf(&b, `<p style="margin:0 0 4px;font-size:13px;color:%s;font-family:ui-monospace,'SF Mono',Consolas,monospace;">%s</p>`, mailFg, html.EscapeString(name))
	}

	if len(t.WhatChanged) > 0 {
		b.WriteString(sectionTitle("What changed"))
		b.WriteString(`<ul style="margin:0;padding:0 0 0 18px;">`)
		for _, change := range t.WhatChanged {
			fmt.Fprintf(&b, `<li style="margin:0 0 6px;font-size:13.5px;line-height:1.55;color:%s;">%s</li>`, mailFg, html.EscapeString(change))
		}
		b.WriteString(`</ul>`)
	}

	if len(t.Gaps) > 0 {
		b.WriteString(sectionTitle("Gaps for this role"))
		for _, g := range t.Gaps {
			color := mailWeak
			if g.Severity == "missing" {
				color = mailMissing
			}
			fmt.Fprintf(&b, `<p style="margin:0 0 8px;font-size:13.5px;line-height:1.55;color:%s;"><strong>%s</strong> <span style="color:%s;">(%s)</span>`,
				mailFg, html.EscapeString(g.Requirement), color, html.EscapeString(g.Severity))
			if g.Evidence != "" {
				fmt.Fprintf(&b, `<br><span style="color:%s;">%s</span>`, mailMuted, html.EscapeString(g.Evidence))
			}
			b.WriteString(`</p>`)
		}
	}

	b.WriteString(button("Open it in cvx", appURL()+"/resumes"))

	subtitle := "Tailored for " + t.TargetRole + " and attached as a PDF."
	return shell("Your resume is ready", subtitle, b.String(),
		"cvx sent this because you generated a resume. The files are attached and ready to send.")
}

// renderRecruiter builds the forwardable application email. Deliberately
// unbranded: it is the candidate's own message, so it carries clean
// typography and a signature, nothing that points back at cvx.
func renderRecruiter(re model.RecruiterEmail, name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div style="max-width:600px;font-family:%s;font-size:15px;line-height:1.6;color:%s;">`, mailFont, mailFg)
	greeting := re.Greeting
	if greeting == "" {
		greeting = model.RecruiterGreeting("")
	}
	fmt.Fprintf(&b, `<p style="margin:0 0 14px;">%s</p>`, html.EscapeString(greeting))
	for _, para := range re.Paragraphs {
		if para == "" {
			continue
		}
		fmt.Fprintf(&b, `<p style="margin:0 0 14px;">%s</p>`, html.EscapeString(para))
	}
	closing := re.Closing
	if closing == "" {
		closing = "Best regards,"
	}
	fmt.Fprintf(&b, `<p style="margin:22px 0 0;">%s<br>%s</p>`, html.EscapeString(closing), html.EscapeString(name))
	b.WriteString(`</div>`)
	return b.String()
}
