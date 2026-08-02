# cvx

Paste a role → tailored one-page PDF resume from your canonical profile. Personal tool, single user.

- `server/`: Go + Echo. Run: `cd server && go run ./cmd/cvx`. Test: `go test ./...`. SQLite at `server/data/cvx.db`, raw SQL, no ORM.
- `web/`: Next.js UI (Bun only). Proxies `/api/*` to :8080.
- LLM: swappable via CVX_LLM_PROVIDER (groq default | openai | anthropic | agentrouter), structured outputs (JSON schema) on every provider. Guardrail is provider-independent.
- Hard guardrail: tailored output may only cite profile content by id (`ValidateTailored`). Rephrase/reorder, never invent.
