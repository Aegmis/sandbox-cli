# Multi-stage build for the `sandbox` binary itself.
#
# Note: this builds/ships the CLI. At runtime `sandbox` shells out to the host
# `docker` CLI, so to actually run it from a container you'd mount the docker
# socket and provide a docker client — but the common use of this image is just
# to produce the binary (see the `export` note at the bottom).

# ---- build stage ----
FROM golang:1.25-bookworm AS build

# Version stamped into the binary (override: --build-arg VERSION=v1.2.3).
ARG VERSION=dev

WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

# Source (includes internal/image/assets/Dockerfile needed by //go:embed).
COPY . .

# Static, stripped binary. CGO off so it runs on any linux/* base.
RUN CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "-s -w -X github.com/amitghadge/sandbox-cli/internal/version.Version=${VERSION}" \
      -o /out/sandbox ./cmd/sandbox

# ---- runtime stage ----
# Includes the docker CLI so `sandbox` can talk to a mounted docker socket.
FROM docker:cli AS runtime
COPY --from=build /out/sandbox /usr/local/bin/sandbox
ENTRYPOINT ["sandbox"]

# ---- export stage ----
# Minimal scratch image carrying only the binary, for extracting it out:
#   docker build --target export --output type=local,dest=./bin .
# -> ./bin/sandbox
FROM scratch AS export
COPY --from=build /out/sandbox /sandbox
