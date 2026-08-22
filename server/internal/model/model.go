package model

import (
	"fmt"
	"regexp"
	"sort"
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

// Certification is a credential with a name and, when the source says so,
// who issued it and when. Kept out of Items because it has no bullets and no
// date range: it is a fact, not a body of work.
type Certification struct {
	Name   string `json:"name"`
	Issuer string `json:"issuer"`
	Year   string `json:"year"`
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

	// The material that has no home among Items and that a thin resume
	// needs: standard optional sections, rendered only when the page has
	// room and only from what the profile actually holds.
	Certifications []Certification `json:"certifications"`
	Languages      []string        `json:"languages"`
	Interests      []string        `json:"interests"`
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

// Section kinds. Kind drives page order and what may be trimmed first when
// the page overflows; Title is still the words printed on the page, because
// "Professional experience" and "Selected projects" are the tailor's call.
const (
	SectionExperience     = "experience"
	SectionProjects       = "projects"
	SectionEducation      = "education"
	SectionCertifications = "certifications"
	SectionVolunteering   = "volunteering"
	SectionOther          = "other"
)

// sectionRank fixes the order sections print in, whatever order the model
// returned them. Experience leads unless the user asked for skills first;
// everything else follows in descending order of how much a hiring decision
// turns on it.
var sectionRank = map[string]int{
	SectionExperience:     0,
	SectionProjects:       1,
	SectionEducation:      2,
	SectionCertifications: 3,
	SectionVolunteering:   4,
	SectionOther:          5,
}

type TSection struct {
	Kind  string  `json:"kind"`
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
	// Certifications, Languages and Interests are one-line sections, copied
	// verbatim from the profile. Certifications stand on their own merit;
	// the other two exist to finish a page that real experience does not
	// fill, so they are the first things dropped when it overflows.
	Certifications []string `json:"certifications"`
	Languages      []string `json:"languages"`
	Interests      []string `json:"interests"`
	Gaps           []Gap    `json:"gaps"`
	WhatChanged    []string `json:"whatChanged"`
}

// DateRange renders an item's dates the way the resume prints them, so
// content added in the editor is formatted the same as content the tailor
// selected.
func (i Item) DateRange() string {
	start, end := strings.TrimSpace(i.StartDate), strings.TrimSpace(i.EndDate)
	switch {
	case start != "" && end != "":
		return start + " – " + end
	case start != "":
		return start + " – Present"
	default:
		return end
	}
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

	// Languages and interests carry no ids, so they are guarded the way
	// selectedSkills is: verbatim membership. A resume may not learn a
	// language on the way to the page.
	if err := verbatimSubset("certification", t.Certifications, certificationNames(p)); err != nil {
		return err
	}
	if err := verbatimSubset("language", t.Languages, p.Languages); err != nil {
		return err
	}
	if err := verbatimSubset("interest", t.Interests, p.Interests); err != nil {
		return err
	}
	return nil
}

// appendDeduped appends the entries of add that base does not already hold,
// compared case-insensitively, preserving order.
func appendDeduped(base, add []string) []string {
	have := map[string]bool{}
	for _, b := range base {
		have[strings.ToLower(strings.TrimSpace(b))] = true
	}
	for _, a := range add {
		key := strings.ToLower(strings.TrimSpace(a))
		if key == "" || have[key] {
			continue
		}
		have[key] = true
		base = append(base, strings.TrimSpace(a))
	}
	return base
}

func certificationNames(p Profile) []string {
	out := make([]string, 0, len(p.Certifications))
	for _, c := range p.Certifications {
		out = append(out, c.Name)
	}
	return out
}

func verbatimSubset(label string, got, allowed []string) error {
	have := map[string]bool{}
	for _, a := range allowed {
		have[strings.ToLower(strings.TrimSpace(a))] = true
	}
	for _, g := range got {
		if !have[strings.ToLower(strings.TrimSpace(g))] {
			return fmt.Errorf("tailored output invented a %s not in the profile: %s", label, g)
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
//
// The item and bullet ceilings are deliberately above what fits a page. The
// renderer trims to the page from the bottom of this relevance order, which
// means a thin profile gets everything it has and a deep one still gets
// exactly one page. Clamping to a page-sized guess up here is what left
// real material on the floor and the page half empty.
const (
	maxResumeItems    = 8
	maxItemBullets    = 5
	maxHeadlineChars  = 110
	maxSummaryWords   = MaxSummaryWords
	maxTargetRole     = 64
	maxRoleSummary    = 150
	maxResumeSkills   = 16
	maxCertifications = 6
	maxLanguages      = 6
	maxInterests      = 6
	maxWhatChanged    = 4
	maxGapsListed     = 6
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
	sortSections(t.Sections)

	t.TargetRole = cutAtWord(dropParenthetical(t.TargetRole), maxTargetRole)
	t.RoleSummary = cutAtWord(t.RoleSummary, maxRoleSummary)
	t.Headline = cutAtWord(t.Headline, maxHeadlineChars)
	t.Summary = cutAtSentence(t.Summary, maxSummaryWords)

	if len(t.SelectedSkills) > maxResumeSkills {
		t.SelectedSkills = t.SelectedSkills[:maxResumeSkills]
	}
	if len(t.Certifications) > maxCertifications {
		t.Certifications = t.Certifications[:maxCertifications]
	}
	if len(t.Languages) > maxLanguages {
		t.Languages = t.Languages[:maxLanguages]
	}
	if len(t.Interests) > maxInterests {
		t.Interests = t.Interests[:maxInterests]
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

// sortSections puts the page in reading order by kind, keeping the model's
// order within a kind. Stable so two sections of the same kind stay as the
// tailor ranked them.
func sortSections(sections []TSection) {
	sort.SliceStable(sections, func(i, j int) bool {
		return sectionOrder(sections[i].Kind) < sectionOrder(sections[j].Kind)
	})
}

func sectionOrder(kind string) int {
	if r, ok := sectionRank[strings.ToLower(strings.TrimSpace(kind))]; ok {
		return r
	}
	return sectionRank[SectionOther]
}

// TrimStep names what a Trim call removed, so a caller can log or explain
// why the page holds less than the tailor selected.
type TrimStep string

// MaxTrimSteps bounds the fit loop. Each step is one line or one item; more
// than this many and the content was never going to fit a page.
const MaxTrimSteps = 16

// Trim removes the single least valuable thing left on the resume and
// reports what went, or ("", false) when there is nothing left that may be
// dropped. It is the other half of over-selecting: the tailor ranks more
// material than fits, and the renderer takes the page back down to one page
// from the bottom of that ranking.
//
// The order is the reverse of how much a hiring decision turns on each
// thing: the optional one-line sections first, then the tail of an optional
// section, then the weakest bullet of the fattest item, then the last item.
func (t *Tailored) Trim() (TrimStep, bool) {
	if len(t.Interests) > 0 {
		t.Interests = nil
		return "interests", true
	}
	if len(t.Languages) > 0 {
		t.Languages = nil
		return "languages", true
	}
	// Certifications outrank the other two lines: a credential the posting
	// asks for is evidence, not filler. They still go before any experience.
	if len(t.Certifications) > 0 {
		t.Certifications = nil
		return "certifications", true
	}
	if step, ok := t.trimOptionalSectionTail(); ok {
		return step, true
	}
	if step, ok := t.trimFattestItemBullet(); ok {
		return step, true
	}
	return t.trimLastItem()
}

// trimOptionalSectionTail drops the last item of the last section that is
// not experience: a fourth project earns its place only while the page has
// room for it.
func (t *Tailored) trimOptionalSectionTail() (TrimStep, bool) {
	for i := len(t.Sections) - 1; i >= 0; i-- {
		s := &t.Sections[i]
		if sectionOrder(s.Kind) == sectionRank[SectionExperience] || len(s.Items) == 0 {
			continue
		}
		dropped := s.Items[len(s.Items)-1]
		s.Items = s.Items[:len(s.Items)-1]
		if len(s.Items) == 0 {
			t.Sections = append(t.Sections[:i], t.Sections[i+1:]...)
		}
		return TrimStep(fmt.Sprintf("%s: %s", s.Title, dropped.Title)), true
	}
	return "", false
}

// trimFattestItemBullet takes one bullet from whichever item has the most,
// so the page loses its most repetitive line rather than gutting one item.
func (t *Tailored) trimFattestItemBullet() (TrimStep, bool) {
	var target *TItem
	for si := range t.Sections {
		for ii := range t.Sections[si].Items {
			it := &t.Sections[si].Items[ii]
			if len(it.Bullets) > minItemBullets && (target == nil || len(it.Bullets) > len(target.Bullets)) {
				target = it
			}
		}
	}
	if target == nil {
		return "", false
	}
	target.Bullets = target.Bullets[:len(target.Bullets)-1]
	return TrimStep(fmt.Sprintf("a bullet from %s", target.Title)), true
}

// trimLastItem is the last resort, and it stops at the floor: a resume with
// fewer than minResumeItems items has stopped being a resume.
func (t *Tailored) trimLastItem() (TrimStep, bool) {
	total := 0
	for _, s := range t.Sections {
		total += len(s.Items)
	}
	if total <= minResumeItems {
		return "", false
	}
	for i := len(t.Sections) - 1; i >= 0; i-- {
		s := &t.Sections[i]
		if len(s.Items) == 0 {
			continue
		}
		dropped := s.Items[len(s.Items)-1]
		s.Items = s.Items[:len(s.Items)-1]
		if len(s.Items) == 0 {
			t.Sections = append(t.Sections[:i], t.Sections[i+1:]...)
		}
		return TrimStep(dropped.Title), true
	}
	return "", false
}

// Clone returns a deep copy, so the renderer can trim a resume to the page
// without changing the one that was stored.
func (t Tailored) Clone() Tailored {
	out := t
	out.SelectedSkills = append([]string(nil), t.SelectedSkills...)
	out.Certifications = append([]string(nil), t.Certifications...)
	out.Languages = append([]string(nil), t.Languages...)
	out.Interests = append([]string(nil), t.Interests...)
	out.Gaps = append([]Gap(nil), t.Gaps...)
	out.WhatChanged = append([]string(nil), t.WhatChanged...)
	out.Sections = make([]TSection, len(t.Sections))
	for i, s := range t.Sections {
		s.Items = make([]TItem, len(t.Sections[i].Items))
		for j, it := range t.Sections[i].Items {
			it.Bullets = append([]TBullet(nil), t.Sections[i].Items[j].Bullets...)
			s.Items[j] = it
		}
		out.Sections[i] = s
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

	// The optional material a note can also carry. A resume that ends
	// half-way down the page is usually missing exactly this, and asking the
	// user for it is the only honest way to fill the space.
	NewCertifications []Certification `json:"newCertifications"`
	NewLanguages      []string        `json:"newLanguages"`
	NewInterests      []string        `json:"newInterests"`
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

	p.Languages = appendDeduped(p.Languages, a.NewLanguages)
	p.Interests = appendDeduped(p.Interests, a.NewInterests)

	haveCert := map[string]bool{}
	for _, c := range p.Certifications {
		haveCert[strings.ToLower(strings.TrimSpace(c.Name))] = true
	}
	for _, c := range a.NewCertifications {
		key := strings.ToLower(strings.TrimSpace(c.Name))
		if key == "" || haveCert[key] {
			continue
		}
		haveCert[key] = true
		p.Certifications = append(p.Certifications, c)
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
