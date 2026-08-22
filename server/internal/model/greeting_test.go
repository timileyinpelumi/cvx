package model

import "testing"

func TestRecruiterGreeting(t *testing.T) {
	cases := []struct{ name, want string }{
		{"Jane Doe", "Hello Jane,"},
		{"jane", "Hello Jane,"},
		{"Ms. Adaeze Okonkwo", "Hello Adaeze,"},
		{"Mary-Jane Watson", "Hello Mary-Jane,"},
		{"", "Hello HR,"},
		{"   ", "Hello HR,"},
		{"the hiring team", "Hello HR,"},
		{"HR", "Hello HR,"},
		{"Recruitment", "Hello HR,"},
		{"jobs@acme.com", "Hello HR,"},
		{"[Hiring Manager]", "Hello HR,"},
	}
	for _, c := range cases {
		if got := RecruiterGreeting(c.name); got != c.want {
			t.Errorf("%q: want %q, got %q", c.name, c.want, got)
		}
	}
}

func TestGreetingUsesTheBestDataAvailable(t *testing.T) {
	cases := []struct {
		name    string
		in      GreetingInput
		variant int
		want    string
	}{
		{"named contact", GreetingInput{RecruiterName: "Jane Doe"}, 0, "Hello Jane,"},
		{"named contact, formal posting", GreetingInput{RecruiterName: "Jane Doe", Tone: ToneFormal}, 0, "Dear Jane,"},
		{"named contact, casual posting", GreetingInput{RecruiterName: "Jane Doe", Tone: ToneCasual}, 0, "Hi Jane,"},
		{"company only", GreetingInput{Company: "Savvy Spender"}, 0, "Hello Savvy Spender hiring team,"},
		{"company with legal suffix", GreetingInput{Company: "Venix Inc"}, 0, "Hello Venix hiring team,"},
		{"placeholder company", GreetingInput{Company: "[Company]"}, 0, "Hello HR,"},
		{"nothing at all", GreetingInput{}, 0, "Hello HR,"},
		{"nothing at all, formal", GreetingInput{Tone: ToneFormal}, 0, "Dear HR,"},
		{"letter with nothing", GreetingInput{Letter: true}, 0, "Dear hiring team,"},
		{"letter with company", GreetingInput{Company: "Venix", Letter: true}, 0, "Dear Venix hiring team,"},
		{"letter stays formal in a casual posting", GreetingInput{RecruiterName: "Jane", Tone: ToneCasual, Letter: true}, 1, "Dear Jane,"},
	}
	for _, c := range cases {
		if got := Greeting(c.in, c.variant); got != c.want {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
}

// The variant is drawn per generation, so it has to be safe for any int and
// has to actually move between the openers a tone allows.
func TestGreetingVariantsStayInRange(t *testing.T) {
	seen := map[string]bool{}
	for v := -5; v < 20; v++ {
		got := Greeting(GreetingInput{Tone: ToneNeutral}, v)
		if got != "Hello HR," && got != "Dear HR," {
			t.Fatalf("variant %d produced %q", v, got)
		}
		seen[got] = true
	}
	if len(seen) != 2 {
		t.Fatalf("neutral tone never varied its opener: %v", seen)
	}
}
