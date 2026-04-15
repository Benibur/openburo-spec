# OpenBuro server — developer commands

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/openburo/openburo-server/internal/version.Version=$(VERSION)

BIN := bin/openburo-server

.PHONY: all build run test lint fmt ci clean help

all: build

build: ## Compile the server binary to bin/openburo-server
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

run: ## Run the server with ./config.yaml
	go run ./cmd/server -config config.yaml

test: ## Run tests with the race detector
	go test ./... -race -count=1

lint: ## Run gofmt check, go vet, and staticcheck
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt issues in:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

fmt: ## Rewrite files with gofmt
	gofmt -w .

ci: lint test build ## Run the full CI pipeline locally

clean: ## Remove build artifacts
	rm -rf bin/

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
