#!/usr/bin/env bash
# install.sh — one-command HubScope deployment for Linux servers (root or sudo user).
#
# Default behavior: download the prebuilt release binary from GitHub Releases
# (version via HUBSCOPE_VERSION, default: latest) -> verify checksum -> install
# binary -> create system user and data directory -> render systemd unit
# (embedded template below is the single source of truth; docs/deployment.md
# describes this script, it does not duplicate the unit) -> enable and start ->
# poll health endpoint -> print next-step guidance. No Go/Node toolchain
# needed — HubScope ships as a single static binary.
#
# With --build-from-source (or HUBSCOPE_BUILD_FROM_SOURCE=1): dependency check
# (go + pnpm) -> make build -> same install steps. For developers only.
#
# Idempotent: every step checks before acting; the unit file is rewritten on
# each run (content is the truth); the data directory is created but never
# cleared. Re-running the script upgrades to the requested release version.
#
# Overridable via environment:
#   HUBSCOPE_VERSION      release tag to install  (default: latest release)
#   HUBSCOPE_PREFIX       install prefix          (default /usr/local)
#   HUBSCOPE_DATA_DIR     data directory          (default /var/lib/hubscope)
#   HUBSCOPE_USER         service system user     (default hubscope)
#   HUBSCOPE_PORT         listen port             (default 8080)
#   HUBSCOPE_SYSTEMD_DIR  systemd unit directory  (default /etc/systemd/system;
#                         exists so tests never touch the real system)
#   RELEASES_BASE         releases download root  (default GitHub Releases URL;
#                         tests point this at a file:// fixture)
#   DOWNLOAD_DIR          scratch dir for downloads (default: mktemp; tests
#                         redirect it into the sandbox)
set -euo pipefail

PREFIX="${HUBSCOPE_PREFIX:-/usr/local}"
DATA_DIR="${HUBSCOPE_DATA_DIR:-/var/lib/hubscope}"
SVC_USER="${HUBSCOPE_USER:-hubscope}"
PORT="${HUBSCOPE_PORT:-8080}"
SYSTEMD_DIR="${HUBSCOPE_SYSTEMD_DIR:-/etc/systemd/system}"
VERSION="${HUBSCOPE_VERSION:-}"
RELEASES_BASE="${RELEASES_BASE:-https://github.com/taliove/hubscope/releases/download}"
GITHUB_API_LATEST="https://api.github.com/repos/taliove/hubscope/releases/latest"
SERVICE_NAME="hubscope"
HEALTH_TIMEOUT_SECONDS=30
BUILD_FROM_SOURCE=0

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log()  { printf '[hubscope-install] %s\n' "$*"; }
fail() { printf '[hubscope-install] ERROR: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<EOF
Usage: install.sh [--build-from-source]

  (default)            download the prebuilt release binary and install it as a
                       systemd service. Requires only curl and tar.
  --build-from-source  build from this checkout with 'make build' instead
                       (requires Go and pnpm). For developers.

Environment overrides: HUBSCOPE_VERSION, HUBSCOPE_PREFIX, HUBSCOPE_DATA_DIR,
HUBSCOPE_USER, HUBSCOPE_PORT — see script header for the full list.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --build-from-source) BUILD_FROM_SOURCE=1 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $arg (see --help)" ;;
  esac
done
[ "${HUBSCOPE_BUILD_FROM_SOURCE:-0}" = "1" ] && BUILD_FROM_SOURCE=1

# as_root runs a command directly when root, otherwise via sudo.
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    fail "this script must run as root, or sudo must be available"
  fi
fi
as_root() {
  if [ -n "$SUDO" ]; then
    sudo "$@"
  else
    "$@"
  fi
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required tool '$1' not found in PATH; install it first ($2)"
}

check_dependencies() {
  if [ "$BUILD_FROM_SOURCE" -eq 1 ]; then
    require_cmd go "Go toolchain: https://go.dev/dl/"
    require_cmd pnpm "pnpm: https://pnpm.io/installation"
    require_cmd make "make"
  else
    require_cmd curl "curl"
    require_cmd tar "tar"
  fi
}

