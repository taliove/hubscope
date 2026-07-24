#!/usr/bin/env bash
# install_test.sh — black-box behavior tests for scripts/install.sh.
#
# Tests external observable behavior only (files on disk, stub call logs, exit
# codes, output), never script internals. Every system-touching step is
# neutralized: HUBSCOPE_PREFIX / HUBSCOPE_DATA_DIR / HUBSCOPE_SYSTEMD_DIR are
# redirected into a temp directory, and `make`, `systemctl`, `useradd`,
# `install`, and `curl` are replaced by stubs placed first in PATH. Runs on
# macOS and Linux without root.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO_ROOT/scripts/install.sh"

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

# build_sandbox STUB_STYLE — creates a fresh temp sandbox with:
#   FAKE_BIN/   stub tools + a fake build product
#   PREFIX/ DATA_DIR/ SYSTEMD_DIR/   redirected install targets
# STUB_STYLE=all      everything needed for a successful run
# STUB_STYLE=no-go    `go` is NOT in PATH (dependency failure case)
# STUB_STYLE=no-pnpm  `pnpm` is NOT in PATH
build_sandbox() {
  local style="$1"
  SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/hubscope-install-test.XXXXXX")"
  FAKE_BIN="$SANDBOX/bin"
  PREFIX_DIR="$SANDBOX/prefix"
  DATA_DIR="$SANDBOX/data"
  SYSTEMD_DIR="$SANDBOX/systemd"
  mkdir -p "$FAKE_BIN" "$PREFIX_DIR" "$DATA_DIR" "$SYSTEMD_DIR"

  # The installer builds via `make build`; the stub copies a canned binary.
  # The parameter expansion only expands inside the stub at run time.
  # shellcheck disable=SC2016
  write_fake_tool make 'if [ "${1:-}" = "build" ]; then cp "'"$FAKE_BIN"'/fake-binary" "'"$REPO_ROOT"'/bin/hubscope"; fi'

  # Record every privileged call for assertions.
  write_fake_tool systemctl 'echo "systemctl $*" >> "'"$SANDBOX"'/systemctl.log"'
  write_fake_tool useradd   'echo "useradd $*" >> "'"$SANDBOX"'/useradd.log"'
  # shellcheck disable=SC2016
  write_fake_tool install   'echo "install $*" >> "'"$SANDBOX"'/install.log"; cp "${3:?}" "${4:?}"'
  # chown targets a synthetic service user that never exists on the test host.
  write_fake_tool chown     'echo "chown $*" >> "'"$SANDBOX"'/chown.log"'
  # Health check succeeds instantly.
  write_fake_tool curl      'exit 0'
  # Neutralize any sudo call in case tests run as non-root on a sudo machine.
  write_fake_tool sudo      'exec "$@"'

  printf '#!/usr/bin/env bash\necho fake hubscope binary\n' > "$FAKE_BIN/fake-binary"

  # Deliberately not `local`: run_install (below) reads this after build_sandbox returns.
  SANDBOX_PATH="$FAKE_BIN:/usr/bin:/bin:/usr/local/bin:/usr/sbin:/sbin"
  if [ "$style" = "all" ] || [ "$style" = "no-pnpm" ]; then
    write_fake_tool go 'echo "go $*" > /dev/null'
  fi
  if [ "$style" = "all" ] || [ "$style" = "no-go" ]; then
    write_fake_tool pnpm 'echo "pnpm $*" > /dev/null'
  fi

  # Arguments are forwarded to the installer for future flag coverage.
  # Callers below intentionally pass no arguments today.
  # shellcheck disable=SC2120
  run_install() {
    env -i PATH="$SANDBOX_PATH" \
      HUBSCOPE_PREFIX="$PREFIX_DIR" \
      HUBSCOPE_DATA_DIR="$DATA_DIR" \
      HUBSCOPE_SYSTEMD_DIR="$SYSTEMD_DIR" \
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

echo "== first install: files land, service enabled, guidance printed =="
build_sandbox all
OUT="$(run_install)"
expect_exit 0 "first install exits 0" run_install_with_env

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
  *"admin create"*) pass "guidance prints admin create hint" ;;
  *) fail "guidance prints admin create hint" ;;
esac
case "$OUT" in
  *"http://localhost:8080"*) pass "guidance prints access URL" ;;
  *) fail "guidance prints access URL" ;;
esac
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
  env -i PATH="$FAKE_BIN:/usr/bin:/bin:/usr/local/bin:/usr/sbin:/sbin" \
    HUBSCOPE_PREFIX="$PREFIX_DIR" HUBSCOPE_DATA_DIR="$DATA_DIR" \
    HUBSCOPE_SYSTEMD_DIR="$SYSTEMD_DIR" HUBSCOPE_PORT=9090 \
    bash "$INSTALLER"
expect_file_contains "$SYSTEMD_DIR/hubscope.service" "Environment=ADDR=:9090" "unit uses overridden port"
cleanup_sandbox

echo "== missing dependencies fail fast with a clear message =="
build_sandbox no-go
set +e
ERR_OUT="$(run_install 2>&1)"
STATUS=$?
set -e
expect_nonzero "$STATUS" "missing go: non-zero exit"
case "$ERR_OUT" in
  *"go"*"not found"*) pass "missing go: error names the tool" ;;
  *) fail "missing go: error names the tool (got: $ERR_OUT)" ;;
esac
expect_missing "$PREFIX_DIR/bin/hubscope" "missing go: nothing half-installed"
cleanup_sandbox

build_sandbox no-pnpm
set +e
ERR_OUT="$(run_install 2>&1)"
STATUS=$?
set -e
expect_nonzero "$STATUS" "missing pnpm: non-zero exit"
case "$ERR_OUT" in
  *"pnpm"*"not found"*) pass "missing pnpm: error names the tool" ;;
  *) fail "missing pnpm: error names the tool (got: $ERR_OUT)" ;;
esac
expect_missing "$PREFIX_DIR/bin/hubscope" "missing pnpm: nothing half-installed"
cleanup_sandbox

echo
echo "install_test.sh: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
