package model

import (
	"fmt"
	"regexp"
	"strings"
)

type Bullet struct {
	ID     string   `json:"id"`
	Text   string   `json:"text"`
	Skills []string `json:"skills"`
}

type Item struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Organization string   `json:"organization"`
	StartDate    string   `json:"startDate"`
	EndDate      string   `json:"endDate"`
	Bullets      []Bullet `json:"bullets"`
}

type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Profile struct {
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Location string   `json:"location"`
	Summary  string   `json:"summary"`
	Links    []Link   `json:"links"`
	Skills   []string `json:"skills"`
	Items    []Item   `json:"items"`
}

type TBullet struct {
	SourceBulletID string `json:"sourceBulletId"`
	Text           string `json:"text"`
}

type TItem struct {
	SourceID     string    `json:"sourceId"`
	Title        string    `json:"title"`
	Organization string    `json:"organization"`
	Dates        string    `json:"dates"`
	Bullets      []TBullet `json:"bullets"`
}

type TSection struct {
	Title string  `json:"title"`
	Items []TItem `json:"items"`
}

type Gap struct {
	Requirement string `json:"requirement"`
	Evidence    string `json:"evidence"`
	Severity    string `json:"severity"`
}

type Tailored struct {
	TargetRole     string     `json:"targetRole"`
	RoleSummary    string     `json:"roleSummary"`
	Headline       string     `json:"headline"`
	Summary        string     `json:"summary"`
	SelectedSkills []string   `json:"selectedSkills"`
	Sections       []TSection `json:"sections"`
	Gaps           []Gap      `json:"gaps"`
	WhatChanged    []string   `json:"whatChanged"`
}

func AssignIDs(p *Profile) {
	for i := range p.Items {
		p.Items[i].ID = fmt.Sprintf("item-%d", i)
		for j := range p.Items[i].Bullets {
			p.Items[i].Bullets[j].ID = fmt.Sprintf("item-%d-b-%d", i, j)
		}
	}
}

func ValidateTailored(p Profile, t Tailored) error {
	items := map[string]bool{}
	bullets := map[string]bool{}
	for _, it := range p.Items {
		items[it.ID] = true
		for _, b := range it.Bullets {
			bullets[b.ID] = true
		}
	}
	for _, s := range t.Sections {
		for _, it := range s.Items {
			if !items[it.SourceID] {
				return fmt.Errorf("tailored output references unknown profile item: %s", it.SourceID)
			}
			for _, b := range it.Bullets {
				if !bullets[b.SourceBulletID] {
					return fmt.Errorf("tailored output references unknown profile bullet: %s", b.SourceBulletID)
				}
			}
		}
	}
	return nil
}

// MinSummaryWords and MaxSummaryWords are the summary's word range, exported
// because the eval harness checks generations against the same standard the
// pipeline enforces.
const (
	MinSummaryWords = 55
	MaxSummaryWords = 75
)

// The resume content standard: hard ceilings NormalizeTailored clamps every
// generation to, whatever the LLM emitted. Lists are relevance-ordered by the
// tailor contract, so trimming from the bottom always drops the weakest.
const (
	maxResumeItems   = 5
	maxItemBullets   = 4
	maxHeadlineChars = 110
	maxSummaryWords  = MaxSummaryWords
	maxTargetRole    = 64
	maxRoleSummary   = 150
	maxResumeSkills  = 14
	maxWhatChanged   = 4
	maxGapsListed    = 6
)

// The other half of the content standard: floors. A ceiling can be enforced
// by trimming, but nothing can invent a missing sentence, so these are
// reported by ShapeIssues and repaired by one more model pass instead.
const (
	minResumeItems  = 3
	minItemBullets  = 2
	minSummaryWords = MinSummaryWords
	minResumeSkills = 6
)

