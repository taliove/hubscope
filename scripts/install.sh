#!/usr/bin/env bash
# install.sh — one-command HubScope deployment for Linux servers (root or sudo user).
#
# Behavior: dependency check -> make build -> install binary -> create system
# user and data directory -> render systemd unit (embedded template below is the
# single source of truth; docs/deployment.md describes this script, it does not
# duplicate the unit) -> enable and start -> poll health endpoint -> print
# next-step guidance.
#
# Idempotent: every step checks before acting; the unit file is rewritten on
# each run (content is the truth); the data directory is created but never
# cleared. Re-running the script upgrades to the current source version.
#
# Overridable via environment:
#   HUBSCOPE_PREFIX      install prefix           (default /usr/local)
#   HUBSCOPE_DATA_DIR    data directory           (default /var/lib/hubscope)
#   HUBSCOPE_USER        service system user      (default hubscope)
#   HUBSCOPE_PORT        listen port              (default 8080)
#   HUBSCOPE_SYSTEMD_DIR systemd unit directory   (default /etc/systemd/system;
#                        exists so tests never touch the real system)
set -euo pipefail

PREFIX="${HUBSCOPE_PREFIX:-/usr/local}"
DATA_DIR="${HUBSCOPE_DATA_DIR:-/var/lib/hubscope}"
SVC_USER="${HUBSCOPE_USER:-hubscope}"
PORT="${HUBSCOPE_PORT:-8080}"
SYSTEMD_DIR="${HUBSCOPE_SYSTEMD_DIR:-/etc/systemd/system}"
SERVICE_NAME="hubscope"
HEALTH_TIMEOUT_SECONDS=30

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log()  { printf '[hubscope-install] %s\n' "$*"; }
fail() { printf '[hubscope-install] ERROR: %s\n' "$*" >&2; exit 1; }

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
  require_cmd go "Go toolchain: https://go.dev/dl/"
  require_cmd pnpm "pnpm: https://pnpm.io/installation"
}

build_binary() {
  log "building hubscope binary (make build)..."
  (cd "$REPO_ROOT" && make build)
  [ -f "$REPO_ROOT/bin/hubscope" ] || fail "make build did not produce bin/hubscope"
}

# BUILD_OUTPUT lets tests redirect the artifact install_binary() picks up;
# production never sets it (defaults to the make build product).
install_binary() {
  local source="${BUILD_OUTPUT:-$REPO_ROOT/bin/hubscope}"
  log "installing binary to $PREFIX/bin/hubscope"
  as_root mkdir -p "$PREFIX/bin"
  as_root install -m 0755 "$source" "$PREFIX/bin/hubscope"
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
  1. Create the first super admin (replace with your own strong password):
       $PREFIX/bin/hubscope admin create --username admin --password 'REPLACE-WITH-STRONG-PASSWORD'
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
  build_binary
  install_binary
  ensure_user
  ensure_data_dir
  render_unit
  restart_service
  wait_healthy
  print_guidance
}

main "$@"
