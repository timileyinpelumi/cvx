# cvx

Paste a role → tailored one-page PDF resume from your canonical profile. Personal tool, single user.

- `server/`: Go + Echo. Run: `cd server && go run ./cmd/cvx`. Test: `go test ./...`. SQLite at `server/data/cvx.db`, raw SQL, no ORM.
- `web/`: Next.js UI (Bun only). Proxies `/api/*` to :8080.
- Claude: `claude-opus-5`, structured outputs (JSON schema) only.
- Hard guardrail: tailored output may only cite profile content by id (`ValidateTailored`). Rephrase/reorder, never invent.
