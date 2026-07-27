# 0008 — 生产环境部署(Docker + Caddy 自动 HTTPS)

> **脱敏说明(2026-07-27 第二轮修订):** 本仓库为公开仓库,规格中的生产机连接参数(IP、SSH 端口、SSH 用户)一律以 `<PROD_HOST>`、`<PROD_SSH_PORT>`、`<PROD_SSH_USER>` 占位符表示,实值存于开发机 `.env.prod`(gitignore);防御姿态类信息(防火墙现状等)不进公开文档。域名属公开信息(DNS 与证书透明日志本就公开),予以保留。

## 背景

HubScope 目前只有测试线部署(192.168.1.101,见 deploy-test-101 skill)。本 spec 定义**首次生产部署**:将稳定版本 **v0.2.3** 部署到生产服务器(腾讯云 CentOS 7,裸机:无 Docker、无 Caddy,连接参数见 `.env.prod`),通过域名 `https://ai-claude-code-hub-static.jetmobo.com/` 对外服务,Caddy 反向代理到容器 8080 端口并自动管理 Let's Encrypt 证书。

**本 spec 的核心约束是数据安全性(用户明确提出):** 数据必须与容器生命周期彻底解耦——`docker rm`、镜像升级、容器崩溃均不得触碰监控数据;备份与恢复机制从第一天就存在,且恢复流程经过实际演练,不是纸面流程。SQLite 是单份数据(W2),丢了不可重建。

**第二轮修订新增核心约束(用户评审提出):**
1. **部署能力必须脚本化**,与测试线 deploy.sh 同构,不靠 SKILL 文档里的命令序列人肉照抄;
2. **配置与代码分离**,环境实值走 `.env.prod`(gitignore)+ `.env.prod.example`(占位符,提交),沿用 `.env.local` 既有惯例;
3. **服务器侧要有标准化运维脚本**(建超管、备份、恢复、状态),两条线同一套;
4. **公开仓库零敏感值**——SKILL/spec/脚本中不得出现生产连接参数与防御姿态;
5. **全新机器初始化能力**——裸机前置工作(装 Docker/Caddy、建用户目录、systemd、防火墙检查、同步运维脚本)固化为幂等的 `deploy.sh init`,换机不靠记忆。

## 决策总览

1. **部署形态:Docker 容器 + 宿主机 Caddy(systemd)反向代理。** 与测试线同构,复用既有 Dockerfile 与运维经验;Caddy 不进容器——证书数据落宿主机 `/var/lib/caddy`,systemd 管理,与容器生命周期完全无关。
2. **数据安全三道防线(本 spec 的承重决策):**
   - **防线 1 — 绑定挂载宿主机目录,禁止匿名卷:** 容器数据目录只挂 `/data/hubscope → /data`(bind mount)。`docker stop/rm`、镜像重建、Docker 升级均不影响宿主机目录;即使整个 Docker 被卸载,数据仍在。
   - **防线 2 — 每次部署前强制备份:** 备份目录 `/data/hubscope-backups/`。备份方式优先 `sqlite3 app.db ".backup ..."`(在线热备,WAL 安全,不锁库不停服;宿主机自带 sqlite3 已验证可用);无 sqlite3 时回落「停容器 → cp → 起容器」。保留最近 ≥3 份备份。首次部署也先建好备份目录并演练一遍流程。
   - **防线 3 — 恢复流程实机演练:** 首次部署完成后,用备份文件在 scratch 目录起第二个临时容器(不同端口)验证备份可恢复、数据完整,然后销毁临时容器。恢复没演练过 = 没有备份。
3. **镜像策略:本地构建 + `docker save` 传输,不在生产机拉镜像构建。** 生产机在国内、CentOS 7、无镜像加速器;Dockerfile 构建需拉大镜像,生产机构建既慢又引入不确定性。流程:开发机 `git archive <tag>` 干净构建 → `docker save | gzip` → scp → 生产机 `docker load`。生产机 Docker 零外网拉取(运行镜像已包含在 save 产物内)。
4. **国内环境软件源(实测结论):**
   - Docker CE:阿里云 yum 镜像,CentOS 7 实测装到 26.1.4 无兼容问题。
   - Caddy:官方预编译单二进制;**生产机直连下载极慢(实测),由开发机下载缓存后 scp 传输**。
   - 不配置 Docker 镜像加速器(生产机零拉取,不需要)。
