# syntax=docker/dockerfile:1
ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /usr/src/app

# Copy the whole module first: `go mod tidy` inspects every import in the
# source tree, so it needs the full project (not just go.mod/go.sum).
COPY . .
RUN go mod tidy

# The main package lives in ./cmd/shellbound, not the repo root.
# modernc.org/sqlite is pure Go, so CGO can stay off -> static binary.
RUN CGO_ENABLED=0 go build -v -o /run-app ./cmd/shellbound


FROM debian:bookworm-slim

# /data is where the SQLite DB and SSH host key live; mount a Fly volume
# here so player accounts and the host key survive restarts/redeploys.
WORKDIR /data

COPY --from=builder /run-app /usr/local/bin/run-app

EXPOSE 80
CMD ["run-app"]
