#!/usr/bin/env bash
# release.sh — the ONLY supported way to cut a HubScope release.
#
# What it does, in order:
#   1. Verify we are on a clean main that is in sync with origin, and that the
#      requested tag does not exist yet (locally or on origin).
#   2. Run the full test gate (make test — the same gate as the git hooks).
#   3. Bump the README version references (download URL + HUBSCOPE_VERSION
#      example) from the current version to the requested one.
#   4. Collect the Highlights for the release notes (an editor opens with a
#      template; 1-3 user-facing sentences — what this release is and why it
#      is worth upgrading).
#   5. Commit the bump, create the annotated tag, push main and the tag.
#      The collected highlights travel to the release workflow as the tag
#      message (the annotation after the version line).
#
# The tag push triggers .github/workflows/release.yml, which cross-compiles
# the binaries and creates the GitHub Release: fixed install/upgrade
# instructions + the highlights + auto-generated, label-categorized notes
# (see .github/release.yml) — no manual release creation step.
#
# Usage: scripts/release.sh vX.Y.Z
#
# Process contract: docs/releasing.md. Per the repo constitution, invoking
# this script at all requires an explicit user instruction.
set -euo pipefail

log()  { printf '[hubscope-release] %s\n' "$*"; }
fail() { printf '[hubscope-release] ERROR: %s\n' "$*" >&2; exit 1; }

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

VERSION="${1:-}"
case "$VERSION" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) fail "usage: scripts/release.sh vX.Y.Z (e.g. v0.3.0)" ;;
esac

command -v gh >/dev/null 2>&1 || fail "gh CLI is required (https://cli.github.com)"

# --- 1. Preconditions ---------------------------------------------------------

[ "$(git branch --show-current)" = "main" ] \
  || fail "must be on main — releases are cut from main only"
[ -z "$(git status --porcelain)" ] \
  || fail "working tree is not clean; commit or stash first"

git fetch origin main --quiet
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] \
  || fail "main is not in sync with origin/main — push or pull first"

if git tag -l "$VERSION" | grep -q .; then
  fail "tag $VERSION already exists locally"
fi
if git ls-remote --tags origin "$VERSION" | grep -q .; then
  fail "tag $VERSION already exists on origin"
fi

# --- 2. Gate ------------------------------------------------------------------

log "running make test..."
make test >/dev/null || fail "make test failed; fix before releasing"

# --- 3. Bump README version references ---------------------------------------

CURRENT="$(grep -oE 'hubscope_v[0-9]+\.[0-9]+\.[0-9]+_linux_amd64' README.md | head -1 \
  | sed -E 's/hubscope_(v[0-9]+\.[0-9]+\.[0-9]+)_linux_amd64/\1/')"
[ -n "$CURRENT" ] || fail "could not find the current version reference in README.md"
[ "$CURRENT" != "$VERSION" ] || fail "README already references $VERSION — nothing to bump"

log "bumping README version references: $CURRENT -> $VERSION"
# sed -i with a backup suffix is the portable form across GNU and BSD sed.
sed -i.bak "s/$CURRENT/$VERSION/g" README.md README.zh-CN.md
rm -f README.md.bak README.zh-CN.md.bak

git add README.md README.zh-CN.md
git diff --cached --stat

# --- 4. Collect release highlights ---------------------------------------------

# The highlights become the tag annotation (after the version line) and the
# release workflow lifts them into the GitHub Release notes. An editor opens
# with a template; lines starting with # are stripped.
HIGHLIGHTS_FILE="$(mktemp "${TMPDIR:-/tmp}/hubscope-release-notes.XXXXXX")"
trap 'rm -f "$HIGHLIGHTS_FILE"' EXIT
cat > "$HIGHLIGHTS_FILE" <<'EOF'

# Highlights for this release: 1-3 user-facing sentences — what this release
# is and why it is worth upgrading. Write for someone deciding whether to
# upgrade, not for a reviewer. Lines starting with # are ignored.
EOF
"${EDITOR:-vi}" "$HIGHLIGHTS_FILE"
HIGHLIGHTS="$(grep -v '^#' "$HIGHLIGHTS_FILE" | sed -e 's/^[[:space:]]*//' -e '/^$/d')"
[ -n "$HIGHLIGHTS" ] || fail "no highlights provided — a release without a user-facing summary is not publishable"

# --- 5. Commit, tag, push -------------------------------------------------------

log "committing version bump"
git commit -m "chore(release): $VERSION" --quiet

log "tagging $VERSION"
git tag -a "$VERSION" -m "$VERSION" -m "$HIGHLIGHTS"

log "pushing main and $VERSION (pre-push gate re-runs make test)"
git push origin main --quiet
git push origin "$VERSION" --quiet

cat <<EOF

Release $VERSION is on its way.

  - Tag pushed; the release workflow now builds the binaries and creates the
    GitHub Release with auto-generated notes:
      https://github.com/taliove/hubscope/actions/workflows/release.yml
  - When it finishes, verify the assets and notes:
      gh release view $VERSION
EOF
