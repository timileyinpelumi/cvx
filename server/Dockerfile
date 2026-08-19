FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# modernc.org/sqlite is pure Go, so the binary is fully static.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /cvx ./cmd/cvx

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H cvx
WORKDIR /app
COPY --from=build /cvx /app/cvx
# data/ is the SQLite volume mount; the app creates data/cvx.db inside it.
RUN mkdir -p /app/data && chown -R cvx /app
USER cvx
EXPOSE 8080
CMD ["/app/cvx"]
