#!/bin/bash
set -euo pipefail

# Load .env.local if it exists
if [ -f .env.local ]; then
  # shellcheck source=/dev/null
  source .env.local
fi

# Configuration
DATA_DIR=${DATA_DIR:-$HOME/data}
BACKUP_DIR=${BACKUP_DIR:-$DATA_DIR/hubscope-backups}
CONTAINER_NAME=hubscope
PORT=${PORT:-8080}
HOST_IP=${HOST_IP:-192.168.1.101}
OPS_DIR=${OPS_DIR:-/opt/hubscope/bin}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

# Set by backup_data: 1 when no app.db existed before this deployment
# (fresh install → init_admin prompts for interactive super_admin creation), 0 otherwise.
DB_FRESH=0

# Icons
ICON_SUCCESS="✓"
ICON_ERROR="✗"
ICON_INFO="ℹ"
ICON_WARNING="⚠"

# Logging functions
log_info() {
  echo "$ICON_INFO $1"
}

log_success() {
  echo "$ICON_SUCCESS $1"
}

log_error() {
  echo "$ICON_ERROR $1"
}

log_warning() {
  echo "$ICON_WARNING $1"
}

# Error handler
error_handler() {
  log_error "Error on line $1"
  exit 1
}

trap 'error_handler $LINENO' ERR

# Check dependencies
check_dependencies() {
  local deps=(docker git curl)
  for dep in "${deps[@]}"; do
    if ! command -v "$dep" &> /dev/null; then
      log_error "$dep is not installed"
      exit 1
    fi
  done
}

# Debug logging
debug() {
  if [ "${DEBUG:-0}" = "1" ]; then
    echo "[DEBUG] $1"
  fi
}

# Show help
show_help() {
  cat << EOF
Usage: deploy.sh <command> [options]

Commands:
  dev              Deploy from current working directory (development)
  tag <version>    Deploy from git tag (production)
  rollback         Rollback to previous deployment
  help             Show this help message

Options:
  --no-cache       Disable Docker build cache

Environment Variables:
  DATA_DIR         Data directory (default: \$HOME/data)
  BACKUP_DIR       Backup directory (default: \$DATA_DIR/hubscope-backups)
  DEBUG            Enable debug output (default: 0)

Examples:
  deploy.sh dev
  deploy.sh tag v0.2.3
  deploy.sh rollback

Configuration:
  Create .env.local file in project root to set environment variables:
    DATA_DIR=/path/to/data
    BACKUP_DIR=/path/to/backups
EOF
}

# Check git status (for tag deployment)
check_git_status() {
  if [ -n "$(git status --porcelain)" ]; then
    log_error "Git working directory is not clean"
    log_info "Please commit or stash your changes before deploying from tag"
    exit 1
  fi
}

# Backup data
backup_data() {
  local ts
  ts=$(date +%Y%m%d-%H%M%S)

  log_info "Backing up data..."
  mkdir -p "$BACKUP_DIR"

  # Record whether the database exists BEFORE this deployment. init_admin
  # uses this flag: a default super_admin is created only when the service
  # starts against a fresh (empty) database — never on top of existing data.
  if [ -s "$DATA_DIR/hubscope/app.db" ]; then
    DB_FRESH=0
    sudo cp "$DATA_DIR/hubscope/app.db" "$BACKUP_DIR/app.db.bak-$ts"
    local db_size
    db_size=$(du -h "$DATA_DIR/hubscope/app.db" | cut -f1)
    log_success "Database backed up ($db_size) to $BACKUP_DIR/app.db.bak-$ts"
  else
    DB_FRESH=1
    log_warning "No existing database found (first deployment)"
  fi

  local prev_image
  prev_image=$(docker inspect $CONTAINER_NAME --format '{{.Config.Image}}' 2>/dev/null || echo "none")
  echo "Previous image: $prev_image" > "$BACKUP_DIR/rollback-$ts.txt"
  log_success "Rollback info saved to $BACKUP_DIR/rollback-$ts.txt"
}

