package model

import "strings"

// Link kinds. A link is either part of the candidate's identity (the handful
// that belong in the contact row) or evidence for one item (a repo, a gist, a
// deployed side project) that has no business competing with the email
// address for header space.
const (
	LinkGitHub    = "github"
	LinkLinkedIn  = "linkedin"
	LinkPortfolio = "portfolio"
	LinkOther     = "other"
)

// identityKindOrder is the contact row's fixed order, most load-bearing
// first, and doubles as the set of kinds allowed in the row at all.
var identityKindOrder = []string{LinkGitHub, LinkLinkedIn, LinkPortfolio}

// maxIdentityLinks caps the contact row. Three plus email, phone, and
// location is already the most a single line can carry.
const maxIdentityLinks = 3

// ClassifyLink derives a link's kind from its URL alone. The LLM's label is
// never trusted for this: "Personal Website" and "ChainPal" are both just
// labels, and only the host and path say which one is an identity.
//
// A GitHub profile (github.com/user) is an identity; a repo or a gist
// (github.com/user/repo, gist.github.com/...) is evidence for an item, so it
// classifies as other and stays out of the contact row.
func ClassifyLink(l Link) string {
	host, path := splitURL(l.URL)
	if host == "" {
		return LinkOther
	}
	switch {
	case host == "gist.github.com":
		return LinkOther
	case host == "github.com":
		if strings.Count(strings.Trim(path, "/"), "/") == 0 && path != "/" && path != "" {
			return LinkGitHub
		}
		return LinkOther
	case host == "linkedin.com":
		if strings.HasPrefix(path, "/in/") {
			return LinkLinkedIn
		}
		return LinkOther
	case isCodeOrProjectHost(host):
		return LinkOther
	}
	// A bare domain the candidate owns, with no deep path, reads as a
	// personal site; anything deeper is a specific page, not an identity.
	if strings.Trim(path, "/") == "" {
		return LinkPortfolio
	}
	return LinkOther
}

// projectHosts are hosts that only ever carry one piece of work, never an
// identity, however the link is labelled.
var projectHosts = map[string]bool{
	"gitlab.com": true, "bitbucket.org": true, "npmjs.com": true,
	"pypi.org": true, "vercel.app": true, "netlify.app": true,
	"herokuapp.com": true, "fly.dev": true, "pages.dev": true,
	"medium.com": true, "dev.to": true, "youtube.com": true,
	"drive.google.com": true, "docs.google.com": true, "notion.so": true,
}

func isCodeOrProjectHost(host string) bool {
	if projectHosts[host] {
		return true
	}
	for suffix := range projectHosts {
		if strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// splitURL returns a link's lowercased host (without "www.") and its path,
// tolerating the scheme-less URLs resumes are full of ("github.com/x").
func splitURL(raw string) (host, path string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	s = strings.TrimPrefix(s, "www.")
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		return strings.ToLower(s[:i]), s[i:]
	}
	return strings.ToLower(s), ""
}

// IdentityLinks returns the links that belong in the contact row: at most
// maxIdentityLinks, at most one per kind, in identityKindOrder. Everything
// else — repos, gists, deployed projects, articles — is left for the items
// that cite it.
func IdentityLinks(p Profile) []Link {
	byKind := map[string]Link{}
	for _, l := range p.Links {
		if strings.TrimSpace(l.URL) == "" {
			continue
		}
		kind := ClassifyLink(l)
		if _, taken := byKind[kind]; !taken {
			byKind[kind] = l
		}
	}
	var out []Link
	for _, kind := range identityKindOrder {
		if l, ok := byKind[kind]; ok {
			out = append(out, l)
			if len(out) == maxIdentityLinks {
				break
			}
		}
	}
	return out
}

// LinkDisplay renders a link the way a resume should: the bare host and path,
// no scheme and no "www.", so a human and an ATS parser read the same string.
func LinkDisplay(l Link) string {
	host, path := splitURL(l.URL)
	if host == "" {
		return strings.TrimSpace(l.Label)
	}
	return strings.TrimSuffix(host+path, "/")
}
