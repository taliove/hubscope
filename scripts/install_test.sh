#!/usr/bin/env bash
# install_test.sh — black-box behavior tests for scripts/install.sh.
#
# Tests external observable behavior only (files on disk, stub call logs, exit
# codes, output), never script internals. Every system-touching step is
# neutralized: HUBSCOPE_PREFIX / HUBSCOPE_DATA_DIR / HUBSCOPE_SYSTEMD_DIR are
# redirected into a temp directory, and `make`, `systemctl`, `useradd`,
# `install`, and `curl` are replaced by stubs placed first in PATH. The release
# download is served from a file:// fixture (RELEASES_BASE) — no network, no
# GitHub dependency. Runs on macOS and Linux without root.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO_ROOT/scripts/install.sh"
FIXTURE_VERSION="v9.9.9"

PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf 'PASS: %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL: %s\n' "$1" >&2; }

# expect_exit CODE "description" CMD [ARG...] — runs CMD, asserts exit code.
expect_exit() {
  local want="$1" desc="$2"
  shift 2
  local got=0
  set +e
  "$@" >/dev/null 2>&1
  got=$?
  set -e
  if [ "$got" -eq "$want" ]; then
    pass "$desc"
  else
    fail "$desc (want exit $want, got $got)"
  fi
}

# expect_file_contains FILE PATTERN "description"
expect_file_contains() {
  local file="$1" pattern="$2" desc="$3"
  if [ -f "$file" ] && grep -q "$pattern" "$file"; then
    pass "$desc"
  else
    fail "$desc (file: $file, pattern: $pattern)"
  fi
}

# --- fake tool builders ------------------------------------------------------

# write_fake_tool NAME SCRIPT — drops an executable stub into the sandbox bin.
write_fake_tool() {
  printf '#!/usr/bin/env bash\n%s\n' "$2" > "$FAKE_BIN/$1"
  chmod +x "$FAKE_BIN/$1"
}

# hide_tool NAME — makes a tool unresolvable inside the sandbox even when a
# real one exists under /usr/bin (CI runners ship go there). A broken
# absolute symlink in the stub dir: `command -v` requires an executable
# *target*, so the lookup falls through — but bash caches the stub-dir name
# and never reaches the real tool later in PATH.
hide_tool() {
  ln -s "/nonexistent/install-test-shadows-$1" "$FAKE_BIN/$1"
}

# expect_file PATH "description" — asserts PATH exists.
expect_file() {
  if [ -f "$1" ]; then
    pass "$2"
  else
    fail "$2"
  fi
}

# expect_dir PATH "description" — asserts PATH is a directory.
expect_dir() {
  if [ -d "$1" ]; then
    pass "$2"
  else
    fail "$2"
  fi
}

# expect_missing PATH "description" — asserts PATH does not exist.
expect_missing() {
  if [ ! -e "$1" ]; then
    pass "$2"
  else
    fail "$2"
  fi
}

# expect_nonzero STATUS "description" — asserts STATUS is not 0.
expect_nonzero() {
  if [ "$1" -ne 0 ]; then
    pass "$2"
  else
    fail "$2"
  fi
}

# --- release fixture ---------------------------------------------------------

# asset_suffix_for_test mirrors the installer's uname mapping so the fixture
# asset name matches what the installer will request on this machine.
asset_suffix_for_test() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
  esac
  case "$arch" in
    x86_64|amd64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
  esac
  printf '%s_%s' "$os" "$arch"
}

# sha256_of FILE — portable checksum helper (macOS lacks sha256sum).
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# make_release_fixture ROOT — creates a fake release under ROOT/v9.9.9/:
# the tarball (containing a stub `hubscope` script) and a checksums file,
# laid out exactly like GitHub Releases download URLs.
make_release_fixture() {
  local root="$1"
  local suffix asset staged hash
  suffix="$(asset_suffix_for_test)"
  asset="hubscope_${FIXTURE_VERSION}_${suffix}.tar.gz"
  mkdir -p "$root/$FIXTURE_VERSION"
  staged="$(mktemp -d)"
  printf '#!/usr/bin/env bash\necho fake hubscope binary\n' > "$staged/hubscope"
  chmod +x "$staged/hubscope"
  tar -czf "$root/$FIXTURE_VERSION/$asset" -C "$staged" hubscope
  rm -rf "$staged"
  hash="$(sha256_of "$root/$FIXTURE_VERSION/$asset")"
  printf '%s  %s\n' "$hash" "$asset" > "$root/$FIXTURE_VERSION/hubscope_${FIXTURE_VERSION}_checksums.txt"
  FIXTURE_ASSET="$asset"
}