# Build image
build_image() {
  local mode=$1
  local version=${2:-}
  local no_cache=${3:-false}

  log_info "Building image..."

  local build_args=""
  if [ "$no_cache" = "true" ]; then
    build_args="--no-cache"
  fi

  if [ "$mode" = "tag" ]; then
    log_info "Building from tag: $version"
    local build_dir="/tmp/hubscope-build-$version"
    rm -rf "$build_dir"
    mkdir -p "$build_dir"
    git archive "$version" | tar -x -C "$build_dir"

    cd "$build_dir"
    docker build $build_args --build-arg VERSION="$version" -t "$CONTAINER_NAME:$version" .
    cd - > /dev/null
    rm -rf "$build_dir"

    IMAGE_TAG="$CONTAINER_NAME:$version"
    log_success "Built production image: $IMAGE_TAG"
  else
    log_info "Building from current working directory (development)"
    # Pass a fresh timestamp as both VERSION and CACHEBUST on every dev deploy:
    # VERSION busts the Go build layer (fresh version stamp) and CACHEBUST busts
    # the frontend build layer — both frontend and backend are recompiled from
    # the current code, never reused from a stale cached layer.
    # Local time (no -u): all deploys happen in one timezone, wall-clock
    # readability beats UTC purity on the test line.
    local dev_version
    dev_version="dev-$(date +%Y%m%d-%H%M%S)"
    docker build $build_args \
      --build-arg VERSION="$dev_version" \
      --build-arg CACHEBUST="$dev_version" \
      -t "$CONTAINER_NAME:dev" .

    IMAGE_TAG="$CONTAINER_NAME:dev"
    log_success "Built development image: $IMAGE_TAG ($dev_version)"
  fi
}

# Stop and remove old container
stop_container() {
  log_info "Stopping old container..."
  docker stop $CONTAINER_NAME 2>/dev/null || log_info "Container was not running"
  docker rm $CONTAINER_NAME 2>/dev/null || log_info "Container did not exist"
}

# Fix data directory permissions
fix_permissions() {
  log_info "Fixing data directory permissions..."
  sudo mkdir -p "$DATA_DIR/hubscope"
  sudo chown -R 10001:10001 "$DATA_DIR/hubscope"
  log_success "Data directory ownership set to 10001:10001"
}

# Start new container
start_container() {
  log_info "Starting new container..."
  # TRUST_PROXY is deliberately NOT set (spec 0011 decision 2): this line
  # exposes the container directly on the LAN (bound to the host IP below)
  # with no reverse proxy in front — verified 2026-07-27: no 80/443 listener
  # on the host, nginx absent/inactive, and /healthz answers with HubScope's
  # own security headers (no Via / Server: nginx). Trusting X-Forwarded-For
  # without a proxy would let any client forge the rate-limit key and the
  # audit IP — strictly worse than not trusting it. Do not "helpfully" add it.
  docker run -d --name $CONTAINER_NAME \
    -p "$HOST_IP:$PORT:8080" \
    -v "$DATA_DIR/hubscope:/data" \
    "$IMAGE_TAG"
  log_success "Container started: $IMAGE_TAG"
}

# Health check
health_check() {
  log_info "Waiting for service to be healthy..."
  local healthy=0
  for i in {1..30}; do
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "http://$HOST_IP:$PORT/healthz" 2>/dev/null)
    if [ "$code" = "200" ]; then
      log_success "Health check passed (attempt $i)"
      healthy=1
      break
    fi
    echo -ne "⏳ Waiting... ($i/30)\r"
    sleep 2
  done
  echo ""

  if [ $healthy -eq 0 ]; then
    log_error "Health check failed after 60 seconds"
    log_info "Container logs:"
    docker logs $CONTAINER_NAME --tail 20
    exit 1
  fi
}

