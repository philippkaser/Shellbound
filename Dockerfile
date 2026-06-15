# ---------- Build stage ----------
FROM golang:1.23 AS builder

WORKDIR /app

# Download dependencies first (better layer caching)
COPY go.mod ./
RUN go mod tidy

# Copy source
COPY . .

# Build static Linux binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o shellbound ./cmd/shellbound

# ---------- Runtime stage ----------
FROM alpine:3.21

RUN apk --no-cache add ca-certificates

WORKDIR /data

# Copy binary
COPY --from=builder /app/shellbound /usr/local/bin/shellbound

# Persistent data locations
ENV SHELLBOUND_ADDR=:80
ENV SHELLBOUND_DB=/data/shellbound.db
ENV SHELLBOUND_HOSTKEY=/data/.ssh/shellbound_ed25519

# Create hostkey directory
RUN mkdir -p /data/.ssh

# Database + SSH host key persistence
VOLUME ["/data"]

EXPOSE 80

ENTRYPOINT ["shellbound"]