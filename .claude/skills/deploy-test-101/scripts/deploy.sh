#!/bin/bash
set -euo pipefail

# Load .env.local if it exists
if [ -f .env.local ]; then
  source .env.local
fi

# Configuration
DATA_DIR=${DATA_DIR:-$HOME/data}
BACKUP_DIR=${BACKUP_DIR:-$DATA_DIR/hubscope-backups}
CONTAINER_NAME=hubscope
PORT=8080
HOST_IP=192.168.1.101

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
  local ts=$(date +%Y%m%d-%H%M%S)

  log_info "Backing up data..."
  mkdir -p "$BACKUP_DIR"

  if [ -f "$DATA_DIR/hubscope/app.db" ]; then
    sudo cp "$DATA_DIR/hubscope/app.db" "$BACKUP_DIR/app.db.bak-$ts"
    local db_size=$(ls -lh "$DATA_DIR/hubscope/app.db" | awk '{print $5}')
    log_success "Database backed up ($db_size) to $BACKUP_DIR/app.db.bak-$ts"
  else
    log_warning "No existing database found (first deployment)"
  fi

  local prev_image=$(docker inspect $CONTAINER_NAME --format '{{.Config.Image}}' 2>/dev/null || echo "none")
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
    docker build $build_args -t "$CONTAINER_NAME:dev" .

    local dev_version=$(docker run --rm "$CONTAINER_NAME:dev" --version 2>&1 | grep -oP 'dev-\d{8}-\d{6}' || echo "unknown")
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
  docker run -d --name $CONTAINER_NAME \
    -p "$HOST_IP:$PORT:$PORT" \
    -v "$DATA_DIR/hubscope:/data" \
    "$IMAGE_TAG"
  log_success "Container started: $IMAGE_TAG"
}

# Health check
health_check() {
  log_info "Waiting for service to be healthy..."
  local healthy=0
  for i in {1..30}; do
    local code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "http://$HOST_IP:$PORT/healthz" 2>/dev/null)
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

# Initialize admin account (first deployment only)
init_admin() {
  log_info "Checking admin accounts..."

  local admin_exists=$(docker exec $CONTAINER_NAME sh -c 'hubscope admin create --username admin --password "test123456" 2>&1' | grep -c "already taken" || echo "0")

  if [ "$admin_exists" -eq 0 ]; then
    log_info "No admin account found, creating default super_admin..."
    docker exec $CONTAINER_NAME sh -c 'hubscope admin create --username admin --password "HubScope2026!"'

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_success "Default super_admin account created:"
    echo "  Username: admin"
    echo "  Password: HubScope2026!"
    echo "  $ICON_WARNING Please change the password after first login!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
  else
    log_info "Admin accounts already exist, skipping account creation"
  fi
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
  local asset_hash=$(curl -s "http://$HOST_IP:$PORT/" | grep -o 'index-[^"]*\.js' | head -1)
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
  local images=$(docker images --filter "reference=$CONTAINER_NAME" --format "{{.ID}} {{.CreatedAt}}" | sort -k2 -r)

  local count=0
  local seven_days_ago=$(date -d '7 days ago' +%s 2>/dev/null || date -v-7d +%s)

  echo "$images" | while read -r id created_at; do
    count=$((count + 1))

    # Always keep the first 3 versions
    if [ $count -le 3 ]; then
      debug "Keeping image $id (in top 3)"
      continue
    fi

    # Check if image is older than 7 days
    local created_ts=$(date -d "$created_at" +%s 2>/dev/null || date -j -f "%Y-%m-%d %H:%M:%S" "$created_at" +%s 2>/dev/null || echo "0")

    if [ "$created_ts" -lt "$seven_days_ago" ] && [ "$created_ts" != "0" ]; then
      log_info "Removing old image: $id (created: $created_at)"
      docker rmi "$id" 2>/dev/null || log_warning "Failed to remove image $id"
    else
      debug "Keeping image $id (less than 7 days old)"
    fi
  done

  log_success "Cleanup completed"
}

# Deploy from current working directory (development)
deploy_dev() {
  local no_cache=${1:-false}

  log_info "Starting development deployment..."

  check_dependencies
  backup_data
  build_image "dev" "" "$no_cache"
  stop_container
  fix_permissions
  start_container
  health_check
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
  local rollback_file=$(ls -t "$BACKUP_DIR"/rollback-*.txt 2>/dev/null | head -1)
  if [ -z "$rollback_file" ]; then
    log_error "No rollback info found"
    exit 1
  fi

  local prev_image=$(grep "Previous image:" "$rollback_file" | cut -d' ' -f3)
  local backup_ts=$(basename "$rollback_file" .txt | sed 's/rollback-//')

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
  docker run -d --name $CONTAINER_NAME \
    -p "$HOST_IP:$PORT:$PORT" \
    -v "$DATA_DIR/hubscope:/data" \
    "$prev_image"

  fix_permissions

  # Health check
  log_info "Verifying rollback..."
  local healthy=0
  for i in {1..30}; do
    local code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "http://$HOST_IP:$PORT/healthz" 2>/dev/null)
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