# Initialize admin account (first deployment only). Gated on DB_FRESH —
# the flag backup_data sets when no app.db existed before this deployment.
# A fresh database means no users, so the default super_admin is safe to
# create. Existing data is never touched, and no probe-with-side-effects
# is needed to detect it.
# Sync standardized ops scripts (scripts/ops/) to the host — same toolset on
# both lines (test + prod), see spec 0008 decision 10
sync_ops_scripts() {
  log_info "Syncing ops scripts to $OPS_DIR ..."
  sudo mkdir -p "$OPS_DIR"
  for f in "$REPO_ROOT"/scripts/ops/hubscope-*; do
    sudo cp "$f" "$OPS_DIR/"
  done
  sudo chmod +x "$OPS_DIR"/hubscope-*
  log_success "Ops scripts synced"
}

init_admin() {
  if [ "${DB_FRESH:-0}" -ne 1 ]; then
    log_info "Existing database detected, skipping admin account creation"
    return
  fi

  # W6 credential boundary: never create a default-password account (a default
  # password in a public repo = anyone can log in). On a fresh DB, prompt for
  # interactive creation; the password is read with read -s and never persisted
  echo ""
  read -r -p "Fresh database detected. Create super_admin now? [Y/n]: " answer
  if [ "$answer" = "n" ] || [ "$answer" = "N" ]; then
    log_warning "Skipped. Create one later with: $OPS_DIR/hubscope-admin-create"
    return
  fi
  "$OPS_DIR/hubscope-admin-create"
}

# Verify deployment
verify_deployment() {
  echo ""
  echo "=== Deployment Verification ==="

  # Check version
  echo "1. Version:"
  docker exec $CONTAINER_NAME hubscope --version

  # Check container status
  echo ""
  echo "2. Container:"
  docker ps --filter name=$CONTAINER_NAME --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

  # Check frontend assets
  echo ""
  echo "3. Frontend assets:"
  local asset_hash
  asset_hash=$(curl -s "http://$HOST_IP:$PORT/" | grep -o 'index-[^"]*\.js' | head -1)
  echo "   $asset_hash"

  # Check database size
  echo ""
  echo "4. Database:"
  ls -lh "$DATA_DIR/hubscope/app.db" 2>/dev/null || echo "   No database file"

  echo ""
  log_success "Deployment completed successfully!"
  log_info "Access: http://$HOST_IP:$PORT/"
}

# Cleanup old images (keep last 3 versions, delete older than 7 days)
cleanup_images() {
  log_info "Cleaning up old Docker images..."

  # Get all hubscope images sorted by creation date (newest first)
  local images
  images=$(docker images --filter "reference=$CONTAINER_NAME" --format "{{.ID}} {{.CreatedAt}}" | sort -k2 -r)

  local count=0
  local seven_days_ago
  seven_days_ago=$(date -d '7 days ago' +%s 2>/dev/null || date -v-7d +%s)

  echo "$images" | while read -r id created_at; do
    count=$((count + 1))

    # Always keep the first 3 versions
    if [ $count -le 3 ]; then
      debug "Keeping image $id (in top 3)"
      continue
    fi

    # Check if image is older than 7 days
    local created_ts
    created_ts=$(date -d "$created_at" +%s 2>/dev/null || date -j -f "%Y-%m-%d %H:%M:%S" "$created_at" +%s 2>/dev/null || echo "0")

    if [ "$created_ts" -lt "$seven_days_ago" ] && [ "$created_ts" != "0" ]; then
      log_info "Removing old image: $id (created: $created_at)"
      docker rmi "$id" 2>/dev/null || log_warning "Failed to remove image $id"
    else
      debug "Keeping image $id (less than 7 days old)"
    fi
  done

  log_success "Cleanup completed"
}

# Test gate: backend tests + frontend test/typecheck/build must pass before
# anything is deployed. Deliberately NOT `make test`: that target also runs
# install-test, which validates scripts/install.sh (the production install
# path) — irrelevant to the docker deploy path — and two of its cases assume
# a clean machine (no hubscope user, go outside /usr/bin), so they can never
# pass on the test server itself. Runs against the working tree for dev
# deploys; tag deploys rely on the release gate (CI green) that produced the tag.
run_tests() {
  log_info "Running tests (backend + frontend)..."
  make backend-test
  make frontend-test
  log_success "All tests passed"
}

