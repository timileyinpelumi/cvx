package ai

import (
	"context"
	"strings"
	"testing"
)

func TestCoverLetterSchemaRequiredFields(t *testing.T) {
	req, ok := coverLetterSchema["required"].([]string)
	if !ok {
		t.Fatalf("required is not []string: %v", coverLetterSchema["required"])
	}
	want := []string{"greeting", "paragraphs", "closing"}
	if len(req) != len(want) {
		t.Fatalf("required = %v, want %v", req, want)
	}
	for i, r := range want {
		if req[i] != r {
			t.Fatalf("required = %v, want %v", req, want)
		}
	}
	if coverLetterSchema["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %v, want false", coverLetterSchema["additionalProperties"])
	}
	props, ok := coverLetterSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties is not a map: %v", coverLetterSchema["properties"])
	}
	for _, key := range want {
		if _, ok := props[key]; !ok {
			t.Fatalf("properties missing %q", key)
		}
	}
}

func TestCoverLetterValid(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: `{"greeting":"Dear hiring team,","paragraphs":["I build backend systems in Python.","I would welcome the chance to contribute."],"closing":"Sincerely,"}`}

	cl, err := CoverLetter(context.Background(), f, p, "Python Backend Engineer")
	if err != nil {
		t.Fatal(err)
	}
	if cl.Greeting != "Dear hiring team," || len(cl.Paragraphs) != 2 || cl.Closing != "Sincerely," {
		t.Fatalf("%+v", cl)
	}
}

func TestCoverLetterCallShape(t *testing.T) {
	p := digitizedSample()
	f := &fakeLLM{out: `{"greeting":"Dear hiring team,","paragraphs":["p1"],"closing":"Sincerely,"}`}

	if _, err := CoverLetter(context.Background(), f, p, "Python Backend Engineer"); err != nil {
		t.Fatal(err)
	}

	if f.system == "" {
		t.Fatal("expected a system prompt")
	}
	for _, phrase := range []string{
		"em dash", "exclamation", "sentence case", "[Company]", "profile",
		// v2 specificity rules.
		"could only be written about this",
		"over a gap with enthusiasm",
	} {
		if !strings.Contains(f.system, phrase) {
			t.Fatalf("system prompt missing %q:\n%s", phrase, f.system)
		}
	}

	if len(f.blocks) != 2 {
		t.Fatalf("want 2 blocks (profile JSON + role), got %d", len(f.blocks))
	}
	if !strings.Contains(f.blocks[0].Text, "Ada") {
		t.Fatalf("expected profile JSON in first block, got %q", f.blocks[0].Text)
	}
	if !strings.Contains(f.blocks[1].Text, "Python Backend Engineer") {
		t.Fatalf("expected role in second block, got %q", f.blocks[1].Text)
	}
}
