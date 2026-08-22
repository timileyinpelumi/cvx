package mail

import (
	"fmt"
	"html"
	"os"
	"strings"

	"cvx/internal/model"
)

// appURL is where an email points a person back to. Falls back to the
// public site so a link is never broken, even before CVX_BASE_URL is set.
func appURL() string {
	if u := strings.TrimRight(os.Getenv("CVX_BASE_URL"), "/"); u != "" {
		return u
	}
	return "https://cvx.app"
}

// firstName keeps a greeting human without getting a full legal name wrong.
func firstName(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// SendWelcome greets a new account and says, in one screen, what to do
// first. Sent once, at signup.
func SendWelcome(to, name, endpoint string) (bool, error) {
	greeting := "Welcome to cvx"
	if first := firstName(name); first != "" {
		greeting = "Welcome to cvx, " + first
	}

	var b strings.Builder
	b.WriteString(paragraph("cvx turns your history into a one-page resume aimed at one job. You paste the ad, it picks what fits, and it never claims anything you have not done."))

	b.WriteString(sectionTitle("Getting started"))
	b.WriteString(steps([]string{
		"Upload your current resume. cvx reads it once into a profile, and every resume it writes comes from that.",
		"Fill in your details: your links, certifications, languages, anything the file did not carry.",
		"Paste a job ad and compose. You get the PDF, and a cover letter and application email if you want them.",
	}))

	b.WriteString(button("Upload your resume", appURL()+"/account"))

	return post(endpoint, sendRequest{
		To:      to,
		Subject: greeting,
		HTML: shell(greeting, "One page, aimed at one job, built only from what you have actually done.",
			b.String(), "You are getting this because you just signed up for cvx."),
		Text: welcomeText(greeting),
	})
}

func welcomeText(greeting string) string {
	return strings.Join([]string{
		greeting + ".",
		"",
		"cvx turns your history into a one-page resume aimed at one job. You paste the ad, it picks what fits, and it never claims anything you have not done.",
		"",
		"Getting started:",
		"1. Upload your current resume. cvx reads it once into a profile.",
		"2. Fill in your details: links, certifications, languages.",
		"3. Paste a job ad and compose.",
		"",
		"Start here: " + appURL() + "/account",
		"",
		"You are getting this because you just signed up for cvx.",
	}, "\n")
}

// SendFarewell confirms a closed account and says plainly what happened to
// the data, because "we have deleted everything" is the one thing people
// want in writing.
func SendFarewell(to, name, endpoint string) (bool, error) {
	title := "Your cvx account is closed"

	var b strings.Builder
	b.WriteString(paragraph("You closed your cvx account, so you are signed out and it will not open again."))
	b.WriteString(paragraph("Signing in with the same email starts a new, empty account. Your old resumes are not carried across, and the ones you already downloaded are yours to keep."))
	b.WriteString(button("Start again", appURL()))

	return post(endpoint, sendRequest{
		To:      to,
		Subject: title,
		HTML: shell(title, "Confirming what happened, in case you need it in writing.",
			b.String(), "You are getting this because your cvx account was closed."),
		Text: strings.Join([]string{
			title + ".",
			"",
			"You closed your cvx account, so you are signed out and it will not open again.",
			"",
			"Signing in with the same email starts a new, empty account. Your old resumes are not carried across, and the ones you already downloaded are yours to keep.",
			"",
			appURL(),
			"",
			"You are getting this because your cvx account was closed.",
		}, "\n"),
	})
}

/* ------------------------------ plain text ------------------------------ */

// notificationText is the plain-text half of the "your resume is ready"
// email: the same information, in the order the HTML presents it.
func notificationText(t model.Tailored, attachmentNames []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Your resume for %s is ready.\n\n", t.TargetRole)

	b.WriteString("Attached:\n")
	for _, name := range attachmentNames {
		fmt.Fprintf(&b, "  %s\n", name)
	}

	if len(t.WhatChanged) > 0 {
		b.WriteString("\nWhat changed:\n")
		for _, change := range t.WhatChanged {
			fmt.Fprintf(&b, "  - %s\n", change)
		}
	}
	if len(t.Gaps) > 0 {
		b.WriteString("\nGaps for this role:\n")
		for _, g := range t.Gaps {
			fmt.Fprintf(&b, "  - %s (%s)", g.Requirement, g.Severity)
			if g.Evidence != "" {
				fmt.Fprintf(&b, ": %s", g.Evidence)
			}
			b.WriteString("\n")
		}
	}

	fmt.Fprintf(&b, "\n%s/resumes\n", appURL())
	b.WriteString("\ncvx sent this because you generated a resume.\n")
	return b.String()
}

// recruiterText is the plain-text half of the forwardable email. Unbranded,
// like the HTML: it is the candidate's own message.
func recruiterText(re model.RecruiterEmail, name string) string {
	parts := []string{re.Greeting}
	parts = append(parts, re.Paragraphs...)
	closing := re.Closing
	if closing == "" {
		closing = "Best regards,"
	}
	parts = append(parts, closing+"\n"+name)
	return strings.Join(parts, "\n\n")
}

/* -------------------------------- pieces -------------------------------- */

func paragraph(text string) string {
	return fmt.Sprintf(`<p style="margin:0 0 12px;font-size:14px;line-height:1.6;color:%s;">%s</p>`,
		mailFg, html.EscapeString(text))
}

// steps renders a numbered list without relying on list styling, which
// several clients drop.
func steps(items []string) string {
	var b strings.Builder
	b.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" width="100%">`)
	for i, item := range items {
		fmt.Fprintf(&b, `<tr>`+
			`<td valign="top" style="width:24px;padding:0 0 10px;font-size:13px;font-weight:600;color:%s;">%d</td>`+
			`<td style="padding:0 0 10px;font-size:13.5px;line-height:1.55;color:%s;">%s</td>`+
			`</tr>`, mailInk, i+1, mailFg, html.EscapeString(item))
	}
	b.WriteString(`</table>`)
	return b.String()
}

// button is a table-based call to action: the only shape that renders the
// same in Outlook as everywhere else.
func button(label, href string) string {
	return fmt.Sprintf(
		`<table role="presentation" cellpadding="0" cellspacing="0" style="margin:20px 0 4px;">`+
			`<tr><td style="background:%s;border-radius:8px;">`+
			`<a href="%s" style="display:inline-block;padding:11px 20px;font-size:13.5px;font-weight:600;color:#ffffff;text-decoration:none;">%s</a>`+
			`</td></tr></table>`,
		mailInk, html.EscapeString(href), html.EscapeString(label))
}
