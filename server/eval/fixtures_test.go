package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"cvx/internal/model"
)

type fixtureEntry struct {
	ID    string `json:"id"`
	File  string `json:"file"`
	Title string `json:"title"`
}

func loadIndex(t *testing.T) []fixtureEntry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("fixtures", "index.json"))
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	var entries []fixtureEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatalf("unmarshal index.json: %v", err)
	}
	return entries
}

func TestIndexHasTwelveEntries(t *testing.T) {
	entries := loadIndex(t)
	if len(entries) != 12 {
		t.Fatalf("want 12 fixtures, got %d", len(entries))
	}
	for _, e := range entries {
		if e.ID == "" || e.File == "" || e.Title == "" {
			t.Fatalf("incomplete index entry: %+v", e)
		}
	}
}

func TestJDFilesExistAndAreLongEnough(t *testing.T) {
	entries := loadIndex(t)
	for _, e := range entries {
		e := e
		t.Run(e.ID, func(t *testing.T) {
			path := filepath.Join("fixtures", e.File)
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			words := strings.Fields(string(b))
			if len(words) < 400 {
				t.Fatalf("%s has %d words, want >= 400", e.File, len(words))
			}
		})
	}
}

func loadProfile(t *testing.T) model.Profile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("fixtures", "profile.json"))
	if err != nil {
		t.Fatalf("read profile.json: %v", err)
	}
	var p model.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("unmarshal profile.json: %v", err)
	}
	return p
}

func TestProfileShape(t *testing.T) {
	p := loadProfile(t)

	if p.Name == "" || p.Email == "" {
		t.Fatalf("profile missing name/email: %+v", p)
	}
	if len(p.Items) != 6 {
		t.Fatalf("want 6 items, got %d", len(p.Items))
	}
	if len(p.Skills) != 18 {
		t.Fatalf("want 18 skills, got %d", len(p.Skills))
	}
	for _, it := range p.Items {
		if len(it.Bullets) < 3 || len(it.Bullets) > 5 {
			t.Fatalf("item %s has %d bullets, want 3-5", it.ID, len(it.Bullets))
		}
		for _, b := range it.Bullets {
			if len(b.Skills) == 0 {
				t.Fatalf("bullet %s has no skills tags", b.ID)
			}
		}
	}
}

func TestProfileIDsAreCanonical(t *testing.T) {
	p := loadProfile(t)

	// Recompute ids on a deep copy so mutation doesn't affect the loaded
	// profile, then compare against what's actually committed in the fixture.
	want := deepCopyProfile(p)
	for i := range want.Items {
		want.Items[i].ID = ""
		for j := range want.Items[i].Bullets {
			want.Items[i].Bullets[j].ID = ""
		}
	}
	model.AssignIDs(&want)

	if !reflect.DeepEqual(p, want) {
		for i := range p.Items {
			if p.Items[i].ID != want.Items[i].ID {
				t.Errorf("item %d: id = %q, want %q", i, p.Items[i].ID, want.Items[i].ID)
			}
			for j := range p.Items[i].Bullets {
				if p.Items[i].Bullets[j].ID != want.Items[i].Bullets[j].ID {
					t.Errorf("item %d bullet %d: id = %q, want %q", i, j, p.Items[i].Bullets[j].ID, want.Items[i].Bullets[j].ID)
				}
			}
		}
		t.Fatalf("profile.json ids are not AssignIDs-canonical")
	}
}

func deepCopyProfile(p model.Profile) model.Profile {
	b, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	var cp model.Profile
	if err := json.Unmarshal(b, &cp); err != nil {
		panic(err)
	}
	return cp
}
