FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" \
    -o shellbound \
    ./cmd/shellbound

FROM alpine:3.21

RUN apk --no-cache add ca-certificates

WORKDIR /data

COPY --from=builder /app/shellbound /usr/local/bin/shellbound

ENV SHELLBOUND_ADDR=:80
ENV SHELLBOUND_DB=/data/shellbound.db
ENV SHELLBOUND_HOSTKEY=/data/.ssh/shellbound_ed25519

RUN mkdir -p /data/.ssh

VOLUME ["/data"]

EXPOSE 80

ENTRYPOINT ["shellbound"]