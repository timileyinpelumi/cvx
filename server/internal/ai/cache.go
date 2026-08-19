package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CacheGet returns the cached response for key and whether it existed.
// CachePut stores one. Both are injected (the store owns persistence) so ai
// stays free of database imports.
type (
	CacheGet func(key string) ([]byte, bool)
	CachePut func(key string, response []byte)
)

// cachedLLM memoizes successful GenerateJSON calls keyed on everything that
// determines the response: provider/model namespace, system prompt, every
// content block, and the schema. Identical requests never reach the provider
// twice; any change to the inputs changes the key.
type cachedLLM struct {
	inner LLM
	ns    string
	get   CacheGet
	put   CachePut
}

// NewCachedLLM wraps inner with a persistent response cache. ns names the
// provider/model so switching providers never serves the other's responses.
func NewCachedLLM(inner LLM, ns string, get CacheGet, put CachePut) LLM {
	return cachedLLM{inner: inner, ns: ns, get: get, put: put}
}

func (c cachedLLM) GenerateJSON(ctx context.Context, system string, blocks []ContentBlock, schema map[string]any) ([]byte, error) {
	key := cacheKey(c.ns, system, blocks, schema)
	if resp, ok := c.get(key); ok {
		return resp, nil
	}

	resp, err := c.inner.GenerateJSON(ctx, system, blocks, schema)
	if err != nil {
		return nil, err
	}
	c.put(key, resp)
	return resp, nil
}

func cacheKey(ns, system string, blocks []ContentBlock, schema map[string]any) string {
	h := sha256.New()
	writePart := func(b []byte) {
		var n [8]byte
		l := len(b)
		for i := 0; i < 8; i++ {
			n[i] = byte(l >> (8 * i))
		}
		h.Write(n[:])
		h.Write(b)
	}

	writePart([]byte(ns))
	writePart([]byte(system))
	for _, b := range blocks {
		writePart([]byte(b.Text))
		writePart(b.PDF)
	}
	schemaJSON, _ := json.Marshal(schema)
	writePart(schemaJSON)

	return hex.EncodeToString(h.Sum(nil))
}
