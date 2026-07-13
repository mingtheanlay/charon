# charon — build & install
BINARY  := charon
PKG     := ./cmd/charon
# Version from git tag if available, else "dev".
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
# Install location (no sudo needed by default).
PREFIX  ?= $(HOME)/.local

.PHONY: build install uninstall test cover lint fmt tidy clean run

build: ## Build ./charon for the current platform
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install: ## Build and install to $(PREFIX)/bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)
	install -d "$(PREFIX)/bin"
	install -m 0755 $(BINARY) "$(PREFIX)/bin/$(BINARY)"
	@echo "Installed $(BINARY) $(VERSION) -> $(PREFIX)/bin/$(BINARY)"

uninstall: ## Remove the installed binary
	rm -f "$(PREFIX)/bin/$(BINARY)"

test: ## Run go vet + race tests
	go vet ./...
	go test -race ./...

cover: ## Run tests with a coverage summary
	go test -coverprofile=coverage.txt ./...
	go tool cover -func=coverage.txt | tail -1

lint: ## Run golangci-lint (install: https://golangci-lint.run)
	golangci-lint run

fmt: ## Format all Go files
	gofmt -w .

tidy: ## Tidy module dependencies
	go mod tidy

clean: ## Remove the local build artifact
	rm -f $(BINARY)

run: ## Build and run the interactive menu
	go run $(PKG)