# detect_asset_suffix maps the current machine onto a release asset suffix
# like linux_amd64. Release assets follow goreleaser naming:
# hubscope_<version>_<os>_<arch>.tar.gz
detect_asset_suffix() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *) fail "unsupported OS: $os (release assets cover linux/darwin only)" ;;
  esac
  case "$arch" in
    x86_64|amd64)   arch="amd64" ;;
    aarch64|arm64)  arch="arm64" ;;
    *) fail "unsupported architecture: $arch (release assets cover amd64/arm64 only)" ;;
  esac
  printf '%s_%s' "$os" "$arch"
}

resolve_version() {
  if [ -n "$VERSION" ]; then
    log "installing requested version $VERSION"
    return 0
  fi
  log "resolving latest release version..."
  VERSION="$(curl -fsSL "$GITHUB_API_LATEST" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$VERSION" ] || fail "could not resolve latest release from $GITHUB_API_LATEST; set HUBSCOPE_VERSION explicitly"
  log "latest release is $VERSION"
}

# fetch_file URL DEST — thin wrapper so tests and logs see every download.
fetch_file() {
  curl -fsSL "$1" -o "$2" || fail "download failed: $1"
}

# sha256_verify FILE EXPECTED_HASH — verifies FILE against its expected
# sha256. macOS ships shasum but not sha256sum; both print "hash  name".
sha256_verify() {
  if command -v sha256sum >/dev/null 2>&1; then
    [ "$(sha256sum "$1" | awk '{print $1}')" = "$2" ]
  elif command -v shasum >/dev/null 2>&1; then
    [ "$(shasum -a 256 "$1" | awk '{print $1}')" = "$2" ]
  else
    fail "neither sha256sum nor shasum available; cannot verify release checksum"
  fi
}

# download_binary fetches the release tarball + checksums into DOWNLOAD_DIR,
# verifies the tarball against its sha256, and extracts the binary to
# $DOWNLOAD_DIR/binary/hubscope.
download_binary() {
  local suffix asset tarball url work expected
  suffix="$(detect_asset_suffix)"
  # Release assets follow goreleaser naming: hubscope_<tag>_<os>_<arch>.tar.gz
  # (the tag's v prefix is kept — hubscope_v0.1.0_linux_amd64.tar.gz).
  asset="hubscope_${VERSION}_${suffix}.tar.gz"
  work="${DOWNLOAD_DIR:-$(mktemp -d)}"
  mkdir -p "$work"
  tarball="$work/$asset"

  log "downloading $asset"
  fetch_file "$RELEASES_BASE/$VERSION/$asset" "$tarball"
  fetch_file "$RELEASES_BASE/$VERSION/hubscope_${VERSION}_checksums.txt" "$work/checksums.txt"

  log "verifying sha256 checksum"
  expected="$(awk -v f="$asset" '$2 == f {print $1}' "$work/checksums.txt")"
  [ -n "$expected" ] || fail "checksums file has no entry for $asset — aborting, nothing installed"
  sha256_verify "$tarball" "$expected" \
    || fail "checksum verification failed for $asset — aborting, nothing installed"

  # The tarball holds a single platform-suffixed binary
  # (hubscope_v0.1.0_linux_amd64), not a file literally named "hubscope".
  mkdir -p "$work/binary"
  tar -xzf "$tarball" -C "$work/binary"
  local extracted
  extracted="$(find "$work/binary" -maxdepth 1 -type f -name 'hubscope_*' | head -1)"
  [ -n "$extracted" ] || fail "tarball did not contain a hubscope binary"
  mv "$extracted" "$work/binary/hubscope"
  DOWNLOADED_BINARY="$work/binary/hubscope"
}

build_binary() {
  log "building hubscope binary (make build)..."
  (cd "$REPO_ROOT" && make build)
  [ -f "$REPO_ROOT/bin/hubscope" ] || fail "make build did not produce bin/hubscope"
}

# obtain_binary picks the artifact install_binary() will pick up:
#   BUILD_OUTPUT override (tests) > built-from-source product > downloaded
#   release binary. Production sets neither BUILD_OUTPUT nor DOWNLOADED_BINARY
#   before obtain_binary runs.
obtain_binary() {
  if [ -n "${BUILD_OUTPUT:-}" ]; then
    BINARY_SOURCE="$BUILD_OUTPUT"
  elif [ "$BUILD_FROM_SOURCE" -eq 1 ]; then
    build_binary
    BINARY_SOURCE="$REPO_ROOT/bin/hubscope"
  else
    resolve_version
    download_binary
    BINARY_SOURCE="$DOWNLOADED_BINARY"
  fi
  [ -f "$BINARY_SOURCE" ] || fail "binary not found at $BINARY_SOURCE"
}

