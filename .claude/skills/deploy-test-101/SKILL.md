---
name: deploy-test-101
description: 测试线部署流程(192.168.1.101):使用 deploy.sh 脚本自动化部署,支持开发/标签两种模式,包含备份、健康检查、账号初始化和回滚功能。
trigger: user-invocable
---

# 测试线部署流程(192.168.1.101)

## 环境信息
- **容器名**: `hubscope`
- **端口**: `192.168.1.101:8080:8080`
- **容器用户**: uid 10001 (非 root)
- **健康检查**: `http://192.168.1.101:8080/healthz`
- **数据卷**: `${DATA_DIR}/hubscope` → `/data` (容器内)
  - 默认 `DATA_DIR=$HOME/data`，可通过环境变量覆盖
- **配置**: 支持项目根目录的 `.env.local` 文件（会被 Git 忽略）

## 快速开始

使用提供的部署脚本（推荐）:

```bash
# 开发部署（从当前工作目录构建）
./.claude/skills/deploy-test-101/scripts/deploy.sh dev

# 标签部署（从指定 git tag 构建）
./.claude/skills/deploy-test-101/scripts/deploy.sh tag v0.2.3

# 回滚到上一个版本
./.claude/skills/deploy-test-101/scripts/deploy.sh rollback

# 查看帮助
./.claude/skills/deploy-test-101/scripts/deploy.sh help

# 禁用 Docker 构建缓存
./.claude/skills/deploy-test-101/scripts/deploy.sh dev --no-cache

# 调试模式
DEBUG=1 ./.claude/skills/deploy-test-101/scripts/deploy.sh dev
```

**配置 (.env.local)**:
```bash
# 1. 复制示例文件
cp .env.local.example .env.local

# 2. 编辑 .env.local，根据你的环境修改配置
# 主要配置项：
#   DATA_DIR=$HOME/data           # 数据目录（默认 $HOME/data）
#   BACKUP_DIR=/path/to/backups  # 备份目录（默认 $DATA_DIR/hubscope-backups）
#   DEBUG=1                      # 启用调试输出（可选）

# 3. .env.local 会被 Git 忽略，不会提交到仓库
# 注意：.env.local.example 是示例文件，会被提交到仓库
```

## 部署模式
- **标签部署(生产)**: 从 git tag 构建 → `hubscope:vX.Y.Z`
- **开发部署(测试)**: 从当前工作目录构建 → `hubscope:dev`,版本号自动生成 `dev-YYYYMMDD-HHMMSS`

## 脚本功能

`deploy.sh` 脚本自动执行以下步骤：

1. **依赖检查**: 验证 docker、git、curl 是否可用
2. **数据备份**: 备份数据库并记录回滚信息
3. **镜像构建**: 根据模式构建 Docker 镜像
4. **容器管理**: 停止旧容器、启动新容器
5. **权限修复**: 设置数据目录为 uid 10001
6. **健康检查**: 验证服务可用性（30 次重试，60 秒超时）
7. **账号初始化**: 首次部署时创建默认管理员账号
8. **部署验证**: 显示版本、容器状态、前端哈希、数据库大小
9. **镜像清理**: 自动删除超过 7 天的旧镜像（保留最近 3 个版本）

## 手动部署流程（备选方案）

如果需要手动执行部署步骤，可参考以下流程：

<details>
<summary>点击展开手动部署步骤</summary>

### 1. 备份数据
```bash
TS=$(date +%Y%m%d-%H%M%S)
DATA_DIR=${DATA_DIR:-$HOME/data}
BACKUP_DIR="$DATA_DIR/hubscope-backups"
mkdir -p $BACKUP_DIR

if [ -f $DATA_DIR/hubscope/app.db ]; then
  sudo cp $DATA_DIR/hubscope/app.db $BACKUP_DIR/app.db.bak-$TS
fi

PREV_IMAGE=$(docker inspect hubscope --format '{{.Config.Image}}' 2>/dev/null || echo "none")
echo "Previous image: $PREV_IMAGE" > $BACKUP_DIR/rollback-$TS.txt
```

### 2. 构建镜像

**标签部署(生产)**:
```bash
TAG=$(git describe --tags --abbrev=0)
BUILD_DIR="/tmp/hubscope-build-$TAG"
rm -rf $BUILD_DIR && mkdir -p $BUILD_DIR
git archive $TAG | tar -x -C $BUILD_DIR
cd $BUILD_DIR && docker build --build-arg VERSION=$TAG -t hubscope:$TAG . && cd -
rm -rf $BUILD_DIR
IMAGE_TAG="hubscope:$TAG"
```

**开发部署(测试)**:
```bash
docker build -t hubscope:dev .
DEV_VERSION=$(docker run --rm hubscope:dev --version 2>&1 | grep -oP 'dev-\d{8}-\d{6}')
IMAGE_TAG="hubscope:dev"
```

### 3. 停止旧容器
```bash
docker stop hubscope 2>/dev/null
docker rm hubscope 2>/dev/null
```

