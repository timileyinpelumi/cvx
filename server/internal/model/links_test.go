package model

import "testing"

func TestClassifyLink(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/timileyin", LinkGitHub},
		{"github.com/timileyin/", LinkGitHub},
		{"https://github.com/timileyin/chainpal", LinkOther},
		{"https://gist.github.com/timileyin/abc123", LinkOther},
		{"https://www.linkedin.com/in/timileyin", LinkLinkedIn},
		{"https://linkedin.com/company/acme", LinkOther},
		{"https://timileyin.dev", LinkPortfolio},
		{"https://timileyin.dev/projects/spawn", LinkOther},
		{"https://spawn.vercel.app", LinkOther},
		{"https://medium.com/@t/post", LinkOther},
		{"", LinkOther},
	}
	for _, c := range cases {
		if got := ClassifyLink(Link{URL: c.url}); got != c.want {
			t.Errorf("%q: want %s, got %s", c.url, c.want, got)
		}
	}
}

// The contact row is the exact failure from the first shipped resumes: every
// project link the digitizer found ended up under the candidate's name.
func TestIdentityLinksKeepsIdentitiesAndDropsProjects(t *testing.T) {
	p := Profile{Links: []Link{
		{Label: "ChainPal", URL: "https://github.com/timileyin/chainpal"},
		{Label: "FUTA", URL: "https://futa.edu.ng/alumni/timileyin"},
		{Label: "Personal Website", URL: "https://timileyin.dev"},
		{Label: "TweetStream", URL: "https://tweetstream.vercel.app"},
		{Label: "PromptSifter Gist", URL: "https://gist.github.com/timileyin/x"},
		{Label: "GitHub", URL: "https://github.com/timileyin"},
		{Label: "LinkedIn", URL: "https://linkedin.com/in/timileyin"},
	}}

	got := IdentityLinks(p)
	want := []string{"github.com/timileyin", "linkedin.com/in/timileyin", "timileyin.dev"}
	if len(got) != len(want) {
		t.Fatalf("want %d links, got %d: %+v", len(want), len(got), got)
	}
	for i, l := range got {
		if LinkDisplay(l) != want[i] {
			t.Errorf("position %d: want %s, got %s", i, want[i], LinkDisplay(l))
		}
	}
}

func TestIdentityLinksOnePerKindAndCapped(t *testing.T) {
	p := Profile{Links: []Link{
		{URL: "https://github.com/first"},
		{URL: "https://github.com/second"},
		{URL: "https://linkedin.com/in/a"},
		{URL: "https://a.dev"},
		{URL: "https://b.dev"},
	}}
	got := IdentityLinks(p)
	if len(got) > maxIdentityLinks {
		t.Fatalf("contact row over cap: %+v", got)
	}
	if LinkDisplay(got[0]) != "github.com/first" {
		t.Fatalf("want the first github link kept, got %s", LinkDisplay(got[0]))
	}
}
