# syntax=docker/dockerfile:1.7
#
# Cross-platform build for the `sandbox-cli` binary.
#
# The compiler always runs on the BUILD platform and cross-compiles via
# GOOS/GOARCH (CGO is off, so no cross toolchain is needed). A multi-platform
# build therefore never pays for QEMU emulation — producing the linux/arm64
# artifact on an amd64 host is as fast as a native build, and vice versa.
#
# Common uses:
#
#   # every supported OS/arch into ./dist, plus SHA256SUMS
#   docker build --target dist --output type=local,dest=./dist .
#
#   # one binary for this machine into ./bin
#   docker build --target export --output type=local,dest=./bin .
#
#   # one binary for an explicit platform
#   docker build --target export --platform linux/arm64 \
#     --output type=local,dest=./bin .
#
#   # a runnable multi-arch image (needs buildx; --push for a registry)
#   docker buildx build --target runtime \
#     --platform linux/amd64,linux/arm64 -t sandbox-cli:dev .
#
#   # stamp a real version (the .git dir is not in the build context)
#   docker build --target dist --build-arg VERSION="$(git describe --tags --always)" \
#     --output type=local,dest=./dist .

ARG GO_VERSION=1.25

# ---- builder base -----------------------------------------------------------
# --platform=$BUILDPLATFORM pins this stage to the machine doing the building,
# so the toolchain runs natively no matter which target we emit.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS base

WORKDIR /src

# Module downloads cache independently of source edits.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Source. Includes internal/image/assets/Dockerfile, which //go:embed needs.
COPY . .

# Static, dependency-free binaries on every target.
ENV CGO_ENABLED=0

# ---- single-target binary ---------------------------------------------------
# TARGETOS/TARGETARCH/TARGETVARIANT are supplied by BuildKit and follow
# --platform; with no --platform they describe the host.
FROM base AS binary
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    # GOARM only matters for 32-bit arm (v6/v7); it is ignored elsewhere.
    export GOARM="$(printf '%s' "${TARGETVARIANT}" | tr -d 'v')"; \
    GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath \
      -ldflags "-s -w -X github.com/Aegmis/sandbox-cli/internal/version.Version=${VERSION}" \
      -o /out/sandbox-cli ./cmd/sandbox-cli

# ---- release matrix ---------------------------------------------------------
# Cross-compiles every target in one pass and writes SHA256SUMS alongside.
# Narrow it with --build-arg TARGETS="linux/amd64 darwin/arm64".
FROM base AS release
ARG VERSION=dev
ARG TARGETS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    mkdir -p /out; \
    for t in ${TARGETS}; do \
      os="${t%%/*}"; arch="${t##*/}"; \
      case "${os}" in windows) ext=".exe" ;; *) ext="" ;; esac; \
      name="sandbox-cli_${VERSION}_${os}_${arch}${ext}"; \
      echo "==> ${name}"; \
      GOOS="${os}" GOARCH="${arch}" \
      go build -trimpath \
        -ldflags "-s -w -X github.com/Aegmis/sandbox-cli/internal/version.Version=${VERSION}" \
        -o "/out/${name}" ./cmd/sandbox-cli; \
    done; \
    cd /out && sha256sum * > SHA256SUMS && cat SHA256SUMS

# ---- runtime image ----------------------------------------------------------
# Carries the docker CLI, since sandbox-cli shells out to `docker`. Running the
# sandbox from this image also requires mounting the host docker socket, which
# hands the container control of the host daemon — prefer running the binary on
# the host and treat this image as a CI convenience.
FROM docker:cli AS runtime
COPY --from=binary /out/sandbox-cli /usr/local/bin/sandbox-cli
ENTRYPOINT ["sandbox-cli"]

# ---- export: one binary -----------------------------------------------------
#   docker build --target export --output type=local,dest=./bin .
#   -> ./bin/sandbox-cli
FROM scratch AS export
COPY --from=binary /out/sandbox-cli /sandbox-cli

# ---- dist: the whole matrix -------------------------------------------------
#   docker build --target dist --output type=local,dest=./dist .
#   -> ./dist/sandbox-cli_<version>_<os>_<arch>[.exe] + SHA256SUMS
FROM scratch AS dist
COPY --from=release /out/ /