// ShapeIssues lists the content-standard floors t misses, phrased as
// instructions the tailor can act on. An empty result means the output is
// usable as-is. Checked before normalization: every ceiling NormalizeTailored
// enforces is a trim, which can only move a value away from a floor when the
// model was already over the top of the range.
//
// Floors are capped by what p actually holds: a profile with two items and
// four skills cannot be made to yield three items and six skills, and asking
// for it would only push the model to invent.
func ShapeIssues(p Profile, t Tailored) []string {
	var out []string

	out = append(out, voiceIssues("summary", t.Summary, p.Name)...)
	out = append(out, voiceIssues("headline", t.Headline, p.Name)...)

	if n := len(strings.Fields(t.Summary)); n < minSummaryWords {
		out = append(out, fmt.Sprintf(
			"The summary is %d words. Rewrite it to %d-%d words: what the candidate does, the evidence that matters most for this role, and the bridge to it.",
			n, minSummaryWords, maxSummaryWords))
	}

	sourceBullets := map[string]int{}
	for _, it := range p.Items {
		sourceBullets[it.ID] = len(it.Bullets)
	}

	items := 0
	for _, sec := range t.Sections {
		for _, it := range sec.Items {
			items++
			want := min(minItemBullets, sourceBullets[it.SourceID])
			if len(it.Bullets) < want {
				out = append(out, fmt.Sprintf(
					"%q has %d bullet(s). Give it at least %d, or drop the item and use the space on one that earns it.",
					it.Title, len(it.Bullets), want))
			}
		}
	}
	if want := min(minResumeItems, len(p.Items)); items < want {
		out = append(out, fmt.Sprintf(
			"Only %d items are selected. Select at least %d: check older roles, side projects, open source, and education before concluding the profile has nothing more.",
			items, want))
	}

	if want := min(minResumeSkills, len(p.Skills)); len(t.SelectedSkills) < want {
		out = append(out, fmt.Sprintf(
			"Only %d skills are selected. Select at least %d, copied verbatim from the profile's skills array.",
			len(t.SelectedSkills), want))
	}

	return out
}

// NormalizeTailored clamps t to the content standard in place: item, bullet,
// skill, gap, and whatChanged counts, headline length (cut at a word
// boundary), and summary length (cut at a sentence boundary when one exists).
// Empty sections are dropped.
func NormalizeTailored(t *Tailored) {
	total := 0
	for _, s := range t.Sections {
		total += len(s.Items)
	}
	dropped := 0

	itemsLeft := maxResumeItems
	sections := t.Sections[:0]
	for _, s := range t.Sections {
		if len(s.Items) > itemsLeft {
			s.Items = s.Items[:itemsLeft]
		}
		itemsLeft -= len(s.Items)
		kept := s.Items[:0]
		for _, it := range s.Items {
			if len(it.Bullets) > maxItemBullets {
				it.Bullets = it.Bullets[:maxItemBullets]
			}
			// A one-bullet item costs a title, a date, and a line of white
			// space to say almost nothing, so it goes — but only while the
			// page still has enough items left to look like a resume.
			if len(it.Bullets) < minItemBullets && total-dropped > minResumeItems {
				dropped++
				continue
			}
			kept = append(kept, it)
		}
		s.Items = kept
		if len(s.Items) > 0 {
			sections = append(sections, s)
		}
	}
	t.Sections = sections

	t.TargetRole = cutAtWord(dropParenthetical(t.TargetRole), maxTargetRole)
	t.RoleSummary = cutAtWord(t.RoleSummary, maxRoleSummary)
	t.Headline = cutAtWord(t.Headline, maxHeadlineChars)
	t.Summary = cutAtSentence(t.Summary, maxSummaryWords)

	if len(t.SelectedSkills) > maxResumeSkills {
		t.SelectedSkills = t.SelectedSkills[:maxResumeSkills]
	}
	if len(t.WhatChanged) > maxWhatChanged {
		t.WhatChanged = t.WhatChanged[:maxWhatChanged]
	}
	if len(t.Gaps) > maxGapsListed {
		t.Gaps = t.Gaps[:maxGapsListed]
	}
}

// selfPronouns are the pronouns a resume never uses about its own subject.
// A resume is written in implied first person: no "I", and above all no
// "he"/"she", which reads as a resume written about the candidate by someone
// else. Plural pronouns are left alone; "teams and their services" is fine.
var selfPronouns = regexp.MustCompile(`(?i)\b(i|me|my|mine|he|him|his|she|her|hers)\b`)

// narratorOpen catches the other half of the same failure: the summary that
// opens by naming the candidate, as though a third party were introducing
// them.
var narratorOpen = regexp.MustCompile(`(?i)^\s*(they|this candidate)\b`)