# corrupt_release_fixture ROOT — replaces the tarball with garbage so the
# recorded checksum no longer matches.
corrupt_release_fixture() {
  local root="$1"
  printf 'tampered payload' > "$root/$FIXTURE_VERSION/$FIXTURE_ASSET"
}

# build_sandbox STUB_STYLE — creates a fresh temp sandbox with:
#   FAKE_BIN/   stub tools + release fixture
#   PREFIX/ DATA_DIR/ SYSTEMD_DIR/   redirected install targets
# STUB_STYLE=all      everything needed for a successful binary-download run
# STUB_STYLE=no-go    `go` is NOT in PATH (source-build dependency failure case)
# STUB_STYLE=no-pnpm  `pnpm` is NOT in PATH (source-build dependency failure case)
# STUB_STYLE=no-toolchain  neither `go` nor `pnpm` (binary install needs neither)
build_sandbox() {
  local style="$1"
  SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/hubscope-install-test.XXXXXX")"
  FAKE_BIN="$SANDBOX/bin"
  PREFIX_DIR="$SANDBOX/prefix"
  DATA_DIR="$SANDBOX/data"
  SYSTEMD_DIR="$SANDBOX/systemd"
  FIXTURE_DIR="$SANDBOX/releases"
  mkdir -p "$FAKE_BIN" "$PREFIX_DIR" "$DATA_DIR" "$SYSTEMD_DIR" "$FIXTURE_DIR"
  make_release_fixture "$FIXTURE_DIR"

  # The source-build path builds via `make build`; the stub copies a canned
  # binary to a path INSIDE the sandbox and run_install points BUILD_OUTPUT at
  # it, so the repo's real bin/hubscope is never touched. (The previous stub
  # copied the fake into $REPO_ROOT/bin/hubscope, silently swapping out the
  # developer's real binary on every `make test` run.)
  # shellcheck disable=SC2016
  write_fake_tool make 'if [ "${1:-}" = "build" ]; then cp "'"$FAKE_BIN"'/fake-binary" "'"$SANDBOX"'/built-binary"; fi'

  # Record every privileged call for assertions.
  write_fake_tool systemctl 'echo "systemctl $*" >> "'"$SANDBOX"'/systemctl.log"'
  write_fake_tool useradd   'echo "useradd $*" >> "'"$SANDBOX"'/useradd.log"'
  # shellcheck disable=SC2016
  write_fake_tool install   'echo "install $*" >> "'"$SANDBOX"'/install.log"; cp "${3:?}" "${4:?}"'
  # chown targets a synthetic service user that never exists on the test host.
  write_fake_tool chown     'echo "chown $*" >> "'"$SANDBOX"'/chown.log"'
  # curl serves fixture files for file:// URLs (release download) and succeeds
  # silently otherwise (health check).
  # shellcheck disable=SC2016
  write_fake_tool curl '
url=""
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
case "$url" in
  file://*)
    src="${url#file://}"
    if [ -n "$out" ]; then cp "$src" "$out"; else cat "$src"; fi
    ;;
  *) exit 0 ;;
esac
'
  # Neutralize any sudo call in case tests run as non-root on a sudo machine.
  write_fake_tool sudo      'exec "$@"'

  printf '#!/usr/bin/env bash\necho fake hubscope binary\n' > "$FAKE_BIN/fake-binary"

  # Deliberately not `local`: run_install (below) reads this after build_sandbox returns.
  # Base dirs stay for real coreutils (mkdir/cp/date/sleep) and the stub
  # shebangs' /usr/bin/env; stub dir first so fakes shadow. go/pnpm are
  # handled per-scenario below (fake or hidden via broken symlink).
  SANDBOX_PATH="$FAKE_BIN:/usr/bin:/bin"

  # env shim: with -i, exports the assignments then re-execs the command
  # through bash. This matters on Linux: coreutils env resolves the command
  # itself and SKIPS broken symlinks, which would find a real go the
  # missing-dependency scenarios hide. bash's resolution honors the shadows.
  # `env bash …` (stub shebangs) also routes here so nested stubs keep the
  # sandbox PATH instead of env's default. Everything else goes to real env.
  # shellcheck disable=SC2016
  write_fake_tool env '