### 4. 修复数据目录权限
```bash
DATA_DIR=${DATA_DIR:-$HOME/data}
sudo mkdir -p $DATA_DIR/hubscope
sudo chown -R 10001:10001 $DATA_DIR/hubscope
```

### 5. 启动新容器
```bash
docker run -d --name hubscope \
  -p 192.168.1.101:8080:8080 \
  -v $DATA_DIR/hubscope:/data \
  $IMAGE_TAG
```

### 6. 健康检查
```bash
for i in {1..30}; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://192.168.1.101:8080/healthz 2>/dev/null)
  [ "$code" = "200" ] && { echo "✓ Healthy"; break; }
  sleep 2
done
```

### 7. 初始化管理员账号(仅首次)
```bash
# 检查是否已有账号(通过尝试创建来探测,错误信息"username already taken"表示已存在)
ADMIN_EXISTS=$(docker exec hubscope sh -c 'hubscope admin create --username admin --password "test123456" 2>&1' | grep -c "already taken")

if [ "$ADMIN_EXISTS" -eq 0 ]; then
  # 首次部署,创建默认账号
  docker exec hubscope sh -c 'hubscope admin create --username admin --password "HubScope2026!"'
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "✓ 默认账号: admin / HubScope2026!"
  echo "⚠ 请登录后立即修改密码"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi
```

### 8. 验证部署
```bash
docker exec hubscope hubscope --version
docker ps --filter name=hubscope
curl -s http://192.168.1.101:8080/ | grep -o 'index-[^"]*\.js'  # 前端资源哈希
ls -lh $DATA_DIR/hubscope/app.db  # 数据库大小(确认不是空库)
```

</details>

## 回滚

使用脚本回滚（推荐）:
```bash
./.claude/skills/deploy-test-101/scripts/deploy.sh rollback
```

手动回滚:
```bash
# 1. 找到回滚信息
ROLLBACK_FILE=$(ls -t $BACKUP_DIR/rollback-*.txt | head -1)
PREV_IMAGE=$(grep "Previous image:" $ROLLBACK_FILE | cut -d' ' -f3)

# 2. 恢复数据库(可选)
read -p "Restore database backup? (y/N) " -n 1 -r && [[ $REPLY =~ ^[Yy]$ ]] && \
  sudo cp $BACKUP_DIR/app.db.bak-$(basename $ROLLBACK_FILE .txt | sed 's/rollback-//') $DATA_DIR/hubscope/app.db && \
  sudo chown 10001:10001 $DATA_DIR/hubscope/app.db

# 3. 重启旧版本
docker rm -f hubscope
docker run -d --name hubscope -p 192.168.1.101:8080:8080 -v $DATA_DIR/hubscope:/data $PREV_IMAGE
sudo chown -R 10001:10001 $DATA_DIR/hubscope
```

## 关键经验

1. **数据库位置**: 生产数据在 `$DATA_DIR/hubscope/app.db`,开发目录(`$CODE_DIR/data/app.db`)可能有旧数据;恢复前确认文件大小和修改时间。

2. **权限是第一杀手**: 容器以 uid 10001 运行,每次重建容器前必须 `chown -R 10001:10001 $DATA_DIR/hubscope`,否则报 `unable to open database file (14)`。

3. **账号创建必须在容器内**: 用 `docker exec hubscope sh -c 'hubscope admin create ...'`,不要用宿主机二进制(权限不匹配会报 `readonly database`)。

4. **前端嵌入 Go 二进制**: 前端变更必须重新 `docker build`,不能仅 `docker restart`;验证方法是检查前端资源哈希值变化。

5. **版本号规范**: 生产 `vX.Y.Z`(git tag),开发 `dev-YYYYMMDD-HHMMSS`(自动生成,可追溯构建时间)。

6. **首次部署初始化**: 检测账号不存在时创建默认超管(admin/HubScope2026!),已有则跳过;检测方式是尝试创建并捕获"username already taken"错误。

7. **数据迁移安全**: SQLite 迁移失败不破坏数据(事务回滚),但容器启动失败;始终保留最近 3 个备份。

8. **健康检查必要性**: 不能仅依赖 `docker ps` STATUS,必须验证 `/healthz` 返回 200;容器可能运行但服务异常。

## 常见问题

**容器启动后立即退出**: `docker logs hubscope` 查看日志;常见原因是权限问题、端口冲突(`sudo lsof -i :8080`)、迁移失败。

**前端更新不生效**: 确认 `docker build` 后前端哈希值变化(`curl ... | grep index-`);若未变,尝试 `docker build --no-cache`。

**数据丢失**: 从 `$BACKUP_DIR` 恢复最近备份;查找所有可能位置 `find ~ -name "app.db" -exec ls -lh {} \;`。

**忘记密码**: 停止容器后用 `docker exec hubscope sh -c 'hubscope admin create --username newadmin --password "..."'` 创建新账号。
