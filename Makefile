BINARY := sandbox
PKG := github.com/amitghadge/sandbox-cli
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION)

.PHONY: build install test test-integration lint fmt clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/sandbox

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/sandbox

test:
	go test ./...

# Requires a running Docker daemon; builds the base image on first run.
test-integration:
	go test -tags docker_integration -count=1 ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
