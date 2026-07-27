---
name: ops
description: 发布与部署流程:发布前检查(门禁/打包/部署要件/数据面/配置面/核心闭环抽查)+ 内网部署流水线骨架(tag 提议+确认/交叉编译/docker import/备份/健康检查/自动回滚,变量化)。打包、打 tag 或部署前使用;执行动作仍需用户明确指令。
---

# 发布与部署流程

发布动作(push、tag、部署)只在用户明确指令后执行;本 skill 只做检查与准备,直到用户明确说「发布/部署」才执行流水线。**发版动作的唯一入口是 `scripts/release.sh vX.Y.Z`,完整流程(含 PR 合并规范)见 [docs/releasing.md](../../../docs/releasing.md);本 skill 是发版前的核对项与内网部署流水线骨架,不替代该流程。**

## A. 发布前检查清单

1. **门禁全绿**:`make test` 通过;`git status` 干净;commit 历史全为英文 Conventional Commits。
2. **打包**:`make package`(单二进制 + Dockerfile + 部署文档 tar 包);本机无 Docker,Dockerfile 只做静态检查,如实告知未实际构建镜像。
3. **部署要件核对**(docs/deployment.md):
   - 首个 `super_admin` 用 `hubscope admin create` CLI 引导(不读环境变量、不写入任何文件);`DATA_PATH`、`ADDR`、`LOG_LEVEL` 经环境变量;
   - 出网代理:目标环境若有 fake-ip 类代理,必须设 `HTTPS_PROXY`,启动日志首行会打印生效代理,核对之;
   - 反向代理后设 `TRUST_PROXY=true`(否则限流/审计取不到真实 IP)。
4. **数据面**:目标机的 app.db 备份;schema 自动迁移只加不删,升级无需手工 SQL,但降级不支持——提示用户确认。
5. **配置面**:飞书 webhook、裁判模型等 settings 在管理后台核对(不入库外任何地方)。
6. **发布后验证**:服务起后核对「建 Hub → 同步模型 → 探测 → 状态/告警 → 评估」核心闭环各一项真实动作。

## B. 内网部署流水线(骨架,变量化)

**触发纪律:本节只在用户明确指令「发布/部署」时执行,绝不主动运行。**

> **目标环境常量不入库**(仓库为 PUBLIC,内网主机/用户/路径不可泄露)。真实值维护在本地 gitignored 的 `.claude/skills/deploy/SKILL.md`(见 `.gitignore` 第 30 行);本节用变量名引用。执行前从该本地文件读入:`DEPLOY_USER_HOST`(user@host)、`DEPLOY_HOST`、`CONTAINER_NAME`、`CONTAINER_PORT`、`DATA_DIR`、`ENV_FILE`、`HEALTH_URL`。

### 目标环境(变量,真实值见本地覆盖文件)

- `DEPLOY_USER_HOST` / `DEPLOY_HOST` — SSH 免密目标机(Ubuntu x86_64,Docker 已装)
- `CONTAINER_NAME` — 容器名(惯例 `hubscope`)
- `CONTAINER_PORT` — 端口绑定(`<host>:8080 -> 8080`)
- `DATA_DIR` — 目标机数据目录(bind mount 到容器 `/data`)
- `ENV_FILE` — 目标机 env 文件(0600,用户手动维护;**首个 super_admin 不经 env,走 `hubscope admin create` CLI**(ADR 0011,ticket 69);env 只放 `DATA_PATH`/`ADDR`/`LOG_LEVEL`/`TRUST_PROXY` 等运行时变量,不含口令)
- `HEALTH_URL` — 健康检查地址
- 目标机访问不了 docker.io,**禁止**在目标机 `docker build`;镜像在 Mac 交叉编译后经 `docker import` 生成。

### 流水线(按序执行,任一步失败即中止;中止后 /tmp 拋留物下次发布自动覆盖,无需清理)

1. **门禁**:`make test`;`git status --porcelain` 必须为空,否则中止并报告脏文件。
2. **版本号(提议 + 确认,先不打 tag)**:有既有 tag → 提议 patch+1;无任何 tag → 提议 `v0.1.0`。**必须用 AskUserQuestion 让用户确认版本号**,记为 `VERSION`。tag 在构建成功后(步骤 3 末尾)才打,避免构建失败留下悬空 tag。
3. **交叉编译 + 打 tag**:`cd web && pnpm install && pnpm build && cd ..`;`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/hubscope-linux ./cmd/hubscope`;`git tag -a $VERSION -m "release $VERSION"`。tag 只打本地,不主动 push。镜像标签 `hubscope:$VERSION`。
4. **env 文件校验**(不读内容,只验存在与权限 0600):失败 → 中止并提示用户在目标机手动创建 `install -m 600 /dev/null $ENV_FILE` 后写入运行时变量(`DATA_PATH`/`ADDR`/`LOG_LEVEL`/`TRUST_PROXY` 等);首个 super_admin 另用 `hubscope admin create` 引导创建(不读 env、不写文件)。敏感值永不经过 Mac 或本会话。
5. **传输 + 打镜像**(此步不动 latest):scp 二进制到目标机 `/tmp`;tar 里必须包含 CA 证书(`/etc/ssl/certs/ca-certificates.crt`,HubScope v0.2.2+ 二进制已内置 `x509.SetFallbackRoots` 兜底,显式携带 CA bundle 是更稳妥兜底);`docker import` 生成 `hubscope:$VERSION`。**不在此步 tag `hubscope:latest`**——latest 永远指向「最后已知健康」的版本,步骤 8 健康检查通过后才更新。
6. **记录回滚物 + 备份数据库**(首装自动跳过):记录 `PREV_IMAGE=镜像 ID`(sha256,不可变,不受 retag 影响);`cp $DATA_DIR/app.db $DATA_DIR/app.db.bak-时间戳`,保留最近 5 份。`PREV_IMAGE=none` 表示首装。记录镜像 ID 而非镜像名——名字会被 retag 漂移,ID 不会。
7. **重建容器**:`docker rm -f $CONTAINER_NAME`;`docker run -d --name $CONTAINER_NAME -p $CONTAINER_PORT -v $DATA_DIR:/data --env-file $ENV_FILE --restart unless-stopped hubscope:$VERSION`。
8. **健康检查 → 通过则更新 latest,失败则自动回滚**:`curl` 30 次 `$HEALTH_URL`;通过则 `docker tag hubscope:$VERSION hubscope:latest` 并完成(同一条命令内完成 latest 重指向,防呆,不靠散文衔接);失败且有 PREV_IMAGE(≠ none)→ 自动回滚到 PREV_IMAGE 镜像并重跑健康检查;回滚也失败 → 输出 `docker logs --tail 50 $CONTAINER_NAME`,中止并报告现场(日志 + `app.db.bak-*` 备份位置),不再做任何操作;失败且无 PREV_IMAGE(首装)→ 保留失败容器,输出日志,中止报告,不进入步骤 9。数据库不做降级(schema 只加不删;真不兼容用 `app.db.bak-*` 人工恢复)。
9. **报告**:部署版本(`VERSION`)、镜像 ID、健康检查结果、是否回滚;提示按 A 节第 6 条做发布后核心闭环抽查;提醒旧镜像 tag 只增不减,可定期手动清理。

### 明确不做

- 不在目标机 `docker build`(docker.io 不可达)。
- 不读取、传输、打印 `$ENV_FILE` 的内容。
- 不做数据库降级。
- 不 `git push`(tag 也不 push,除非用户明确要求)。
