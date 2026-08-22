# Deploying cvx

One unit: the Go API and the Next server ship in one image and talk over
loopback, with SQLite on a disk beside them. `docker compose up` is the
whole deployment, on a laptop or on one small AWS instance.

## Why not Vercel

Vercel runs functions with an ephemeral filesystem. SQLite needs a file that
survives a restart and a single writer, so a Vercel deployment would lose
every profile and resume on each cold start. Keeping SQLite means keeping one
long-running host. If you ever want Vercel, the database has to move first
(libSQL/Turso is the closest thing to "still SQLite"), which is a bigger
change than this deployment.

## AWS, the small way

One instance, one disk, no orchestration. EC2 `t4g.small` or a Lightsail
instance, both fine; arm64 works, the image builds for it.

1. Launch the instance with a 20GB volume, ports 80 and 443 open, 22 to your
   IP only.
2. Point your domain's A record at its static IP (allocate an Elastic IP so
   it survives a stop).
3. On the instance:

```
sudo dnf install -y docker git            # or apt install on Ubuntu
sudo systemctl enable --now docker
sudo usermod -aG docker $USER             # log out and back in

git clone <your repo> cvx && cd cvx
cp .env.example .env && $EDITOR .env      # see below
docker compose up -d --build
```

Caddy gets the certificate on first request. `docker compose logs -f` shows
both halves starting.

## Environment

`.env` next to the compose file:

```
CVX_DOMAIN=cvx.example.com
CVX_BASE_URL=https://cvx.example.com
CVX_SESSION_SECRET=<openssl rand -hex 32>
CVX_ALLOWED_EMAILS=you@example.com

GROQ_API_KEY=...
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
RESEND_API_KEY=...
CVX_EMAIL_FROM=cvx <hello@cvx.example.com>
```

`CVX_ALLOWED_EMAILS` is not optional in practice: without it, anyone with a
Google or GitHub account can sign in. `CVX_ENV=production` is set by the
compose file; it hard-refuses `CVX_DEV_USER` and marks cookies Secure.

Register both callback URLs with the providers:

- `https://<domain>/auth/google/callback`
- `https://<domain>/auth/github/callback`

## Data

SQLite lives in the `cvx-data` volume at `/app/data/cvx.db`. Migrations run
on startup.

Back it up without stopping anything. The binary has a one-shot mode that
takes SQLite's own consistent snapshot, so you never copy a half-written
file:

```
docker compose exec app /app/cvx -backup /app/data/backup-$(date +%F).db
docker compose cp app:/app/data/backup-$(date +%F).db .
```

Worth a cron entry on the host, with the copy pushed to S3.

One volume means one writer: never run two app replicas against it.

## Updating

```
git pull
docker compose up -d --build
```

The container replaces itself; the volume stays. If either process inside
dies, the container exits and Docker restarts it, so you never get a live web
tier in front of a dead API.

## Local

`cd server && go run ./cmd/cvx` and `cd web && bun run dev` is still the
development loop. To run the shipping image without the proxy:

```
docker build -t cvx . && docker run --rm -p 3000:3000 --env-file server/.env cvx
```
