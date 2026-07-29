#!/bin/bash
set -euo pipefail

# HubScope production-line deploy script (runs on the dev machine, drives the
# remote host over ssh)
#
# Commands:
#   init           Idempotent bare-host bootstrap (Docker/Caddy/dirs/ops scripts/conflict checks)
#   tag <version>  Build from a git tag and deploy (the only production deploy mode)
#   rollback       Roll back to the previous image
#   status         Production status at a glance
#
# Config: .env.prod at the repo root (gitignored); see .env.prod.example
# Discipline: zero hardcoded sensitive values in this script — every real
# value comes from .env.prod (spec 0008 decision 8)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

# shellcheck source=/dev/null
if [ -f "$REPO_ROOT/.env.prod" ]; then
  source "$REPO_ROOT/.env.prod"
fi

[ "${DEBUG:-0}" = "1" ] && set -x

# ---- Configuration (defaults mirror .env.prod.example one-to-one) ----
PROD_HOST=${PROD_HOST:-}
PROD_SSH_PORT=${PROD_SSH_PORT:-22}
PROD_SSH_USER=${PROD_SSH_USER:-root}
PROD_DOMAIN=${PROD_DOMAIN:-}
PROD_CADDYFILE=${PROD_CADDYFILE:-/etc/caddy/Caddyfile}
PROD_DATA_DIR=${PROD_DATA_DIR:-/data/hubscope}
PROD_BACKUP_DIR=${PROD_BACKUP_DIR:-/data/hubscope-backups}
PROD_OPS_DIR=${PROD_OPS_DIR:-/opt/hubscope/bin}
PROD_PORT=${PROD_PORT:-8080}
PROD_DOCKER_YUM_MIRROR=${PROD_DOCKER_YUM_MIRROR:-https://mirrors.aliyun.com/docker-ce}
PROD_CADDY_VERSION=${PROD_CADDY_VERSION:-latest}
CONTAINER_NAME=hubscope
CADDY_CACHE_DIR="$HOME/.cache/hubscope"

ICON_SUCCESS="✓"; ICON_ERROR="✗"; ICON_INFO="ℹ"; ICON_WARNING="⚠"
log_info()    { echo "$ICON_INFO $1"; }
log_success() { echo "$ICON_SUCCESS $1"; }
log_error()   { echo "$ICON_ERROR $1" >&2; }
log_warning() { echo "$ICON_WARNING $1"; }

error_handler() {
  log_error "第 $1 行执行失败"
  exit 1
}
trap 'error_handler $LINENO' ERR

# ---- Remote execution wrappers ----
prod_ssh() {
  ssh -p "$PROD_SSH_PORT" "$PROD_SSH_USER@$PROD_HOST" "$@"
}
prod_ssh_tty() {
  ssh -t -p "$PROD_SSH_PORT" "$PROD_SSH_USER@$PROD_HOST" "$@"
}
prod_scp_to() {
  scp -P "$PROD_SSH_PORT" "$1" "$PROD_SSH_USER@$PROD_HOST:$2"
}

require_env() {
  local missing=()
  [ -z "$PROD_HOST" ] && missing+=("PROD_HOST")
  [ -z "$PROD_DOMAIN" ] && missing+=("PROD_DOMAIN")
  if [ ${#missing[@]} -gt 0 ]; then
    log_error ".env.prod 缺少必填变量: ${missing[*]}"
    log_info "参考 .env.prod.example 创建 $REPO_ROOT/.env.prod"
    exit 1
  fi
  command -v ssh >/dev/null || { log_error "ssh 未安装"; exit 1; }
  command -v docker >/dev/null || { log_error "本机 docker 未安装(构建镜像需要)"; exit 1; }
}

check_remote() {
  log_info "检查生产机连通性..."
  prod_ssh "echo ok" >/dev/null 2>&1 || {
    log_error "无法 ssh 到 $PROD_SSH_USER@$PROD_HOST:$PROD_SSH_PORT"
    exit 1
  }
  log_success "生产机可达"
}

show_help() {
  sed -n '3,15p' "$0" | sed 's/^# \?//'
}

# =============================================================================
# init — idempotent bare-host bootstrap
# =============================================================================

remote_caddy_unit() {
  prod_ssh "bash -s" <<'UNIT'
set -e
id caddy 2>/dev/null || useradd --system --home /var/lib/caddy --shell /sbin/nologin caddy
mkdir -p /etc/caddy /var/lib/caddy
chown -R caddy:caddy /var/lib/caddy
cat > /etc/systemd/system/caddy.service <<'EOF'
[Unit]
Description=Caddy
Documentation=https://caddyserver.com/docs/
After=network.target network-online.target
Requires=network-online.target

[Service]
Type=notify
User=caddy
Group=caddy
ExecStart=/usr/bin/caddy run --environ --config /etc/caddy/Caddyfile
ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile --force
TimeoutStopSec=5s
LimitNOFILE=1048576
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
UNIT
}

cmd_init() {
  require_env
  check_remote
  log_info "开始初始化(幂等,已就绪组件自动跳过)..."

  # 1. OS probe: fail loudly on unsupported distros instead of half-installing
  local os_id
  os_id=$(prod_ssh "source /etc/os-release && echo \$ID")
  case "$os_id" in
    centos|rhel|rocky|almalinux) log_success "OS: $os_id(yum/dnf 系,支持)" ;;
    *) log_error "不支持的发行版: $os_id(init 目前只适配 yum/dnf 系,详见 spec 0008 Out of Scope)"; exit 1 ;;
  esac

  # 2. Docker CE
  if prod_ssh "command -v docker >/dev/null 2>&1"; then
    log_success "Docker 已安装,跳过($(prod_ssh 'docker --version' | cut -d' ' -f3 | tr -d ','))"
  else
    log_info "安装 Docker CE(源: $PROD_DOCKER_YUM_MIRROR)..."
    prod_ssh "bash -s" <<EOF
set -e
yum install -y yum-utils
yum-config-manager --add-repo $PROD_DOCKER_YUM_MIRROR/linux/centos/docker-ce.repo
sed -i "s+https://download.docker.com+$PROD_DOCKER_YUM_MIRROR+" /etc/yum.repos.d/docker-ce.repo
yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
EOF
    log_success "Docker CE 安装完成"
  fi
  prod_ssh "systemctl is-active docker >/dev/null" || { log_error "docker 服务未运行"; exit 1; }

  # 3. Caddy (measured fact: downloading directly on the prod host is very
  #    slow, so the binary is cached on the dev machine and scp'd over)
  if prod_ssh "command -v caddy >/dev/null 2>&1"; then
    log_success "Caddy 已安装,跳过($(prod_ssh 'caddy version' | head -1 | awk '{print $1}'))"
  else
    log_info "安装 Caddy(开发机缓存 → scp)..."
    mkdir -p "$CADDY_CACHE_DIR"
    local caddy_bin="$CADDY_CACHE_DIR/caddy-${PROD_CADDY_VERSION}-linux-amd64"
    if [ ! -x "$caddy_bin" ]; then
      log_info "下载 Caddy $PROD_CADDY_VERSION 到开发机缓存..."
      curl -sL --fail --max-time 300 -o "$caddy_bin" \
        "https://caddyserver.com/api/download?os=linux&arch=amd64" || {
          log_error "Caddy 下载失败(可设 PROD_CADDY_VERSION 固定版本后重试)"; exit 1; }
      chmod +x "$caddy_bin"
    fi
    "$caddy_bin" version | head -1
    prod_scp_to "$caddy_bin" /usr/bin/caddy
    prod_ssh "chmod +x /usr/bin/caddy"
    log_success "Caddy 二进制就位"
  fi
  remote_caddy_unit
  log_success "caddy 用户/目录/systemd unit 就绪"

  # 4. Data and ops directories
  prod_ssh "mkdir -p '$PROD_DATA_DIR' '$PROD_BACKUP_DIR' '$PROD_OPS_DIR' && chown -R 10001:10001 '$PROD_DATA_DIR'"
  log_success "目录就绪: $PROD_DATA_DIR / $PROD_BACKUP_DIR / $PROD_OPS_DIR"

  # 5. Conflict checks: old deployments / port occupation → stop and report,
  #    never auto-remove (a human looks before anything is cleared)
  log_info "冲突检查(端口 $PROD_PORT / 旧原生部署残留)..."
  if prod_ssh "command -v sqlite3 >/dev/null 2>&1"; then
    log_success "sqlite3 可用(热备姿势)"
  else
    log_warning "sqlite3 不可用,备份将回落停服 cp;建议: yum install -y sqlite"
  fi
  local port_owner
  port_owner=$(prod_ssh "ss -tlnp | grep ':$PROD_PORT ' | grep -v docker-proxy || true")
  if [ -n "$port_owner" ]; then
    log_error "端口 $PROD_PORT 被非 Docker 进程占用:"
    echo "$port_owner" >&2
    log_error "请人工确认处置后再跑 init(脚本不会自动清除)"
    exit 1
  fi
  if prod_ssh "test -f /usr/local/bin/hubscope -o -f /etc/systemd/system/hubscope.service"; then
    log_error "检测到旧原生部署残留(/usr/local/bin/hubscope 或 hubscope.service)"
    log_error "一台机器只选一种部署模型;请人工确认数据与处置后再跑 init"
    exit 1
  fi
  log_success "无端口冲突、无旧部署残留"

  # 6. Firewall state (report and fix firewalld only; cloud security groups
  #    are beyond this script's reach)
  if prod_ssh "systemctl is-active firewalld >/dev/null 2>&1"; then
    log_info "firewalld 运行中,确保 80/443 放行..."
    prod_ssh "firewall-cmd --permanent --add-service=http --add-service=https && firewall-cmd --reload"
    log_success "firewalld 已放行 http/https"
  else
    log_info "firewalld 未运行(防火墙由其他层承担,如云平台安全组,请人工确认 80/443 已放行)"
  fi

  # 7. Sync ops scripts
  log_info "同步 scripts/ops → $PROD_OPS_DIR ..."
  for f in "$REPO_ROOT"/scripts/ops/hubscope-*; do
    prod_scp_to "$f" "$PROD_OPS_DIR/"
  done
  prod_ssh "chmod +x $PROD_OPS_DIR/hubscope-*"
  log_success "运维脚本已同步:$(find "$REPO_ROOT"/scripts/ops/ -maxdepth 1 -name 'hubscope-*' -printf '%f ')"

  # 8. Placeholder Caddyfile + start
  if prod_ssh "test -s '$PROD_CADDYFILE'"; then
    log_success "Caddyfile 已存在,保留现状"
  else
    log_info "写入占位 Caddyfile..."
    prod_ssh "bash -s" <<'EOF'
set -e
cat > /etc/caddy/Caddyfile <<'CADDY'
:80 {
	respond "HubScope coming soon" 200
}
CADDY
EOF
  fi
  prod_ssh "systemctl enable --now caddy"
  log_success "Caddy 服务运行中"

  echo ""
  log_success "初始化完成。下一步: deploy.sh tag <版本>"
}

# =============================================================================
# tag — tag deploy (the only production deploy mode)
# =============================================================================

# Compression contract: the edge terminates TLS, so response compression lives
# here, not in the Go binary — one `encode` covers both the embedded SPA assets
# (the ~1MB index-*.js dominates page load) and every /api JSON response.
# zstd preferred (browsers since 2024 send it in Accept-Encoding), gzip as the
# universal fallback; Caddy handles negotiation + Vary automatically.
# XFF trust contract (spec 0011 decision 2): the app reads X-Forwarded-For's
# first hop as clientIP, so the edge proxy MUST replace the client-supplied
# value. header_up (no +/- prefix) overwrites any existing value — including a
# spoofed leftmost hop — with the direct peer IP. (Since Caddy v2.7 the
# default already strips client-supplied XFF; the explicit header_up pins the
# replace contract regardless of version and guards against future
# trusted_proxies drift.)
# Future trap: if a CDN is ever placed in front of Caddy and trusted_proxies
# is configured, switch to {client_ip} — {remote_host} would then hard-
# overwrite the chain with the CDN node IP.
ensure_caddy_site() {
  if ! prod_ssh "grep -q '$PROD_DOMAIN' '$PROD_CADDYFILE' 2>/dev/null"; then
    log_info "写入 Caddy 站点块($PROD_DOMAIN → 127.0.0.1:$PROD_PORT)..."
    prod_ssh "bash -s" <<EOF
set -e
cat > '$PROD_CADDYFILE' <<'CADDY'
$PROD_DOMAIN {
	encode zstd gzip
	reverse_proxy 127.0.0.1:$PROD_PORT {
		header_up X-Forwarded-For {remote_host}
	}
}
CADDY
systemctl reload caddy
EOF
    log_success "Caddy 站点块已生效(首次证书申请约 15-30s)"
  elif prod_ssh "grep -q 'header_up X-Forwarded-For' '$PROD_CADDYFILE' 2>/dev/null && grep -q 'encode zstd gzip' '$PROD_CADDYFILE' 2>/dev/null"; then
    log_info "Caddy 站点块已存在且 XFF 替换与压缩(encode zstd gzip)已就位,跳过"
  else
    # Migration: the existing site block predates the XFF fix (spec 0011
    # decision 2) and/or the encode directive. Back up → rewrite → validate →
    # auto-restore on failure. The rewrite is a full-file write, same as the
    # create path (this script owns the file). Idempotent: a successful
    # migration lands in the skip branch above; a failed validation restores
    # the backup and the next run retries.
    log_info "存量 Caddy 站点块缺少 XFF 替换或压缩(encode zstd gzip),执行迁移(先备份)..."
    prod_ssh "bash -s" <<EOF
set -e
backup='$PROD_CADDYFILE.bak-'\$(date +%Y%m%d-%H%M%S)
cp -a '$PROD_CADDYFILE' "\$backup"
cat > '$PROD_CADDYFILE' <<'CADDY'
$PROD_DOMAIN {
	encode zstd gzip
	reverse_proxy 127.0.0.1:$PROD_PORT {
		header_up X-Forwarded-For {remote_host}
	}
}
CADDY
if ! caddy validate --config '$PROD_CADDYFILE' >/dev/null 2>&1; then
  cp -a "\$backup" '$PROD_CADDYFILE'
  systemctl reload caddy || true
  echo "Caddyfile 校验失败,已还原备份: \$backup" >&2
  exit 1
fi
systemctl reload caddy
echo "原配置备份: \$backup"
EOF
    log_success "存量站点块已迁移(XFF 替换与压缩就位)"
  fi
}

cmd_tag() {
  local version=${1:-}
  [ -n "$version" ] || { log_error "用法: deploy.sh tag <版本> (如 v0.2.4)"; exit 1; }
  require_env
  check_remote

  git -C "$REPO_ROOT" rev-parse "$version" >/dev/null 2>&1 || {
    log_error "本地不存在 git tag: $version"; exit 1; }

  # 1. Fresh-DB probe (drives the admin-creation prompt; must run before backup)
  local db_fresh=0
  prod_ssh "test -s '$PROD_DATA_DIR/app.db'" || db_fresh=1

  # 2. Mandatory pre-deploy backup (hot backup preferred, via remote ops script)
  log_info "部署前备份..."
  if ! prod_ssh "DATA_DIR='$PROD_DATA_DIR' BACKUP_DIR='$PROD_BACKUP_DIR' CONTAINER_NAME=$CONTAINER_NAME '$PROD_OPS_DIR/hubscope-backup'"; then
    if [ "$db_fresh" -eq 1 ]; then
      log_warning "首次部署,无数据可备份,继续"
    else
      log_error "备份失败,中止部署"
      exit 1
    fi
  fi

  # 3. Clean build on the dev machine
  log_info "从 tag $version 干净构建镜像..."
  local build_dir="/tmp/hubscope-build-$version"
  rm -rf "$build_dir"
  mkdir -p "$build_dir"
  git -C "$REPO_ROOT" archive "$version" | tar -x -C "$build_dir"
  docker build --build-arg VERSION="$version" -t "$CONTAINER_NAME:$version" "$build_dir"
  rm -rf "$build_dir"
  log_success "镜像构建完成: $CONTAINER_NAME:$version"

  # 4. Transfer and load (the prod host never pulls images from the internet)
  log_info "传输镜像到生产机..."
  local tarball="/tmp/hubscope-$version.tar.gz"
  docker save "$CONTAINER_NAME:$version" | gzip > "$tarball"
  prod_scp_to "$tarball" /tmp/
  prod_ssh "docker load -i /tmp/hubscope-$version.tar.gz && rm -f /tmp/hubscope-$version.tar.gz"
  rm -f "$tarball"
  log_success "镜像已载入生产机"

  # 5. Swap the container (permission fix before start is mandatory — the
  #    number-one killer lesson)
  log_info "更换容器..."
  prod_ssh "bash -s" <<EOF
set -e
chown -R 10001:10001 '$PROD_DATA_DIR'
docker stop $CONTAINER_NAME 2>/dev/null || true
docker rm $CONTAINER_NAME 2>/dev/null || true
# TRUST_PROXY=true: production sits behind Caddy, whose site block replaces
# X-Forwarded-For with the direct peer IP (spec 0011 decision 2)
docker run -d --name $CONTAINER_NAME --restart unless-stopped \
  -p 127.0.0.1:$PROD_PORT:$PROD_PORT \
  -e TRUST_PROXY=true \
  -v '$PROD_DATA_DIR':/data \
  $CONTAINER_NAME:$version
EOF

  # 6. Caddy site (idempotent)
  ensure_caddy_site

  # 7. Health checks: local + public
  log_info "健康检查..."
  local healthy=0 i code
  for i in $(seq 1 30); do
    code=$(prod_ssh "curl -s -o /dev/null -w '%{http_code}' --max-time 3 http://127.0.0.1:$PROD_PORT/healthz" 2>/dev/null || true)
    if [ "$code" = "200" ]; then
      log_success "本地 healthz 200(第 $i 次)"
      healthy=1
      break
    fi
    sleep 2
  done
  if [ "$healthy" -ne 1 ]; then
    log_error "健康检查失败。回滚: deploy.sh rollback;日志: ssh 后 docker logs $CONTAINER_NAME"
    exit 1
  fi
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "https://$PROD_DOMAIN/healthz" 2>/dev/null || true)
  if [ "$code" = "200" ]; then
    log_success "公网 https://$PROD_DOMAIN/healthz 200"
  else
    log_warning "公网检查未通过(code=$code;首次证书申请可能需要 ~30s,稍后复验)"
  fi

  # 8. Version verification (the --version trap: binaries at v0.2.3 and older
  #    lack the flag and would START A SERVER grabbing the port instead)
  local oldest_modern
  oldest_modern=$(printf '%s\n' "v0.2.3" "$version" | sort -V | head -1)
  if [ "$oldest_modern" = "v0.2.3" ] && [ "$version" != "v0.2.3" ]; then
    prod_ssh "docker exec $CONTAINER_NAME hubscope --version" || true
  else
    log_info "该版本无 --version flag(v0.2.4 才引入),版本溯源 = 镜像 tag $version"
  fi

  # 9. Fresh DB → prompt for interactive admin creation (read -s, nothing persisted)
  if [ "$db_fresh" -eq 1 ]; then
    echo ""
    read -r -p "检测到全新数据库,现在创建 super_admin? [Y/n]: " answer
    if [ "$answer" != "n" ] && [ "$answer" != "N" ]; then
      prod_ssh_tty "DATA_DIR='$PROD_DATA_DIR' '$PROD_OPS_DIR/hubscope-admin-create'"
    else
      log_warning "已跳过;稍后请执行: ssh 到生产机运行 $PROD_OPS_DIR/hubscope-admin-create"
    fi
  fi

  echo ""
  log_success "部署完成: $CONTAINER_NAME:$version @ https://$PROD_DOMAIN/"
}

