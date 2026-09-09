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

echo "── Building the interface"
npm --prefix web run build

echo "── Building croft $VERSION"
docker run --rm \
  -e GOTOOLCHAIN=auto \
  -e VERSION="$VERSION" \
  -v "$PWD:/src" \
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

echo ""
ls -la bin/
echo ""
echo "  Release with:"
echo "    gh release create v$VERSION bin/croft-linux-amd64 bin/croft-linux-arm64"
