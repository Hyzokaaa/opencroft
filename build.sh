#!/bin/bash
set -e

# build.sh — Build the release binaries.
#
#   bash build.sh 0.3.1
#
# The version is stamped into the binary. Doing this by hand is how the panel
# ended up reporting 0.2.0 while running 0.3.0, so it lives in a script.

VERSION="${1:-dev}"
IMAGE="${IMAGE:-golang:1.25-alpine}"

cd "$(dirname "${BASH_SOURCE[0]}")"

# Git Bash on Windows rewrites paths handed to docker, and reports a POSIX
# working directory that the daemon does not understand.
export MSYS_NO_PATHCONV=1
SRC="$(pwd -W 2>/dev/null || pwd)"

echo "── Building the interface"
npm --prefix web run build

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
