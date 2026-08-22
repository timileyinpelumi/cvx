package pdfgen

import "fmt"

// Style is the user's saved resume appearance: which theme, which accent
// color, how dense, and whether skills lead. Zero value is not valid on its
// own — read paths call Normalized, write paths call Validate.
type Style struct {
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`
	Density     string `json:"density"`
	SkillsFirst bool   `json:"skillsFirst"`
	// HidePhone and HideLocation keep contact details off the page. Resumes
	// get posted to job boards and forwarded on, and a phone number or a
	// home city on a public document is not something you can take back.
	HidePhone    bool `json:"hidePhone"`
	HideLocation bool `json:"hideLocation"`
}

var Accents = []string{"#1C2422", "#2244D9", "#0F766E", "#7C2D92", "#B3341E", "#B07818"}

var themes = map[string]bool{"classic": true, "modern": true, "compact": true}
var densities = map[string]bool{"normal": true, "tight": true}

func DefaultStyle() Style {
	return Style{Theme: "modern", Accent: "#2244D9", Density: "normal"}
}

func (s Style) Validate() error {
	if !themes[s.Theme] {
		return fmt.Errorf("unknown theme %q", s.Theme)
	}
	if !densities[s.Density] {
		return fmt.Errorf("unknown density %q", s.Density)
	}
	for _, a := range Accents {
		if s.Accent == a {
			return nil
		}
	}
	return fmt.Errorf("unknown accent %q", s.Accent)
}

func (s Style) Normalized() Style {
	d := DefaultStyle()
	if themes[s.Theme] {
		d.Theme = s.Theme
	}
	if densities[s.Density] {
		d.Density = s.Density
	}
	for _, a := range Accents {
		if s.Accent == a {
			d.Accent = a
		}
	}
	d.SkillsFirst = s.SkillsFirst
	d.HidePhone, d.HideLocation = s.HidePhone, s.HideLocation
	return d
}

// theme is the resolved render configuration. Everything the pipeline draws
// with comes from here; no theme conditionals live outside resolveTheme
// except Compact's two-column skills.
type theme struct {
	displayFamily string
	bodyFamily    string
	marginSide    float64
	marginTop     float64
	marginBottom  float64

	namePt     float64
	headlinePt float64
	contactPt  float64
	summaryPt  float64
	sectionPt  float64
	itemPt     float64
	datePt     float64
	bulletPt   float64
	skillsPt   float64

	leading         float64
	gapSection      float64
	gapItems        float64
	gapBullets      float64
	sectionTitlePad float64
	orphanMinRemain float64

	accentR, accentG, accentB int
	nameInAccent              bool
	titlesInAccent            bool
	rulesInAccent             bool
	twoColSkills              bool

	hidePhone    bool
	hideLocation bool
}

func hexRGB(hex string) (int, int, int) {
	var r, g, b int
	fmt.Sscanf(hex, "#%02X%02X%02X", &r, &g, &b)
	return r, g, b
}

func resolveTheme(s Style) theme {
	s = s.Normalized()
	r, g, b := hexRGB(s.Accent)

	t := theme{
		displayFamily: fontFamily,
		bodyFamily:    fontFamily,
		marginSide:    18, marginTop: 16, marginBottom: 16,
		namePt: 19, headlinePt: 10.5, contactPt: 8.5, summaryPt: 9.5,
		sectionPt: 10.5, itemPt: 10, datePt: 9.5, bulletPt: 10, skillsPt: 10,
		leading: 1.35, gapSection: 6, gapItems: 3, gapBullets: 1.2,
		sectionTitlePad: 2, orphanMinRemain: 30,
		accentR: r, accentG: g, accentB: b,
		nameInAccent: true, titlesInAccent: true, rulesInAccent: false,
		hidePhone: s.HidePhone, hideLocation: s.HideLocation,
	}

	switch s.Theme {
	case "classic":
		t.displayFamily = serifFamily
		t.marginSide, t.marginTop, t.marginBottom = 20, 18, 18
		t.namePt, t.sectionPt = 20, 11
		t.headlinePt, t.itemPt, t.bulletPt, t.skillsPt = 10, 10, 9.5, 9.5
		t.leading = 1.4
		t.sectionTitlePad = 0
		t.nameInAccent, t.titlesInAccent, t.rulesInAccent = false, false, true
	case "compact":
		t.marginSide, t.marginTop, t.marginBottom = 16, 14, 14
		t.namePt, t.sectionPt = 17, 9.5
		t.headlinePt, t.summaryPt = 9.5, 9
		t.itemPt, t.datePt, t.bulletPt, t.skillsPt = 9.5, 9, 9.5, 9.5
		t.leading = 1.25
		t.gapSection, t.gapItems, t.gapBullets = 4.5, 2.2, 0.9
		t.sectionTitlePad = 0
		t.orphanMinRemain = 24
		t.nameInAccent = false
		t.twoColSkills = true
	}

	// Tight has to read as a different sheet at a glance, not a nudge.
	if s.Density == "tight" {
		t.leading *= 0.88
		t.gapSection *= 0.65
		t.gapItems *= 0.65
		t.gapBullets *= 0.7
		t.sectionTitlePad *= 0.5
		t.marginTop -= 2
		t.marginBottom -= 2
	}

	return t
}
