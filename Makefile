# Shellbound — terminal MMO over SSH.

BINARY  := shellbound
PKG     := ./cmd/shellbound
HOSTKEY := ./.ssh/shellbound_ed25519

.PHONY: build run start stop status logs test vet tidy hostkey clean service unservice

build: ## Compile the server binary
	go build -o $(BINARY) $(PKG)

run: ## Run the server in the foreground (generates a host key on first start)
	go run $(PKG)

start: ## Start detached so it survives logout (no service); pick a mode with ./scripts/run.sh
	./scripts/run.sh start

stop: ## Stop the detached server
	./scripts/run.sh stop

status: ## Show whether the detached server is running
	./scripts/run.sh status

logs: ## Follow the detached server's log
	./scripts/run.sh logs

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

service: build ## Install + start a systemd unit so the server survives logout & reboot (run as root)
	sed 's#__DIR__#$(CURDIR)#g' deploy/$(BINARY).service > /etc/systemd/system/$(BINARY).service
	systemctl daemon-reload
	systemctl enable $(BINARY)
	systemctl restart $(BINARY)
	@echo "shellbound is running under systemd."
	@echo "  status: systemctl status $(BINARY)"
	@echo "  logs:   journalctl -u $(BINARY) -f"
	@echo "  apply code changes: make service   (rebuilds and restarts)"

unservice: ## Stop and remove the systemd unit (run as root; keeps the database)
	-systemctl disable --now $(BINARY)
	rm -f /etc/systemd/system/$(BINARY).service
	systemctl daemon-reload
