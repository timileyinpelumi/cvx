# Deploying cvx

One container, one deploy: the Go API and the Next.js server ship in the
same image and talk over loopback, so cookies and OAuth stay same-origin
without a second platform in the path.

## One time

1. Rotate anything that has been in a `.env`: the Groq key, and both OAuth
   client secrets.
2. Pick the production domain and set `CVX_BASE_URL` in `fly.toml` to it.
   Every OAuth callback and every link in an email is built from it.
3. Register the callback URLs with both providers:
   - `https://<domain>/auth/google/callback`
   - `https://<domain>/auth/github/callback`
4. `fly launch --no-deploy` (from the repo root), then
   `fly volumes create cvx_data --size 1`.

## Secrets

```
fly secrets set \
  GROQ_API_KEY=... \
  CVX_SESSION_SECRET=$(openssl rand -hex 32) \
  GOOGLE_CLIENT_ID=... GOOGLE_CLIENT_SECRET=... \
  GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... \
  RESEND_API_KEY=... \
  CVX_ALLOWED_EMAILS=you@example.com
```

`CVX_ALLOWED_EMAILS` is not optional in practice: without it, anyone with a
Google or GitHub account can sign in. `CVX_ENV=production` is already set in
`fly.toml`; it hard-refuses `CVX_DEV_USER` and marks cookies Secure.

## Deploy

```
fly deploy
```

The image builds the API, builds the web app in standalone mode, and runs
both from `docker/entrypoint.sh`. If either process exits the container
exits, so Fly restarts a whole instance rather than leaving a half-serving
one up. `/healthz` is proxied through the web tier to the API, so a passing
check means both halves are alive.

## Data

SQLite lives on the `cvx_data` volume at `/app/data/cvx.db`. Migrations run
on startup. One volume means one machine: keep `min_machines_running` at 0
or 1, and do not scale the app out.

## Email

Verify a sending domain in Resend and set `CVX_EMAIL_FROM`. Until then
Resend's sandbox sender only delivers to your own address, which is enough
to test the welcome, farewell, and resume emails.

## Local

`cd server && go run ./cmd/cvx` and `cd web && bun run dev` is still the
development loop. To check the shipping image:

```
docker build -t cvx . && docker run --rm -p 3000:3000 --env-file server/.env cvx
```