install_binary() {
  log "installing binary to $PREFIX/bin/hubscope"
  as_root mkdir -p "$PREFIX/bin"
  as_root install -m 0755 "$BINARY_SOURCE" "$PREFIX/bin/hubscope"
}

ensure_user() {
  if id "$SVC_USER" >/dev/null 2>&1; then
    log "system user '$SVC_USER' already exists, skipping creation"
  else
    log "creating system user '$SVC_USER'"
    # nologin lives at /usr/sbin/nologin on Debian/Ubuntu and /sbin/nologin on RHEL.
    NOLOGIN_SHELL="$(command -v nologin || echo /usr/sbin/nologin)"
    as_root useradd --system --user-group --no-create-home --shell "$NOLOGIN_SHELL" "$SVC_USER"
  fi
}

ensure_data_dir() {
  if [ -d "$DATA_DIR" ]; then
    log "data directory $DATA_DIR already exists, contents preserved"
  else
    log "creating data directory $DATA_DIR"
    as_root mkdir -p "$DATA_DIR"
  fi
  # Deliberate on every run: ownership must stay consistent after upgrades,
  # even if an administrator adjusted it manually. Cost: recursive walk on
  # large data trees.
  as_root chown -R "$SVC_USER:$SVC_USER" "$DATA_DIR"
}

# render_unit writes the systemd unit. This heredoc is the ONLY source of
# truth for the unit content; keep docs/deployment.md pointing here.
render_unit() {
  log "writing systemd unit $SYSTEMD_DIR/$SERVICE_NAME.service"
  as_root mkdir -p "$SYSTEMD_DIR"
  as_root tee "$SYSTEMD_DIR/$SERVICE_NAME.service" >/dev/null <<UNIT
[Unit]
Description=HubScope - AI Hub monitoring
After=network.target

[Service]
Type=simple
User=$SVC_USER
Group=$SVC_USER
WorkingDirectory=$DATA_DIR
Environment=ADDR=:$PORT
Environment=DATA_PATH=$DATA_DIR/app.db
ExecStart=$PREFIX/bin/hubscope
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
UNIT
}

restart_service() {
  log "enabling and (re)starting $SERVICE_NAME"
  as_root systemctl daemon-reload
  as_root systemctl enable --now "$SERVICE_NAME"
  # Explicit restart so a re-run picks up the freshly installed binary.
  as_root systemctl restart "$SERVICE_NAME"
}

wait_healthy() {
  local url="http://localhost:$PORT/api/overview"
  log "waiting for $url (up to ${HEALTH_TIMEOUT_SECONDS}s)..."
  local deadline=$(( $(date +%s) + HEALTH_TIMEOUT_SECONDS ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if curl -sf -o /dev/null "$url"; then
      log "health check passed"
      return 0
    fi
    sleep 1
  done
  # A slow first boot (initial migration) should not abort the install; the
  # service is already enabled and will keep retrying via Restart=on-failure.
  log "WARNING: health check did not pass within ${HEALTH_TIMEOUT_SECONDS}s; inspect with: journalctl -u $SERVICE_NAME -n 50"
  return 0
}

print_guidance() {
  cat <<EOF

HubScope installed and running.

Next steps:
  # The CLI resolves the database via DATA_PATH (default ./data/app.db),
  # so the admin command must run with the service's DATA_PATH — otherwise
  # it silently writes to a fresh db in the caller's cwd and logins fail
  # against the real one.
  1. Create the first super admin (replace with your own strong password):
       sudo DATA_PATH=$DATA_DIR/app.db $PREFIX/bin/hubscope admin create --username admin --password 'REPLACE-WITH-STRONG-PASSWORD'
  2. Open the dashboard:
       http://localhost:$PORT
  3. Manage the service:
       systemctl status $SERVICE_NAME
       journalctl -u $SERVICE_NAME -f

Data directory: $DATA_DIR (SQLite at $DATA_DIR/app.db)
EOF
}

main() {
  check_dependencies
  obtain_binary
  install_binary
  ensure_user
  ensure_data_dir
  render_unit
  restart_service
  wait_healthy
  print_guidance
}

main