// voiceIssues enforces the resume voice on one field: implied first person,
// no pronouns for the candidate, and never the candidate's own name. The
// reader already knows whose resume this is.
func voiceIssues(label, text, candidateName string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var out []string

	if m := selfPronouns.FindString(text); m != "" {
		out = append(out, fmt.Sprintf(
			"The %s uses the pronoun %q. Write it in implied first person: no pronouns at all, starting from the role (\"Backend engineer who builds...\", not \"He builds...\" or \"I build...\").",
			label, m))
	}
	if narratorOpen.MatchString(text) {
		out = append(out, fmt.Sprintf(
			"The %s opens as though someone else were introducing the candidate. Open on the role itself.", label))
	}
	for _, part := range strings.Fields(candidateName) {
		if len(part) < 3 {
			continue
		}
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(part) + `\b`).MatchString(text) {
			out = append(out, fmt.Sprintf(
				"The %s names the candidate (%q). A resume never refers to its own subject by name; start from the role instead.",
				label, part))
			break
		}
	}
	return out
}

// dropParenthetical strips a trailing "(...)" aside, which is how a pasted
// job title usually carries its list of duties. Those belong in RoleSummary,
// not in the title the resume and its filename are named after.
func dropParenthetical(s string) string {
	if i := strings.Index(s, " ("); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// cutAtWord returns s unchanged when it fits in maxChars, else the longest
// prefix that ends on a whole word.
func cutAtWord(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	cut := s[:maxChars]
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:-")
}

// cutAtSentence returns s unchanged when it fits in maxWords, else the
// longest run of whole sentences that fits, falling back to a word cut when
// the first sentence alone is over the limit.
func cutAtSentence(s string, maxWords int) string {
	words := strings.Fields(s)
	if len(words) <= maxWords {
		return s
	}
	kept := strings.Join(words[:maxWords], " ")
	if i := strings.LastIndexAny(kept, ".!?"); i > 0 {
		return kept[:i+1]
	}
	return kept + "."
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// Filename caps: real names and LLM-authored role titles both run long, and
// the download name has to stay scannable in a file manager.
const (
	filenameNameCap = 20
	filenameRoleCap = 28
)

// filenameBase builds "Name_Job_Title_1234": name and role cleaned to
// Title_Cased underscore-separated words, each part capped at a word
// boundary, plus the numeric id that keeps regenerated files apart.
func filenameBase(name, role string, id int) string {
	clean := func(s string, cap int) string {
		s = strings.Trim(nonAlnum.ReplaceAllString(s, "_"), "_")
		words := strings.Split(s, "_")
		for i, w := range words {
			if w == "" {
				continue
			}
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
		s = strings.Join(words, "_")
		if len(s) > cap {
			cut := s[:cap]
			if i := strings.LastIndexByte(cut, '_'); i > 0 {
				cut = cut[:i]
			}
			s = cut
		}
		return s
	}
	return fmt.Sprintf("%s_%s_%04d", clean(name, filenameNameCap), clean(role, filenameRoleCap), id)
}

func Filename(name, role string, id int) string {
	return filenameBase(name, role, id) + ".pdf"
}

// CoverLetter is an optional second document generated alongside the
// tailored resume: a short letter addressed to the hiring team (or a named
// company), referencing only facts present in the candidate's profile.
// Ungrounded is one claim in generated prose that the profile does not
// support: what was written, where, and what the profile actually says.
type Ungrounded struct {
	Artifact    string `json:"artifact"`
	Claim       string `json:"claim"`
	ProfileSays string `json:"profileSays"`
}

type CoverLetter struct {
	// Greeting is composed from the parsed Posting, never written by the
	// model.
	Greeting   string   `json:"greeting"`
	Paragraphs []string `json:"paragraphs"`
	Closing    string   `json:"closing"`
}

type RecruiterEmail struct {
	Subject string `json:"subject"`
	// Greeting is composed from the parsed Posting, never written by the
	// model.
	Greeting   string   `json:"greeting"`
	Paragraphs []string `json:"paragraphs"`
	Closing    string   `json:"closing"`
}

func CoverFilename(name, role string, id int) string {
	return filenameBase(name, role, id) + "_Cover.pdf"
}

// BulletDraft and ItemDraft mirror Bullet and Item minus id fields — the LLM
// never assigns ids; MergeAdditions does that deterministically, continuing
// the existing item-N / item-N-b-j sequence.
type BulletDraft struct {
	Text   string   `json:"text"`
	Skills []string `json:"skills"`
}

type ItemDraft struct {
	Kind         string        `json:"kind"`
	Title        string        `json:"title"`
	Organization string        `json:"organization"`
	StartDate    string        `json:"startDate"`
	EndDate      string        `json:"endDate"`
	Bullets      []BulletDraft `json:"bullets"`
}

// BulletAddition adds new bullets to an existing item, referenced by its
// exact (already-assigned) id.
type BulletAddition struct {
	ItemID  string        `json:"itemId"`
	Bullets []BulletDraft `json:"bullets"`
}

// ProfileAdditions is what ai.ExtendProfile derives from a candidate's typed
// note: new skills, whole new items, and/or bullets to append to items that
// already exist in the profile. MergeAdditions is the only thing that turns
// these into ided Profile content.
type ProfileAdditions struct {
	NewSkills       []string         `json:"newSkills"`
	NewItems        []ItemDraft      `json:"newItems"`
	BulletAdditions []BulletAddition `json:"bulletAdditions"`
}

// MergeAdditions applies a into p in place: new skills are appended deduped
// case-insensitively against existing skills; new items are appended with
// the next sequential item-N id (continuing from len(p.Items)) and their
// bullets item-N-b-j; bullet additions are appended to the existing item
// they reference, continuing that item's own bullet index. Every
// BulletAddition.ItemID is validated against p's existing items before any
// mutation happens, so an unknown id (guardrail spirit: additions may only
// extend content the id scheme already knows about) leaves p unchanged and
// returns an error naming the id.
func MergeAdditions(p *Profile, a ProfileAdditions) error {
	for _, ba := range a.BulletAdditions {
		found := false
		for i := range p.Items {
			if p.Items[i].ID == ba.ItemID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("profile extend: unknown item id %q", ba.ItemID)
		}
	}

	existingSkills := map[string]bool{}
	for _, sk := range p.Skills {
		existingSkills[strings.ToLower(sk)] = true
	}
	for _, sk := range a.NewSkills {
		key := strings.ToLower(sk)
		if existingSkills[key] {
			continue
		}
		existingSkills[key] = true
		p.Skills = append(p.Skills, sk)
	}

	// len(p.Items) is only a valid source of fresh, unused item-N ids because
	// nothing ever deletes an item; revisit this if a delete path is added.
	next := len(p.Items)
	for _, di := range a.NewItems {
		item := Item{
			ID:           fmt.Sprintf("item-%d", next),
			Kind:         di.Kind,
			Title:        di.Title,
			Organization: di.Organization,
			StartDate:    di.StartDate,
			EndDate:      di.EndDate,
			Bullets:      make([]Bullet, len(di.Bullets)),
		}
		for j, db := range di.Bullets {
			item.Bullets[j] = Bullet{ID: fmt.Sprintf("item-%d-b-%d", next, j), Text: db.Text, Skills: db.Skills}
		}
		p.Items = append(p.Items, item)
		next++
	}

	for _, ba := range a.BulletAdditions {
		for i := range p.Items {
			if p.Items[i].ID != ba.ItemID {
				continue
			}
			nextB := len(p.Items[i].Bullets)
			for _, db := range ba.Bullets {
				p.Items[i].Bullets = append(p.Items[i].Bullets, Bullet{
					ID: fmt.Sprintf("%s-b-%d", ba.ItemID, nextB), Text: db.Text, Skills: db.Skills,
				})
				nextB++
			}
			break
		}
	}

	return nil
}

// NonNil returns s unchanged if it is already non-nil, or an empty
// (non-nil) slice of the same type otherwise. Use this on any slice field
// that reaches an HTTP JSON response, so a nil slice (e.g. an LLM response
// that omitted "gaps"/"whatChanged", or an empty SQL scan) serializes as
// [] rather than null — UI code that unconditionally .map()s over these
// fields would otherwise crash.
func NonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
