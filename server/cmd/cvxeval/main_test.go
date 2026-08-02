package main

import (
	"reflect"
	"testing"
)

func TestDefaultJudgeIsStructuredOutputCapable(t *testing.T) {
	// Groq only supports strict json_schema Structured Outputs (which
	// ai.LLM.GenerateJSON always requests) for openai/gpt-oss-20b and
	// openai/gpt-oss-120b — see https://console.groq.com/docs/structured-outputs.
	// The default judge must be on that list, and groq is the only provider
	// guaranteed configured (see plan Global Constraints).
	if defaultJudgeProvider != "groq" {
		t.Fatalf("want default judge provider groq, got %q", defaultJudgeProvider)
	}
	supported := map[string]bool{"openai/gpt-oss-20b": true, "openai/gpt-oss-120b": true}
	if !supported[defaultJudgeModel] {
		t.Fatalf("default judge model %q is not on Groq's Structured Outputs strict-mode list", defaultJudgeModel)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,, c ", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}