5. **Caddy 配置:`/etc/caddy/Caddyfile` + systemd 服务。** 内容仅一个站点块:域名 → `reverse_proxy 127.0.0.1:8080`,自动 HTTPS(HTTP→HTTPS 跳转、证书申请与续期全自动)。容器只绑 `127.0.0.1:8080`(不暴露公网,443/80 由 Caddy 独占)。证书数据 `/var/lib/caddy` 由 systemd unit 默认守护,重装/重启 Caddy 不丢证书(避免 Let's Encrypt 重复签发触发速率限制)。
6. **容器运行参数:** `--name hubscope --restart unless-stopped -p 127.0.0.1:8080:8080 -v /data/hubscope:/data`。启动前 `chown -R 10001:10001 /data/hubscope`(第一杀手教训:容器 uid 10001,权限不对直接起不来)。
7. **前置条件(已确认):** DNS A 记录已指向生产机;云安全组已放行 80/443。部署时仍需检查宿主机防火墙状态(云组放行 ≠ 宿主放行,宿主防火墙现状不进公开文档,以部署时实测为准)。
8. **敏感信息三级分离(第二轮新增):**
   - **公开仓库**:`.env.prod.example`(全占位符)+ 参数化脚本(零硬编码敏感值)+ 脱敏的 SKILL/spec(变量引用与泛化描述);
   - **`.env.prod`(gitignore,仅开发机)**:实值——`PROD_HOST` / `PROD_SSH_PORT` / `PROD_SSH_USER` / `PROD_DOMAIN` / `PROD_DATA_DIR` / `PROD_DOCKER_YUM_MIRROR`(可覆盖)等;
   - **执行档案**(`.scratch/prod-deploy/`,含实值)加入 .gitignore,不进仓库。
   - 附带修复:测试线 deploy.sh 硬编码默认超管密码改为 `read -s` 交互输入(公开仓库中的默认密码 = 任何人可登测试线管理台)。
9. **部署能力脚本化(第二轮新增):** `deploy-prod/scripts/deploy.sh`,在开发机执行、ssh 驱动远程,命令词汇与测试线对齐并扩展:
   - `init` — 全新机器幂等初始化:探测 OS(不支持明确报错)→ 装 Docker CE(yum 源可经 env 覆盖)→ Caddy 二进制开发机缓存 + scp → caddy 用户/目录/官方 systemd unit → `/data` 目录 + 10001 权限 → **防火墙与 8080 占用检查(发现冲突报警停下,不自动清除——清别人东西前要人看)** → 同步 `scripts/ops/` 到 `/opt/hubscope/bin/` → 占位 Caddyfile + 启动。重复执行 = 幂等校验,可当「环境体检」。
   - `tag <版本>` — 标签部署:备份(热备)→ git archive 干净构建 → save/scp/load → 权限修复 → 换容器 → 本地+公网健康检查 → fresh DB 时提示交互建号。
   - `rollback` — 回滚:最近 rollback 记录 → 停新起旧 → 健康检查。
   - `status` — 状态一览(容器/镜像/数据盘/Caddy/证书到期日)。
   - 生产无 `dev` 模式(生产只走 tag)。
10. **服务器侧标准化运维脚本(第二轮新增):** 仓库 `scripts/ops/`(零敏感值、全参数化,公开仓库可放),由 deploy.sh 同步到服务器 `/opt/hubscope/bin/`,**测试线与生产线同一套**:
    - `hubscope-admin-create` — 交互式建超管/用户,`read -s` 读密码,不进历史/命令行/日志(W6);
    - `hubscope-backup` — `sqlite3 .backup` 热备 + integrity_check + 保留最近 N 份;
    - `hubscope-restore` — 停容器 → 恢复备份 → 权限修复 → 起容器 → 健康检查;
    - `hubscope-status` — 容器/镜像/数据盘/Caddy/证书到期日一览。
11. **fresh DB 建号策略(第二轮新增):** deploy.sh 检测到全新库 → 提示「现在创建 super_admin? [Y/n]」→ Y 则经 ssh `-t` 调 `hubscope-admin-create`(read -s);N 则跳过并提醒。不自动建默认密码账号,不留无人知道密码的账号,也不因遗忘导致管理台进不去。
12. **首次启动引导:** 首个 super_admin 经 `hubscope-admin-create` 创建(W6:凭证经 CLI 注入,bcrypt 落库,不回明文)。
13. **回滚方案:** 部署前记录当前镜像;健康检查失败 → 停新容器 → 起旧镜像容器 → 必要时恢复最近备份。回滚为 deploy.sh 一等命令,不靠临场发挥。

## 数据模型变更

无(应用 schema 迁移由 `store.Open` 自动完成,W2;本 spec 不新增表)。

## 承重墙影响分析

本 spec **不修改任何承重墙**,只消费:
- **W2(存储层):** 备份/恢复流程尊重 SQLite 单文件 + WAL;热备用 `.backup` 命令是 SQLite 官方安全姿势,不直接 cp 运行中的 db 文件。
- **W6(凭证边界):** admin 创建走容器内 CLI + `read -s` 交互,密码不进历史/命令行/日志/环境变量文件;公开仓库零默认密码(测试线硬编码密码一并修除);Caddy 反代不引入任何凭证明文日志。
- **W8(单二进制):** 镜像构建路径与 W8 一致(前端 embed,单容器交付)。

## 测试决策(验收接缝)

本 spec 是运维交付,应用层不新增自动化测试;验收走**最高层黑盒接缝——公网入口**,与 W1「测外部可观察行为」精神一致:

1. **接缝 1 — 公网验收(基础验证):**
   - `https://ai-claude-code-hub-static.jetmobo.com/healthz` 返回 200
   - 首页返回前端 HTML(含 hash 资源名)
   - TLS 证书签发者为 Let's Encrypt、域名匹配、无警告
   - HTTP→HTTPS 跳转生效
2. **接缝 2 — 备份/恢复演练(数据安全验收):**
   - 备份文件生成且非空、`PRAGMA integrity_check` 通过
   - 用备份在 scratch 目录 + 临时端口起第二个容器,`/healthz` 200;演练完销毁
3. **接缝 3 — 数据解耦验证(防 Docker 干掉数据):**
   - `docker stop && docker rm` 后数据文件仍在且大小不变;重建同名容器后数据无损
4. **接缝 4 — 脚本质量(第二轮新增):**
   - `scripts/ops/*.sh` 与 deploy.sh 全部过 shellcheck(纳入 `make lint` 的脚本检查清单,fail-closed)
   - `deploy.sh init` 幂等性实机验证:在已就绪的生产机上重跑,全部组件报告「已就绪/跳过」,不产生变更
   - `.env.prod.example` 与 deploy.sh 实际读取的变量清单一一对应(防示例文件腐化)
5. **接缝 5 — 泄露自查(第二轮新增):**
   - 提交前 grep 待提交文件,无生产连接参数实值(IP/SSH 端口)、无默认密码、无防御姿态描述

**Prior art:** deploy-test-101 skill 的部署/回滚流程(本 spec 是其生产线变体);`scripts/install_test.sh` 的「测行为不测实现」精神;`.env.local`/`.env.local.example` 惯例(本 spec 扩展为 prod 线)。

## User Stories

1. As a 服务消费者(状态板读者),I want 通过 `https://ai-claude-code-hub-static.jetmobo.com/` 访问状态板,so that 不用记 IP 和端口,且浏览器不弹证书警告。
2. As a 服务消费者,I want HTTP 自动跳转 HTTPS,so that 输错协议也能到正确的站。
3. As a 运维者,I want 应用数据在宿主机固定目录且与容器生命周期无关,so that `docker rm`、升级镜像、Docker 崩溃都不会丢监控历史。
4. As a 运维者,I want 每次部署前自动备份数据库,so that 任何一次部署出问题都能回到部署前状态。
5. As a 运维者,I want 备份是热备(不停服)且有保守回落(停服 cp),so that 备份本身不造成可用性损失,也不因工具缺失而跳过。
6. As a 运维者,I want 恢复流程实机演练过,so that 真出事故时恢复不是靠祈祷。
7. As a 运维者,I want 容器崩溃/机器重启后服务自动拉起,so that 半夜挂了不用人肉救。
8. As a 运维者,I want 容器只监听 127.0.0.1、公网只暴露 Caddy 的 80/443,so that 攻击面最小。
9. As a 运维者,I want 证书申请与续期全自动,so that 不会某天证书过期全站告警。
10. As a 运维者,I want Caddy 证书数据落宿主机持久目录,so that 重装/重启 Caddy 不重复申请证书触发速率限制。
11. As a 运维者,I want `deploy.sh tag vX.Y.Z` 一条命令完成升级(备份/构建/传输/换容器/健康检查全自动),so that 部署不靠记忆、不拼手速。
12. As a 运维者,I want `deploy.sh init` 一条命令把全新机器初始化到可部署状态且幂等,so that 换机/重建不靠文档照抄,还能当环境体检验。
13. As a 运维者,I want init 发现端口冲突或旧部署残留时报警停下,so that 不会在没看清状况时清掉别人的东西。
14. As a 运维者,I want 服务器上有标准化运维命令(建超管/备份/恢复/状态),so that 日常运维不拼一次性 ssh 命令,两条线行为一致。
15. As a 运维者,I want 全新部署时脚本提示我交互建超管(read -s),so that 不留默认密码账号,也不会忘了建号进不了管理台。
16. As a 运维者,I want 回滚是 deploy.sh 的一等命令,so that 部署失败的最坏结果是回到旧版本而不是裸奔。
17. As a 项目维护者,I want 环境实值(IP/端口/账号)只在 gitignore 的 .env.prod 里,公开仓库只有占位符示例,so that 开源不暴露生产攻击面。
18. As a 项目维护者,I want 镜像在开发机构建、docker save 传输到生产,so that 生产机零外网拉镜像、构建环境单一可控。
19. As a 项目维护者,I want 所有部署/运维脚本过 shellcheck 门禁,so that 脚本质量与应用代码同标准。

## Out of Scope

- **CI/CD 自动化发布**(GitHub Actions 构建/推送镜像、tag 触发部署)——部署由开发机手动触发,流程固化在 deploy.sh;自动化留待后续 spec。
- **生产机系统级监控/告警**(宿主机 CPU/磁盘监控、日志聚合、uptime 监控)。
- **多机/高可用/负载均衡**——单机单容器是本产品的部署模型(W8)。
- **CDN、WAF、防盗链**等域名前置层。
- **备份异地容灾**(备份与数据同机;异地同步留待后续)。
- **非 CentOS 7 发行版的 init 适配**——init 探测到不支持的发行版明确报错,适配留待实际需求出现。

## 进一步说明

- **证书获取时序:** Caddy 首次启动时在线申请证书,Let's Encrypt 走 HTTP-01/TLS-ALPN-01 挑战,需要 80/443 已放行且 DNS 已生效。首次访问前给 Caddy ~30s 申请窗口;失败看 `journalctl -u caddy`。(2026-07-27 实测:首次申请 ~15s 一次通过。)
- **版本溯源注意:** v0.2.3 及更早的 tag 不含版本注入机制(`--version` flag、UI 版本显示、Dockerfile VERSION build-arg 均为 tag 之后的提交引入);在老版本二进制上跑 `hubscope --version` 不会打印版本,而是正常启动服务器抢占 8080——两条线各踩过一次。版本溯源对老版本只能靠「git archive 干净构建 + 镜像 tag」,v0.2.4+ 起可用 `--version` 验证。
- **一台机器只选一种部署模型:** 首次部署时发现生产机存在先前原生部署(systemd + 裸二进制),与容器争 8080 导致 service 崩溃循环;确认无真实数据后归档旧库、清除原生痕迹。init 的冲突检查即由此教训而来。
- **与测试线的差异登记:** 生产线 = 绑 127.0.0.1(测试线绑 LAN IP)、`--restart unless-stopped`(测试线无)、Caddy 终结 TLS(测试线裸 HTTP)、镜像 docker save 传输(测试线本机构建)、部署经 ssh 远程驱动(测试线本地执行)、无 dev 模式。差异全部写进 deploy-prod skill,不与 deploy-test-101 混淆。
