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

// The resume content standard: hard ceilings NormalizeTailored clamps every
// generation to, whatever the LLM emitted. Lists are relevance-ordered by the
// tailor contract, so trimming from the bottom always drops the weakest.
const (
	maxResumeItems   = 5
	maxItemBullets   = 4
	maxHeadlineChars = 110
	maxSummaryWords  = 75
	maxResumeSkills  = 14
	maxWhatChanged   = 4
	maxGapsListed    = 6
)

// NormalizeTailored clamps t to the content standard in place: item, bullet,
// skill, gap, and whatChanged counts, headline length (cut at a word
// boundary), and summary length (cut at a sentence boundary when one exists).
// Empty sections are dropped.
func NormalizeTailored(t *Tailored) {
	itemsLeft := maxResumeItems
	sections := t.Sections[:0]
	for _, s := range t.Sections {
		if len(s.Items) > itemsLeft {
			s.Items = s.Items[:itemsLeft]
		}
		itemsLeft -= len(s.Items)
		for i := range s.Items {
			if len(s.Items[i].Bullets) > maxItemBullets {
				s.Items[i].Bullets = s.Items[i].Bullets[:maxItemBullets]
			}
		}
		if len(s.Items) > 0 {
			sections = append(sections, s)
		}
	}
	t.Sections = sections

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
type CoverLetter struct {
	Greeting   string   `json:"greeting"`
	Paragraphs []string `json:"paragraphs"`
	Closing    string   `json:"closing"`
}

type RecruiterEmail struct {
	Subject    string   `json:"subject"`
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
