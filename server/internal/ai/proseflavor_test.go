package ai

import (
	"strings"
	"testing"
)

// Every flavor has to be usable as-is: an empty angle, a missing subject
// shape, or a sign-off prose QC would reject makes the whole draw a coin
// flip on failure.
func TestProseFlavorsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range proseFlavors {
		if f.Name == "" || f.Angle == "" || f.Subject == "" {
			t.Errorf("incomplete flavor: %+v", f)
		}
		if seen[f.Name] {
			t.Errorf("duplicate flavor name %q", f.Name)
		}
		seen[f.Name] = true
		if !strings.Contains(f.Subject, "role title") {
			t.Errorf("flavor %q subject shape drops the role title: %s", f.Name, f.Subject)
		}
		for medium, closing := range map[string]string{"email": f.EmailClosing, "letter": f.LetterClosing} {
			if !strings.HasSuffix(closing, ",") {
				t.Errorf("flavor %q %s closing does not end with a comma: %q", f.Name, medium, closing)
			}
			if len(strings.Fields(closing)) > 5 {
				t.Errorf("flavor %q %s closing is longer than a sign-off", f.Name, medium)
			}
			if !closingAllowed(closing, medium == "letter") {
				t.Errorf("flavor %q %s closing is not accepted by prose QC: %q", f.Name, medium, closing)
			}
		}
	}
	if len(proseFlavors) < 3 {
		t.Fatal("too few flavors to vary anything")
	}
}

// A printed letter cannot sign off the way an email can.
func TestLetterClosingsStayFormal(t *testing.T) {
	for _, casual := range []string{"Thanks,", "Best,"} {
		if closingAllowed(casual, true) {
			t.Errorf("%q should not pass on a cover letter", casual)
		}
		if !closingAllowed(casual, false) {
			t.Errorf("%q should pass on an email", casual)
		}
	}
}

// The point of the draw is that two emails from the same profile and posting
// do not come out identical.
func TestPickFlavorVaries(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		seen[pickFlavor().Name] = true
	}
	if len(seen) != len(proseFlavors) {
		t.Fatalf("want every flavor drawn over 200 picks, got %d of %d", len(seen), len(proseFlavors))
	}
}

func TestClosingAllowedRejectsOffMenuSignOffs(t *testing.T) {
	for _, bad := range []string{"Yours faithfully,", "Cheers!", "Warmest regards to you and yours,"} {
		if closingAllowed(bad, false) || closingAllowed(bad, true) {
			t.Errorf("%q should not pass", bad)
		}
	}
}
