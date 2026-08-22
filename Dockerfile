# One container: the Go API and the Next.js server, behind a single port.
#
# They were two deployments with the web tier proxying to the API.
# Same-origin was the reason, and it still is — but a loopback proxy inside
# one image gives the same cookies and OAuth behaviour with one thing to
# deploy, one place for secrets, and no cross-host hop per request.

# --- the API -----------------------------------------------------------
FROM golang:1.25-alpine AS api
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
# modernc.org/sqlite is pure Go, so the binary is fully static.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /cvx ./cmd/cvx

# --- the web app -------------------------------------------------------
FROM oven/bun:1-alpine AS web
WORKDIR /src
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
# The API is a loopback hop now, so the rewrites are built against localhost.
ENV CVX_API_ORIGIN=http://127.0.0.1:8080
ENV NEXT_TELEMETRY_DISABLED=1
RUN bun run build

# --- what actually ships ----------------------------------------------
FROM node:22-alpine
RUN apk add --no-cache ca-certificates && adduser -D -H cvx
WORKDIR /app

COPY --from=api /cvx /app/cvx
COPY --from=web /src/.next/standalone ./web/
COPY --from=web /src/.next/static ./web/.next/static
# No public/ directory: the icon, manifest and OG image are all routes.

# data/ is the SQLite volume mount; the app creates data/cvx.db inside it.
RUN mkdir -p /app/data && chown -R cvx /app
COPY docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

USER cvx
ENV NODE_ENV=production
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
ENV CVX_ADDR=127.0.0.1:8080
EXPOSE 3000
CMD ["/app/entrypoint.sh"]