if [ "${1:-}" = "-i" ]; then
  shift
  while [ $# -gt 0 ] && [ "${1%%=*}" != "$1" ]; do
    export "$1"; shift
  done
  exec /usr/bin/env bash -c "exec \"\$@\"" dummy "$@"
fi
if [ "${1:-}" = "bash" ]; then
  shift
  exec /usr/bin/env bash -c "exec \"\$@\"" dummy bash "$@"
fi
exec /usr/bin/env "$@"
'
  if [ "$style" = "all" ] || [ "$style" = "no-pnpm" ]; then
    write_fake_tool go 'echo "go $*" > /dev/null'
  else
    # A fake make without the go requirement would let the run succeed even
    # without go; shadow any real go that the base PATH might provide.
    hide_tool go
  fi
  if [ "$style" = "all" ] || [ "$style" = "no-go" ]; then
    write_fake_tool pnpm 'echo "pnpm $*" > /dev/null'
  else
    hide_tool pnpm
  fi
  # run_install runs the installer against the fixture release; extra
  # arguments (e.g. --build-from-source) are forwarded.
  run_install() {
    # The stub env (sandbox dir) handles the -i form itself: it exports the
    # assignments and re-execs through bash, so the command is resolved by
    # bash — which honors the broken-symlink shadows — instead of env, which
    # on Linux would skip them and find the real toolchain.
    PATH="$SANDBOX_PATH" env -i PATH="$SANDBOX_PATH" \
      HUBSCOPE_PREFIX="$PREFIX_DIR" \
      HUBSCOPE_DATA_DIR="$DATA_DIR" \
      HUBSCOPE_SYSTEMD_DIR="$SYSTEMD_DIR" \
      HUBSCOPE_VERSION="$FIXTURE_VERSION" \
      RELEASES_BASE="file://$FIXTURE_DIR" \
      DOWNLOAD_DIR="$SANDBOX/download" \
      bash "$INSTALLER" "$@"
  }
}

cleanup_sandbox() {
  if [ -n "${SANDBOX:-}" ]; then
    rm -rf "$SANDBOX"
    SANDBOX=""
  fi
}
trap cleanup_sandbox EXIT

# --- test cases ---------------------------------------------------------------

# run_install_with_env runs the installer in the current sandbox; it mirrors
# the sandbox-local run_install so it is legal as an expect_exit command.
run_install_with_env() {
  run_install
}

echo "== binary install: files land, service enabled, guidance printed =="
build_sandbox all
OUT="$(run_install)"
expect_exit 0 "binary install exits 0" run_install_with_env

expect_file "$PREFIX_DIR/bin/hubscope" "binary installed to PREFIX/bin"
expect_dir "$DATA_DIR" "data directory created"
expect_file "$SYSTEMD_DIR/hubscope.service" "systemd unit written"

UNIT="$SYSTEMD_DIR/hubscope.service"
expect_file_contains "$UNIT" "Type=simple" "unit has Type=simple"
expect_file_contains "$UNIT" "User=hubscope" "unit has User=hubscope"
expect_file_contains "$UNIT" "Group=hubscope" "unit has Group=hubscope"
expect_file_contains "$UNIT" "Restart=on-failure" "unit has Restart=on-failure"
expect_file_contains "$UNIT" "NoNewPrivileges=true" "unit is hardened (NoNewPrivileges)"
expect_file_contains "$UNIT" "ProtectSystem=strict" "unit is hardened (ProtectSystem)"
expect_file_contains "$UNIT" "Environment=ADDR=:8080" "unit sets ADDR from HUBSCOPE_PORT"
expect_file_contains "$UNIT" "Environment=DATA_PATH=$DATA_DIR/app.db" "unit sets DATA_PATH inside data dir"
expect_file_contains "$UNIT" "WorkingDirectory=$DATA_DIR" "unit sets WorkingDirectory to data dir"

expect_file_contains "$SANDBOX/systemctl.log" "daemon-reload" "systemctl daemon-reload invoked"
expect_file_contains "$SANDBOX/systemctl.log" "enable --now hubscope" "systemctl enable --now invoked"
expect_file_contains "$SANDBOX/useradd.log" "useradd" "system user creation attempted"

case "$OUT" in
  *"$FIXTURE_VERSION"*) pass "output names the installed version" ;;
  *) fail "output names the installed version" ;;
esac
case "$OUT" in
  *"admin create"*) pass "guidance prints admin create hint" ;;
  *) fail "guidance prints admin create hint" ;;