# =============================================================================
# rollback — roll back to the previous image
# =============================================================================

cmd_rollback() {
  require_env
  check_remote

  local prev_image
  prev_image=$(prod_ssh "cat \$(ls -t '$PROD_BACKUP_DIR'/rollback-*.txt 2>/dev/null | head -1) 2>/dev/null | sed 's/Previous image: //'")
  if [ -z "$prev_image" ] || [ "$prev_image" = "none" ]; then
    log_error "找不到可回滚的镜像记录($PROD_BACKUP_DIR/rollback-*.txt)"
    exit 1
  fi
  log_info "回滚到镜像: $prev_image"
  prod_ssh "'$PROD_OPS_DIR/hubscope-status'" 2>/dev/null || true
  read -r -p "确认回滚? [y/N]: " answer
  [ "$answer" = "y" ] || [ "$answer" = "Y" ] || { log_info "已取消"; exit 0; }

  prod_ssh "bash -s" <<EOF
set -e
chown -R 10001:10001 '$PROD_DATA_DIR'
docker stop $CONTAINER_NAME && docker rm $CONTAINER_NAME
# TRUST_PROXY=true: production sits behind Caddy, whose site block replaces
# X-Forwarded-For with the direct peer IP (spec 0011 decision 2)
docker run -d --name $CONTAINER_NAME --restart unless-stopped \
  -p 127.0.0.1:$PROD_PORT:$PROD_PORT \
  -e TRUST_PROXY=true \
  -v '$PROD_DATA_DIR':/data \
  $prev_image
EOF

  local i code
  for i in $(seq 1 30); do
    code=$(prod_ssh "curl -s -o /dev/null -w '%{http_code}' --max-time 3 http://127.0.0.1:$PROD_PORT/healthz" 2>/dev/null || true)
    if [ "$code" = "200" ]; then
      log_success "回滚成功,healthz 200(第 $i 次)"
      exit 0
    fi
    sleep 2
  done
  log_error "回滚后健康检查失败,查日志: docker logs $CONTAINER_NAME;必要时恢复备份: $PROD_OPS_DIR/hubscope-restore"
  exit 1
}

# =============================================================================
# status — production status at a glance
# =============================================================================

cmd_status() {
  require_env
  prod_ssh "DATA_DIR='$PROD_DATA_DIR' BACKUP_DIR='$PROD_BACKUP_DIR' CONTAINER_NAME=$CONTAINER_NAME PORT=$PROD_PORT '$PROD_OPS_DIR/hubscope-status'"
}

# =============================================================================

case "${1:-help}" in
  init)     cmd_init ;;
  tag)      cmd_tag "${2:-}" ;;
  rollback) cmd_rollback ;;
  status)   cmd_status ;;
  help|--help|-h) show_help ;;
  *) log_error "未知命令: $1"; show_help; exit 1 ;;
esac
