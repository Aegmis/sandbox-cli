BINARY := sandbox-cli
PKG := github.com/Aegmis/sandbox-cli
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION)

# Release matrix for cross-compilation (override: make cross TARGETS="linux/amd64").
TARGETS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build install test test-integration lint fmt clean cross dist docker-build image

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/sandbox-cli

# Cross-compile every target with the local Go toolchain (no Docker needed).
# -> dist/sandbox-cli_<version>_<os>_<arch>[.exe] + SHA256SUMS
cross:
	@mkdir -p dist
	@set -eu; for t in $(TARGETS); do \
	  os="$${t%%/*}"; arch="$${t##*/}"; \
	  case "$$os" in windows) ext=".exe" ;; *) ext="" ;; esac; \
	  name="$(BINARY)_$(VERSION)_$${os}_$${arch}$${ext}"; \
	  echo "==> $$name"; \
	  CGO_ENABLED=0 GOOS="$$os" GOARCH="$$arch" \
	    go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o "dist/$$name" ./cmd/sandbox-cli; \
	done; \
	cd dist && sha256sum $(BINARY)_* > SHA256SUMS

# Same matrix, but inside Docker (hermetic; no local Go toolchain required).
dist:
	docker build --target dist --build-arg VERSION=$(VERSION) \
	  --build-arg TARGETS="$(TARGETS)" --output type=local,dest=./dist .

# One binary for this machine, built in Docker. -> bin/sandbox-cli
docker-build:
	docker build --target export --build-arg VERSION=$(VERSION) \
	  --output type=local,dest=./bin .

# Multi-arch runnable image (requires buildx).
image:
	docker buildx build --target runtime --build-arg VERSION=$(VERSION) \
	  --platform linux/amd64,linux/arm64 -t $(BINARY):$(VERSION) .

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/sandbox-cli

test:
	go test ./...

# Requires a running Docker daemon; builds the base image on first run.
test-integration:
	go test -tags docker_integration -count=1 ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin dist bin-docker
