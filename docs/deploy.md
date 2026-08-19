# Deploying cvx

Web on Vercel, API (Go + SQLite) on Fly.io. The browser only ever talks to the
Vercel origin; Next rewrites proxy `/api`, `/auth`, and the PDF routes to the
API, so cookies and OAuth stay same-origin.

## API (Fly.io) — from `server/`

```sh
fly launch --no-deploy        # accepts fly.toml; pick the app name
fly volumes create cvx_data --size 1
fly secrets set \
  GROQ_API_KEY=... \
  CVX_SESSION_SECRET=... \
  GOOGLE_CLIENT_ID=... GOOGLE_CLIENT_SECRET=... \
  GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... \
  RESEND_API_KEY=... \
  CVX_ALLOWED_EMAILS=you@example.com
fly deploy
```

Set `CVX_BASE_URL` in fly.toml to the public (Vercel) domain — OAuth
callbacks flow through the proxy. `CVX_ENV=production` is already set there:
it hard-refuses `CVX_DEV_USER` and marks cookies Secure.

## Web (Vercel) — from `web/`

Project env vars:

- `CVX_API_ORIGIN` = `https://<fly-app>.fly.dev` (rewrites are built with it)
- `NEXT_PUBLIC_BASE_URL` = `https://<vercel-domain>` (absolute OG/meta URLs)

Then `vercel deploy --prod` or connect the repo.

## One-time account work

- Rotate the Groq key and both OAuth client secrets (they passed through chat).
- Add production callback URLs on both OAuth apps:
  `https://<domain>/auth/google/callback`, `https://<domain>/auth/github/callback`.
- Verify a sending domain in Resend and set `CVX_EMAIL_FROM`; until then
  emails only deliver to the Resend account owner's address.
- Set `CVX_ALLOWED_EMAILS` so only your accounts can sign in.