# Deploy from current working directory (development)
deploy_dev() {
  local no_cache=${1:-false}

  log_info "Starting development deployment..."

  check_dependencies
  run_tests
  backup_data
  build_image "dev" "" "$no_cache"
  stop_container
  fix_permissions
  start_container
  health_check
  sync_ops_scripts
  init_admin
  verify_deployment
  cleanup_images
}

# Deploy from git tag (production)
deploy_tag() {
  local version=$1
  local no_cache=${2:-false}

  log_info "Starting production deployment from tag: $version"

  check_dependencies
  check_git_status
  backup_data
  build_image "tag" "$version" "$no_cache"
  stop_container
  fix_permissions
  start_container
  health_check
  sync_ops_scripts
  init_admin
  verify_deployment
  cleanup_images

  # Tag as latest
  docker tag "$CONTAINER_NAME:$version" "$CONTAINER_NAME:latest"
  log_success "Tagged $CONTAINER_NAME:$version as latest"
}

# Rollback to previous deployment
rollback() {
  log_info "Rolling back to previous deployment..."

  # Find latest rollback info
  local rollback_file
  # shellcheck disable=SC2012  # mtime ordering is required; find expresses this worse
  rollback_file=$(ls -t "$BACKUP_DIR"/rollback-*.txt 2>/dev/null | head -1)
  if [ -z "$rollback_file" ]; then
    log_error "No rollback info found"
    exit 1
  fi

  local prev_image
  prev_image=$(grep "Previous image:" "$rollback_file" | cut -d' ' -f3)
  local backup_ts
  backup_ts=$(basename "$rollback_file" .txt | sed 's/rollback-//')

  log_info "Previous image: $prev_image"
  log_info "Backup timestamp: $backup_ts"

  # Ask about database restore
  read -p "Restore database backup? (y/N) " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    local backup_file="$BACKUP_DIR/app.db.bak-$backup_ts"
    if [ -f "$backup_file" ]; then
      docker stop $CONTAINER_NAME 2>/dev/null || true
      sudo cp "$backup_file" "$DATA_DIR/hubscope/app.db"
      sudo chown 10001:10001 "$DATA_DIR/hubscope/app.db"
      log_success "Database restored from $backup_file"
    else
      log_warning "Backup file not found: $backup_file"
    fi
  fi

  # Start old version
  stop_container
  # No TRUST_PROXY here either — same reason as start_container: no reverse
  # proxy in front of this line (verified 2026-07-27), so trusting
  # X-Forwarded-For would make the rate-limit key and audit IP client-forgeable.
  docker run -d --name $CONTAINER_NAME \
    -p "$HOST_IP:$PORT:8080" \
    -v "$DATA_DIR/hubscope:/data" \
    "$prev_image"

  fix_permissions

  # Health check
  log_info "Verifying rollback..."
  local healthy=0
  for i in {1..30}; do
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "http://$HOST_IP:$PORT/healthz" 2>/dev/null)
    if [ "$code" = "200" ]; then
      log_success "Rollback successful (attempt $i)"
      healthy=1
      break
    fi
    sleep 2
  done

  if [ $healthy -eq 0 ]; then
    log_error "Rollback health check failed"
    docker logs $CONTAINER_NAME --tail 20
    exit 1
  fi
}

# Main
main() {
  local command=${1:-help}
  local version=${2:-}
  local no_cache=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case $1 in
      --no-cache)
        no_cache=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  case $command in
    dev)
      deploy_dev "$no_cache"
      ;;
    tag)
      if [ -z "$version" ]; then
        log_error "Tag version is required"
        show_help
        exit 1
      fi
      deploy_tag "$version" "$no_cache"
      ;;
    rollback)
      rollback
      ;;
    help|--help|-h)
      show_help
      ;;
    *)
      log_error "Unknown command: $command"
      show_help
      exit 1
      ;;
  esac
}

main "$@"
