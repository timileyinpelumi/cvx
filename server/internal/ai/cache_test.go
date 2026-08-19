package ai

import (
	"context"
	"testing"
)

type countingLLM struct {
	calls int
	out   string
}

func (c *countingLLM) GenerateJSON(context.Context, string, []ContentBlock, map[string]any) ([]byte, error) {
	c.calls++
	return []byte(c.out), nil
}

func TestCachedLLMMemoizes(t *testing.T) {
	mem := map[string][]byte{}
	inner := &countingLLM{out: `{"ok":true}`}
	llm := NewCachedLLM(inner, "groq/test",
		func(k string) ([]byte, bool) { v, ok := mem[k]; return v, ok },
		func(k string, v []byte) { mem[k] = v },
	)

	blocks := []ContentBlock{{Text: "hello"}}
	schema := map[string]any{"type": "object"}

	for i := 0; i < 3; i++ {
		out, err := llm.GenerateJSON(context.Background(), "sys", blocks, schema)
		if err != nil || string(out) != `{"ok":true}` {
			t.Fatalf("call %d: %s %v", i, out, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("want 1 upstream call for identical requests, got %d", inner.calls)
	}

	// Any input change must miss: block text, system, schema, PDF bytes.
	llm.GenerateJSON(context.Background(), "sys", []ContentBlock{{Text: "hello2"}}, schema)
	llm.GenerateJSON(context.Background(), "sys2", blocks, schema)
	llm.GenerateJSON(context.Background(), "sys", blocks, map[string]any{"type": "array"})
	llm.GenerateJSON(context.Background(), "sys", []ContentBlock{{Text: "hello", PDF: []byte{1}}}, schema)
	if inner.calls != 5 {
		t.Fatalf("want each changed input to miss, got %d upstream calls", inner.calls)
	}
}

func TestCachedLLMNamespaceSeparatesProviders(t *testing.T) {
	mem := map[string][]byte{}
	get := func(k string) ([]byte, bool) { v, ok := mem[k]; return v, ok }
	put := func(k string, v []byte) { mem[k] = v }

	a := &countingLLM{out: `{"p":"a"}`}
	b := &countingLLM{out: `{"p":"b"}`}
	blocks := []ContentBlock{{Text: "x"}}

	NewCachedLLM(a, "groq/m1", get, put).GenerateJSON(context.Background(), "s", blocks, nil)
	out, _ := NewCachedLLM(b, "openai/m2", get, put).GenerateJSON(context.Background(), "s", blocks, nil)
	if string(out) != `{"p":"b"}` {
		t.Fatalf("namespace collision: got %s", out)
	}
}

// TestCachedLLMBoundaryAmbiguity: length-prefixed hashing means shifting
// bytes between adjacent parts must produce different keys.
func TestCachedLLMBoundaryAmbiguity(t *testing.T) {
	k1 := cacheKey("ns", "ab", []ContentBlock{{Text: "c"}}, nil)
	k2 := cacheKey("ns", "a", []ContentBlock{{Text: "bc"}}, nil)
	if k1 == k2 {
		t.Fatal("adjacent parts collide")
	}
}
