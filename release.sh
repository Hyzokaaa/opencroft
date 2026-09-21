#!/bin/bash
set -e

# release.sh — Everything between a green tree and a release people can install.
#
#   bash release.sh 0.20.0
#   bash release.sh 0.20.0 --dry-run     print what it would do and stop
#
# build.sh makes the binaries. This decides the version, checks the tree is in a
# state worth releasing, tags it, pushes and publishes. It exists because the
# five steps in between are the ones easy to do in the wrong order — tagging
# before the tests pass, or publishing a binary built from a dirty tree.

cd "$(dirname "${BASH_SOURCE[0]}")"

VERSION="${1:-}"
DRY_RUN=false
[ "${2:-}" = "--dry-run" ] && DRY_RUN=true

if [ -z "$VERSION" ]; then
  LAST=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
  echo "[ERROR] Which version? The last one is $LAST." >&2
  echo "" >&2
  echo "  feat: since then → bump the minor. fix: or perf: → bump the patch." >&2
  echo "" >&2
  git log --format="  %s" "$LAST..HEAD" >&2
  exit 1
fi

VERSION="${VERSION#v}"
TAG="v$VERSION"

# ── What must be true before anything is published ────────────────────────────

if [ -n "$(git status --porcelain)" ]; then
  echo "[ERROR] The tree is dirty. A release has to be a commit somebody can check out." >&2
  git status --short >&2
  exit 1
fi

if git rev-parse "$TAG" >/dev/null 2>&1; then
  echo "[ERROR] $TAG already exists. Tags that are already published are never moved." >&2
  exit 1
fi

BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "main" ]; then
  echo "[ERROR] On $BRANCH, not main." >&2
  exit 1
fi

LAST=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
NOTES=$(git log --format="- %s" "${LAST:+$LAST..}HEAD")

echo ""
echo "── Releasing $TAG"
echo ""
echo "$NOTES" | sed 's/^/  /'
echo ""

if [ "$DRY_RUN" = true ]; then
  echo "  Dry run — nothing was built, tagged or pushed."
  echo ""
  exit 0
fi

# ── Build, then tag ───────────────────────────────────────────────────────────
#
# In that order on purpose. build.sh runs vet and the tests inside the image, so
# a tag is only ever created for a tree that passed them.

bash build.sh "$VERSION"

# Annotated, not lightweight: it carries its own date, author and message, which
# is what `git describe` and every mirror expect to find.
git tag -a "$TAG" -m "$TAG

$NOTES"

git push origin main
git push origin "$TAG"

# ── Publish ───────────────────────────────────────────────────────────────────

if command -v gh >/dev/null; then
  gh release create "$TAG" \
    --title "$TAG" \
    --notes "$NOTES" \
    bin/croft-linux-amd64 bin/croft-linux-arm64 bin/SHA256SUMS
  echo ""
  echo "  Published. On a server:  sudo croft update"
else
  echo ""
  echo "  gh is not installed, so the tag is pushed but nothing is published yet."
  echo "  Until a release exists, \`croft update\` has nothing to find. Finish with:"
  echo ""
  echo "    gh release create $TAG --title $TAG \\"
  echo "      bin/croft-linux-amd64 bin/croft-linux-arm64 bin/SHA256SUMS"
  echo ""
  echo "  Or upload those three files to the release page by hand."
fi
