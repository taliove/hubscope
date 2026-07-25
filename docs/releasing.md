# Releasing HubScope

This document is the single source of truth for how changes land on `main`
and how releases are cut. The `release-check` skill remains the pre-release
*checklist*; `scripts/release.sh` is the *mechanism*.

## Landing changes: the PR workflow

All changes reach `main` through pull requests — no direct pushes, no local
fast-forward merges. The pre-push hook and CI are safety nets, not the
process.

1. **Branch** from an up-to-date `main`: `feat/<slug>` or `fix/<slug>`.
2. **Develop** per the constitution (AGENTS.md): impact analysis, TDD at the
   W1 seam, independent code review before committing.
3. **Open the PR** — the template (.github/pull_request_template.md) asks for
   a summary, a change list, and a test plan. Fill all three.
4. **Label exactly once**: `enhancement` | `bug` | `documentation` | `chore` |
   `ci`. The label drives the release-notes category
   (.github/release.yml); missing or multiple labels put the PR in
   "Other Changes".
5. **CI green**, then **squash merge** (`gh pr merge --squash --delete-branch`).
   Squash keeps `main` a flat list of one commit per PR, which is what makes
   the auto-generated release notes readable.
6. The remote branch is deleted on merge; delete the local one too.

## Cutting a release

Prerequisites: on `main`, clean tree, everything intended for the release
already merged. Then exactly one command:

```sh
scripts/release.sh vX.Y.Z
```

The script:

1. Verifies the preconditions (on `main`, clean, in sync with origin, tag
   unused) and runs `make test`.
2. Bumps the README version references (download URL, `HUBSCOPE_VERSION`
   example) in both `README.md` and `README.zh-CN.md`.
3. Opens an editor for the **release highlights** — 1-3 user-facing
   sentences about what the release is and why it is worth upgrading. They
   travel to the workflow as the tag annotation and land in the release
   notes. A release without highlights is refused.
4. Commits the bump (`chore(release): vX.Y.Z`), creates the annotated tag,
   pushes `main` and the tag.

The tag push triggers `.github/workflows/release.yml`, which:

1. Builds the frontend, cross-compiles `linux/darwin × amd64/arm64` binaries,
   generates `hubscope_<tag>_checksums.txt`, smoke-tests the linux/amd64
   binary.
2. Creates the GitHub Release itself with a notes body assembled from:
   a fixed **Install / Upgrade** section (one-command install, Docker,
   binaries + checksum verification, in-place upgrade note), the
   **Highlights** from the tag annotation, and the auto-generated
   **What's Changed** list categorized by label (Features / Fixes /
   Documentation / Maintenance / Other Changes). The generated
   "New Contributors" section is stripped — noise in a solo-maintained repo.
   No manual `gh release create` step; re-running the workflow on an
   existing release only refreshes assets.

After the workflow finishes, verify:

```sh
gh release view vX.Y.Z   # notes + 5 assets (4 tarballs + checksums)
```

## Versioning

Semver, loosely: **minor** for user-visible features and behavior changes,
**patch** for fixes, docs, and process/tooling changes. Pre-1.0, minor bumps
may include anything.

## Fixing a botched release

If a tag was pushed by mistake:

```sh
git push origin :refs/tags/vX.Y.Z        # deletes the remote tag (stops nothing if the workflow already ran)
gh release delete vX.Y.Z --yes           # if the release was already created
git tag -d vX.Y.Z                        # local tag
```

Then fix forward and re-run `scripts/release.sh` with the same or a new
version. Never force-push `main` (the pre-push hook blocks it anyway).
