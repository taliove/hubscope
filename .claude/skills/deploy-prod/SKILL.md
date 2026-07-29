---
name: deploy-prod
description: 生产线部署流程(腾讯云 CentOS 7 + Docker + Caddy 自动 HTTPS)。入口是 deploy.sh(init/tag/rollback/status)+ scripts/ops 运维脚本;连接参数走 .env.prod(gitignore),公开仓库零敏感值。
trigger: user-invocable
---

# 生产线部署流程

> 依据 spec `docs/specs/0008-production-deployment.md`。**本 skill 的职责是「何时用哪个命令、为什么、有哪些坑」,不是命令全集**——命令都在脚本里,照抄命令序列部署是反模式。

## 架构与边界

- **链路:** 公网 → Caddy(443/80,systemd 原生,Let's Encrypt 自动 HTTPS;站点块含 `encode zstd gzip`——压缩固定在边缘层,SPA 静态资源与 /api JSON 全覆盖,Go 二进制不做压缩)→ `127.0.0.1:8080` → Docker 容器 `hubscope` → bind mount 宿主机数据目录。
- **连接参数:** 全部在仓库根目录 `.env.prod`(gitignore,模板 `.env.prod.example`):`PROD_HOST` / `PROD_SSH_PORT` / `PROD_SSH_USER` / `PROD_DOMAIN` / `PROD_DATA_DIR` 等。**本文件与任何提交物中不得出现实值**(三级分离纪律,spec 0008 决策 8)。
- **数据安全三道防线(核心纪律):** ① 数据只走宿主机 bind mount,容器生死与数据无关;② 每次部署前强制备份(热备优先);③ 恢复流程实机演练过(2026-07-27 首次部署已验证三条全部通过)。

## 命令入口

全部操作从仓库根目录出发:

```bash
DS=.claude/skills/deploy-prod/scripts/deploy.sh

$DS init          # 全新机器幂等初始化(也可当环境体检重复跑)
$DS tag vX.Y.Z    # 部署一个 git tag(生产唯一部署模式,无 dev)
$DS rollback      # 回滚到上一镜像(读备份目录的 rollback 记录)
$DS status        # 生产状态一览(容器/数据/备份/Caddy/证书到期日)
```

服务器侧标准化运维脚本(deploy.sh 自动同步到 `$PROD_OPS_DIR`,测试线同套):

```bash
hubscope-admin-create   # 交互建超管/用户(read -s,密码不落任何持久记录,W6)
hubscope-backup         # sqlite3 .backup 热备 + integrity_check + 保留策略
hubscope-restore        # 从备份恢复(确认提示 → 校验 → 停-换-修权限-起-检查)
hubscope-status         # 状态一览(同 $DS status 的本机版)
```

## 何时用哪个

| 场景 | 命令 |
|---|---|
| 新机器第一次部署 | `$DS init` → `$DS tag vX.Y.Z` |
| 日常升级 | 打 tag → `$DS tag vX.Y.Z`(自动备份/构建/传输/换容器/双健康检查) |
| 升级后健康检查失败 | `$DS rollback`;怀疑数据损坏再 `hubscope-restore` |
| 建/加管理账号 | ssh 到生产机跑 `hubscope-admin-create` |
| 手动备份/巡检 | `$DS status` 或生产机上 `hubscope-backup` / `hubscope-status` |
| Caddy/证书问题 | `journalctl -u caddy --since "-10min"`(证书 ~15s 申请,续期全自动,数据在 `/var/lib/caddy` 勿删) |

## 关键经验教训(2026-07-27 首次生产部署,已脱敏)

1. **uid 10001 权限是第一杀手:** 每次换容器**之前**必做 chown(deploy.sh 已内置),否则 `unable to open database file`。
2. **`--version` 陷阱:** v0.2.3 及更早的二进制没有 `--version` flag,跑它会**正常启动服务器并抢占 8080**——两条线各踩过一次。老版本溯源只能靠镜像 tag;v0.2.4+ 才能用 `--version` 验证(deploy.sh 已按版本号自动分流)。
3. **一台机器只选一种部署模型:** 原生 install.sh 部署与 Docker 部署共存会争端口、双写数据。首次部署时发现旧原生残留,确认无真实数据后归档旧库、清除痕迹。`$DS init` 的冲突检查(端口占用/原生残留→报警停下,不自动清除)即由此教训而来。
4. **国内环境实测:** Docker CE 走阿里云 yum 源(CentOS 7 装到 26.1.4 无兼容问题);Caddy 二进制生产机直连下载极慢,**开发机缓存后 scp**(deploy.sh init 内置);生产机 Docker **零外网拉镜像**,永远 `docker save` 传输。
5. **SSH 会话偶发掉线(curl exit 56):** 对生产机新绑定端口立刻 curl 时偶发。**重连即可,远端操作实际已生效**——掉线后先重连核对状态,不要盲目重跑(deploy.sh 各命令幂等,重跑安全)。
6. **防火墙分层:** 宿主防火墙状态以 `$DS init` 实测为准(firewalld 运行则自动放行 http/https);云平台安全组在脚本触及范围外,公网不通而本地通时先查安全组。
7. **备份姿势:** CentOS 7 自带 sqlite3 支持 `.backup` 热备,秒级完成,无需停服(`hubscope-backup` 默认姿势,无 sqlite3 时回落停服 cp)。
8. **容器加固约定(spec 0015 决策 8):** 所有 `docker run`(部署与回滚两处)一律带 `--log-opt max-size=50m --log-opt max-file=3`(json-file 日志轮转,防磁盘慢性风险)与 `--memory 1g`(容器异常不拖垮同机 Caddy);新加 `docker run` 路径时必须保持一致。

## 与测试线(deploy-test-101)差异登记

| 维度 | 生产线(本 skill) | 测试线 |
|---|---|---|
| 端口绑定 | `127.0.0.1`(Caddy 终结公网) | LAN IP 直暴 |
| TLS | Caddy 自动 HTTPS | 裸 HTTP |
| 重启策略 | `--restart unless-stopped` | 无 |
| 镜像来源 | 开发机构建 + docker save 传输 | 本机直接构建 |
| 部署执行 | 开发机 ssh 驱动 | 本机执行 |
| 部署模式 | 只有 tag | dev + tag |
| 配置 | `.env.prod` | `.env.local` |
