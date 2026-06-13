# Shellbound — terminal MMO over SSH.

BINARY  := shellbound
PKG     := ./cmd/shellbound
HOSTKEY := ./.ssh/shellbound_ed25519

.PHONY: build run test vet tidy hostkey clean

build: ## Compile the server binary
	go build -o $(BINARY) $(PKG)

run: ## Run the server (generates a host key on first start)
	go run $(PKG)

test: ## Run all unit tests
	go test ./...

vet: ## Static checks
	go vet ./...

tidy: ## Resolve dependencies and write go.sum
	go mod tidy

hostkey: ## Pre-generate the ed25519 host key (optional; the server self-generates)
	mkdir -p ./.ssh
	ssh-keygen -t ed25519 -N "" -f $(HOSTKEY)

clean: ## Remove build artifacts (keeps the database)
	rm -f $(BINARY)
