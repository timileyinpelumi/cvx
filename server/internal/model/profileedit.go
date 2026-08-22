package model

import "strings"

// ProfileEdits is the small set of profile facts a person has to be able to
// fix by hand. Deliberately not a CV manager: work history comes from the
// uploaded resume and from notes, and every resume writes its own summary,
// so neither is here.
type ProfileEdits struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Location string `json:"location"`

	// The only links a resume ever prints: who you are, not what you worked
	// on. Project and repo links belong to the item that cites them.
	GitHub    string `json:"github"`
	LinkedIn  string `json:"linkedin"`
	Portfolio string `json:"portfolio"`

	// The material that fills a page real experience does not.
	Certifications []Certification `json:"certifications"`
	Languages      []string        `json:"languages"`
	Interests      []string        `json:"interests"`
}

// EditsFrom reads the editable facts back out of a profile, so the editor
// shows what is stored rather than a guess.
func EditsFrom(p Profile) ProfileEdits {
	e := ProfileEdits{
		Name:           p.Name,
		Email:          p.Email,
		Phone:          p.Phone,
		Location:       p.Location,
		Certifications: p.Certifications,
		Languages:      p.Languages,
		Interests:      p.Interests,
	}
	for _, l := range p.Links {
		switch ClassifyLink(l) {
		case LinkGitHub:
			if e.GitHub == "" {
				e.GitHub = l.URL
			}
		case LinkLinkedIn:
			if e.LinkedIn == "" {
				e.LinkedIn = l.URL
			}
		case LinkPortfolio:
			if e.Portfolio == "" {
				e.Portfolio = l.URL
			}
		}
	}
	return e
}

// ApplyEdits returns stored with the editable facts replaced. Work history,
// skills, and everything else the digitizer produced are untouched: this
// endpoint owns exactly the fields the editor shows.
func ApplyEdits(stored Profile, e ProfileEdits) Profile {
	out := stored
	out.Name = strings.TrimSpace(e.Name)
	out.Email = strings.TrimSpace(e.Email)
	out.Phone = strings.TrimSpace(e.Phone)
	out.Location = strings.TrimSpace(e.Location)
	out.Certifications = cleanCertifications(e.Certifications)
	out.Languages = cleanStrings(e.Languages)
	out.Interests = cleanStrings(e.Interests)

	// Links the resume never prints (repos, gists, deployed projects) are
	// kept as they were; only the three identity slots are replaced.
	var links []Link
	for _, l := range stored.Links {
		if ClassifyLink(l) == LinkOther {
			links = append(links, l)
		}
	}
	for _, slot := range []struct{ label, url string }{
		{"GitHub", e.GitHub},
		{"LinkedIn", e.LinkedIn},
		{"Website", e.Portfolio},
	} {
		if url := strings.TrimSpace(slot.url); url != "" {
			links = append(links, Link{Label: slot.label, URL: url})
		}
	}
	out.Links = links
	return out
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		key := strings.ToLower(s)
		if s == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func cleanCertifications(in []Certification) []Certification {
	out := make([]Certification, 0, len(in))
	for _, c := range in {
		c.Name, c.Issuer, c.Year = strings.TrimSpace(c.Name), strings.TrimSpace(c.Issuer), strings.TrimSpace(c.Year)
		if c.Name == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}
