.PHONY: all build test test-cover lint fmt tidy deps check install uninstall release snapshot smoke clean

BINARY := granola
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-s -w \
	-X github.com/open-cli-collective/granola-cli/internal/version.Version=$(VERSION) \
	-X github.com/open-cli-collective/granola-cli/internal/version.Commit=$(COMMIT) \
	-X github.com/open-cli-collective/granola-cli/internal/version.Date=$(DATE)"

# Standard keyring opt-out tags (cli-common working-with-secrets.md §1.10):
# exclude the 1Password and passage backends credstore never exposes.
export GOFLAGS := -tags=keyring_no1password,keyring_nopassage

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

deps:
	go mod download
	go mod verify

check: tidy fmt lint test build

smoke: build
	./$(BINARY) version
	./$(BINARY) --help > /dev/null
	for grp in account config folders login meetings tools; do \
		./$(BINARY) $$grp --help > /dev/null 2>&1 || exit 1; \
	done

install: build
	mkdir -p $(HOME)/.local/bin
	cp $(BINARY) $(HOME)/.local/bin/

uninstall:
	rm -f $(HOME)/.local/bin/$(BINARY)

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY)
	rm -rf dist/
	rm -f coverage.out coverage.html