esac
case "$OUT" in
  *"http://localhost:8080"*) pass "guidance prints access URL" ;;
  *) fail "guidance prints access URL" ;;
esac
cleanup_sandbox

echo "== binary install needs no go/pnpm =="
build_sandbox no-toolchain
expect_exit 0 "binary install without go/pnpm exits 0" run_install_with_env
expect_file "$PREFIX_DIR/bin/hubscope" "binary installed without any toolchain"
cleanup_sandbox

echo "== checksum mismatch aborts before anything is installed =="
build_sandbox all
corrupt_release_fixture "$FIXTURE_DIR"
set +e
ERR_OUT="$(run_install 2>&1)"
STATUS=$?
set -e
expect_nonzero "$STATUS" "checksum mismatch: non-zero exit"
case "$ERR_OUT" in
  *"checksum"*) pass "checksum mismatch: error mentions checksum" ;;
  *) fail "checksum mismatch: error mentions checksum (got: $ERR_OUT)" ;;
esac
expect_missing "$PREFIX_DIR/bin/hubscope" "checksum mismatch: nothing half-installed"
cleanup_sandbox

echo "== source build: --build-from-source requires the toolchain =="
build_sandbox no-go
set +e
ERR_OUT="$(run_install --build-from-source 2>&1)"
STATUS=$?
set -e
expect_nonzero "$STATUS" "source build, missing go: non-zero exit"
case "$ERR_OUT" in
  *"go"*"not found"*) pass "source build, missing go: error names the tool" ;;
  *) fail "source build, missing go: error names the tool (got: $ERR_OUT)" ;;
esac
expect_missing "$PREFIX_DIR/bin/hubscope" "source build, missing go: nothing half-installed"
cleanup_sandbox

build_sandbox no-pnpm
set +e
ERR_OUT="$(run_install --build-from-source 2>&1)"
STATUS=$?
set -e
expect_nonzero "$STATUS" "source build, missing pnpm: non-zero exit"
case "$ERR_OUT" in
  *"pnpm"*"not found"*) pass "source build, missing pnpm: error names the tool" ;;
  *) fail "source build, missing pnpm: error names the tool (got: $ERR_OUT)" ;;
esac
expect_missing "$PREFIX_DIR/bin/hubscope" "source build, missing pnpm: nothing half-installed"
cleanup_sandbox

echo "== source build: happy path builds and installs =="
build_sandbox all
expect_exit 0 "source build exits 0" run_install_with_env --build-from-source
expect_file "$PREFIX_DIR/bin/hubscope" "source-built binary installed to PREFIX/bin"
cleanup_sandbox

echo "== re-run is idempotent: data preserved, unit rewritten, exit 0 =="
build_sandbox all
run_install >/dev/null
SENTINEL="$DATA_DIR/sentinel.db"
printf 'precious data' > "$SENTINEL"
mkdir -p "$DATA_DIR/subdir"
printf 'more data' > "$DATA_DIR/subdir/other"

expect_exit 0 "re-run exits 0" run_install_with_env
if [ -f "$SENTINEL" ] && [ "$(cat "$SENTINEL")" = "precious data" ]; then
  pass "re-run preserves data directory contents"
else
  fail "re-run preserves data directory contents"
fi
expect_file "$DATA_DIR/subdir/other" "re-run preserves nested files"
expect_file "$SYSTEMD_DIR/hubscope.service" "re-run rewrites unit (upgrade semantics)"
cleanup_sandbox

echo "== env overrides: custom prefix / data dir / port honored =="
build_sandbox all
expect_exit 0 "install with custom port exits 0" \
  env PATH="$SANDBOX_PATH" \
    HUBSCOPE_PREFIX="$PREFIX_DIR" HUBSCOPE_DATA_DIR="$DATA_DIR" \
    HUBSCOPE_SYSTEMD_DIR="$SYSTEMD_DIR" HUBSCOPE_PORT=9090 \
    HUBSCOPE_VERSION="$FIXTURE_VERSION" \
    RELEASES_BASE="file://$FIXTURE_DIR" \
    DOWNLOAD_DIR="$SANDBOX/download" \
    bash "$INSTALLER"
expect_file_contains "$SYSTEMD_DIR/hubscope.service" "Environment=ADDR=:9090" "unit uses overridden port"
cleanup_sandbox

echo
echo "install_test.sh: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
