#!/bin/bash
set -e

# build.sh — Build the release binaries.
#
#   bash build.sh 0.3.1
#
# The version is stamped into the binary. Doing this by hand is how the panel
# ended up reporting 0.2.0 while running 0.3.0, so it lives in a script.

VERSION="${1:-dev}"
IMAGE="${IMAGE:-golang:1.27-alpine}"

cd "$(dirname "${BASH_SOURCE[0]}")"

# Git Bash on Windows rewrites paths handed to docker, and reports a POSIX
# working directory that the daemon does not understand.
export MSYS_NO_PATHCONV=1
SRC="$(pwd -W 2>/dev/null || pwd)"

echo "── Building the interface"
npm --prefix web run build

# Docker is not required to produce a binary — croft is cgo-free, so any Go
# toolchain cross-compiles it. What the image is worth is running the tests on
# Linux, which is where croft actually runs: a test can pass on the machine
# that builds and fail on the machine that serves. Two of them did.
#
# So it is used when it is there, and the fallback says what is not being
# checked rather than pretending the two paths are the same.
if ! docker info >/dev/null 2>&1; then
  echo "── No docker daemon — building with the local toolchain"
  echo "   The tests will run on $(uname -s), not on Linux."
  echo ""

  go vet ./... || exit 1
  go test ./... || exit 1
  for arch in amd64 arm64; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
      go build -ldflags="-s -w -X main.version=$VERSION" \
      -o "bin/croft-linux-$arch" ./cmd/croft || exit 1
  done

  (cd bin && sha256sum croft-linux-amd64 croft-linux-arm64 > SHA256SUMS)
  echo ""
  ls -la bin/
  echo ""
  echo "  Release with:"
  echo "    gh release create v$VERSION bin/croft-linux-amd64 bin/croft-linux-arm64 bin/SHA256SUMS"
  exit 0
fi

echo "── Building croft $VERSION"
docker run --rm \
  -e GOTOOLCHAIN=auto \
  -e VERSION="$VERSION" \
  -v "$SRC:/src" \
  -v croft-gomod:/go/pkg/mod \
  -w /src \
  "$IMAGE" sh -c '
    go vet ./... || exit 1
    go test ./... || exit 1
    for arch in amd64 arm64; do
      CGO_ENABLED=0 GOOS=linux GOARCH=$arch \
        go build -ldflags="-s -w -X main.version=$VERSION" \
        -o bin/croft-linux-$arch ./cmd/croft || exit 1
    done
  '

# Published beside the binaries so `croft update` can tell a truncated
# download from a good one.
(cd bin && sha256sum croft-linux-amd64 croft-linux-arm64 > SHA256SUMS)

echo ""
ls -la bin/
echo ""
echo "  Release with:"
echo "    gh release create v$VERSION bin/croft-linux-amd64 bin/croft-linux-arm64 bin/SHA256SUMS"
