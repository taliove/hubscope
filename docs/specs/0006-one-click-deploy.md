# 0006 — 一键部署脚本与分发包

## 背景

HubScope 是**部署型产品**:读者是"要在自己服务器上部署一套 AI Hub 监控系统的人",不是 import 一个库的开发者。交付物是 Go 单二进制(前端 embed、SQLite 内嵌、无运行时 node 依赖,W8)。

但当前**获取路径只有一条:git clone + 完整工具链(Go + pnpm)+ make build**。`git tag` 为空、无 release、无 `scripts/`、README 无「下载与部署」章节(readme-writer 三问验收,2026-07-24)。部署型产品没有使用者获取路径,等于产品不可达。

本 spec 补齐使用者路径:**一键部署脚本(scripts/install.sh)+ 完善 Docker 路径(docker-compose 一键起)**,让目标读者两台命令内跑起一套可用的 HubScope。预编译二进制 release 流程(GitHub release / tag)在本 spec 之外(见「Out of Scope」),install.sh 的形态按「源码构建落部署」设计,未来有了 release 后脚本可平滑切换为「下载二进制落部署」。

## 决策总览

1. **`scripts/install.sh` 一键部署(核心交付物)**:面向 Linux 服务器(root 或可 sudo 用户)的一条命令,完成:环境检查(Go/pnpm 或提示)→ 构建二进制 → 安装到 `/usr/local/bin/hubscope` → 建数据目录 `/var/lib/hubscope` → 落 systemd unit → 启动并等待健康检查通过 → 打印下一步引导(`admin create` 命令 + 访问地址)。**幂等**:重复执行 = 升级到当前源码版本(重新构建、替换二进制、重启服务),不破坏既有数据与配置。
2. **systemd 与 docs/deployment.md 同构**:unit 内容复用 deployment.md 已验证的形态(User/Group、WorkingDirectory、DATA_PATH=/var/lib/hubscope/app.db、Restart=on-failure),修正其中的旧名残留(`ahc` 用户名、`ahc-data` volume 名 → `hubscope`),deployment.md 同步更新,**两处只有一份事实**:脚本内嵌 unit 模板为唯一来源,deployment.md 描述脚本行为而非复制 unit 全文。
3. **Docker 路径纳入**:根目录已有 Dockerfile。补 `docker-compose.yml` 一键起(build + volume `/var/lib/hubscope` 或 named volume `hubscope-data` + 端口映射 8080 + 首次启动提示 admin create);修正 deployment.md 的 `ahc-data` volume 旧名。Docker 路径与脚本路径并列,读者二选一。
4. **README 补「下载与部署」章节**(吸收 readme-writer 三问验收的 HIGH 发现):按使用者路径排序——一键脚本 → Docker → 从源码构建(标注贡献者路径),并同步修正过期事实(5 能力 Suite 词表、`make hooks`、数据落点、配置表指向)。README 修改与脚本同 spec 交付,因为脚本的存在性是 README 写法的前提。
5. **tar.gz 包定位不变**:`make package` 保持现状(二进制 + Dockerfile + 部署文档),install.sh 进包,不新增 .deb/.rpm/Homebrew。
6. **不引入新承重墙**:本 spec 使用 W8(单二进制交付)而非修改它;install.sh 是 W8 交付模型的延伸,不改变 embed/单文件/无 node 依赖的任何约定。

## 数据模型变更

无。

## 承重墙影响分析(W8,四问)

**为什么必须碰 W8 周边(不改 W8 本身,新增其延伸)?** 不改的代价:产品只有贡献者路径可达,部署者读者(目标用户)无法获取;README 三问验收「怎么下载部署」❌ 永久失格。

**影响哪些调用方?**
- 直接:README.md(新增下载节)、docs/deployment.md(与脚本对齐 + 旧名修正)、Makefile(install.sh 纳入 package 内容)
- 间接:`cmd/hubscope`(install.sh 依赖其 CLI:admin create、--help;不改代码)、W6(admin create 引导文案出现在脚本输出中,凭证边界不变)

**有没有替代方案?** ① 只做文档不改脚本(部署者仍需手工 10+ 步,门槛不消除);② 直接做 GitHub release 二进制(需要 tag/CI 流程,当前无,工作量大且 install.sh 未来仍需要——只是从"构建"换成"下载")。选定:先脚本后 release,脚本形态对两者兼容。

**改完回归测试什么?** 三层:当前功能层(新增 install 脚本测试,见「测试决策」)+ 关联功能层(`cmd/hubscope` 既有测试回退,脚本不触代码但依赖 CLI 行为)+ 核心业务闭环层(`make test` 全量,pre-commit 门禁强制)。

## 测试决策

好测试的标准(W1 唯一测试接缝 + 本 spec 特殊性):脚本测试属**交付物行为测试**,不走 HTTP API 接缝(W1 管后端行为),但遵守同一精神——**测外部可观察行为,不测脚本内部实现**。

1. **`scripts/install.sh` 的测试(新增,测试层 1)**:
   - **静态检查**:CI 门禁加 `shellcheck scripts/install.sh`(进 `make lint` 或 `make test` 的静态检查段)。
   - **幂等与行为测试**:`scripts/install_test.sh`(或 Go 黑盒 `testscript` 风格)在临时目录 + 假 PREFIX(`HUBSCOPE_PREFIX`/`HUBSCOPE_DATA_DIR` env 覆盖,避免测试碰 /usr/local)+ stub systemctl(`PATH` 前置假 systemctl 记录调用)下验证:
     - 首次执行:二进制落到 PREFIX、数据目录创建、unit 文件生成、引导输出包含 admin create 提示
     - 重复执行:成功(幂等)、数据目录内容未被清空、unit 被重写
     - 无 Go 环境:报错信息明确指出缺什么,不半截安装
   - **Docker 路径**:`docker compose config` 合法性检查进 CI(不实际起容器);compose 文件内容关键断言(port 8080、volume、build context)走脚本化 grep 测试。
2. **关联功能层**:`go test ./cmd/...`(admin create CLI 是脚本引导的一部分,行为不回归)。
3. **核心业务闭环层**:`make test` 全量(门禁强制)。

**Prior art**:仓库现有测试全在 `internal/server/*_test.go`(HTTP 接缝)与 `cmd/hubscope/admin_test.go`(CLI 函数级);脚本测试无先例,采用「临时目录 + env 覆盖 PREFIX + stub systemctl」的最低侵入方案,不引入 bats 等新测试依赖(shell + Go test 足够)。

## User Stories

1. As a 部署者(有 Linux 服务器),I want 一条命令把 HubScope 装成 systemd 服务,so that 不用读部署文档抄 10 步手工操作。
2. As a 部署者,I want 脚本告诉我装完下一步干什么(建管理员、访问哪个地址),so that 装完不卡壳。
3. As a 部署者,I want 重复跑脚本是安全的(升级语义),so that 升级不用背另一套流程。
4. As a 部署者,I want 缺 Go/pnpm 时脚本明说缺什么,so that 不是装到一半莫名失败。
5. As a 部署者,I want 数据放在固定位置(/var/lib/hubscope)与我跑脚本的目录无关,so that 换个目录启动不会"数据消失"。
6. As a 偏好容器的部署者,I want `docker compose up` 一条命令起服务,so that 不碰宿主机环境。
7. As a Docker 部署者,I want 容器重建后数据还在(named volume),so that 升级镜像不丢监控历史。
8. As a Docker 部署者,I want 首次启动后知道怎么建管理员(容器内执行 admin create 的命令提示),so that 完成初始化。
9. As a 项目新读者,I want README 有一节告诉我怎么下载部署(脚本/Docker/源码三路径),so that 30 秒选定我的路径。
10. As a 项目新读者,I want README 的功能描述与代码现状一致(5 能力 Suite),so that 不被旧词表误导。
11. As a 维护者,I want systemd unit 只有一份事实来源(脚本内嵌模板),so that deployment.md 与脚本永不漂移。
12. As a 维护者,I want 旧项目名残留(ahc/ahc-data)从部署路径清除,so that 新人不困惑。
13. As a 维护者,I want install.sh 有 shellcheck + 幂等测试,so that 脚本改坏会被门禁拦住。

## Implementation Decisions

- **`scripts/install.sh`**(新,POSIX sh 或 bash,set -euo pipefail):
  - 可配置点全部走 env 覆盖,默认值即生产约定:`HUBSCOPE_PREFIX`(默认 /usr/local)、`HUBSCOPE_DATA_DIR`(默认 /var/lib/hubscope)、`HUBSCOPE_USER`(默认 hubscope,不存在则创建系统用户)、`HUBSCOPE_PORT`(默认 8080,写入 unit 的 Environment=ADDR)、`HUBSCOPE_SYSTEMD_DIR`(默认 /etc/systemd/system——unit 写入路径可重定向,是测试不碰真实 systemd 的前提,实现期补录)。
  - 步骤:依赖检查(go、pnpm 缺失即明确报错退出)→ `make build` → install 二进制 → 建用户/数据目录并 chown → 渲染 systemd unit(内嵌 heredoc 模板,唯一事实来源)→ systemctl daemon-reload && enable --now → 轮询 `http://localhost:$PORT/api/overview` 直至 200(超时 30s 报日志提示)→ 打印引导(绿色对勾风格,含 `hubscope admin create` 示例与访问地址)。
  - 幂等:每步先查后做;unit 每次重写(内容即事实);数据目录只建不碰内容。
- **systemd unit 模板**:Type=simple、User/Group=hubscope、Environment=DATA_PATH/ADDR、WorkingDirectory=DATA_DIR、Restart=on-failure、NoNewPrivate/ProtectSystem 基本加固(与 deployment.md 既有建议对齐,如有出入以更严者为准并在 deployment.md 更新)。
- **`docker-compose.yml`**(新,仓库根):service 单条——build: .、ports 8080:8080、volume named `hubscope-data` → /var/lib/hubscope、restart unless-stopped;README 与 deployment.md 注明首次 `docker compose exec hubscope hubscope admin create ...` 建管理员。
- **docs/deployment.md 更新**:改为「推荐路径 = scripts/install.sh(自动完成本节全部步骤)」,手工步骤保留为参考;`ahc`/`ahc-data` 旧名 → `hubscope`/`hubscope-data`;Docker 节改 compose 优先。
- **README.md 更新**(同批):新增「下载与部署」节(脚本 → Docker → 源码构建);修正 5 能力 Suite 词表;Development 节补 `make hooks`;快速上手补数据落点与 :8080 默认;配置表一行指向 deployment.md;bcrypt 去掉 cost 10 实现细节。
- **Makefile**:`package` 目标产物纳入 `scripts/install.sh`;`lint`/`test` 纳入 shellcheck(若环境无 shellcheck 则降级为警告不卡门禁——门禁 fail-closed 原则与开发者机器无 shellcheck 的现实折中:CI 必查,本地缺工具告警)。

## Out of Scope

- **GitHub release / tag / 预编译二进制分发**:需要 CI 与发布决策,另起 ticket;install.sh 已按可切换设计。
- .deb/.rpm/Homebrew/Snap 等系统包。
- Windows/macOS 安装脚本(对内产品,部署目标是 Linux;macOS 开发机走 `make dev`)。
- nginx/HTTPS/域名配置(deployment.md 既有内容,不在本 spec 变更)。
- 多实例/高可用部署。
- install.sh 的卸载子命令(可后续补,首版保持最小)。

## Further Notes

- 本 spec 起源于 readme-writer agent 的三问验收(2026-07-24):「怎么下载部署」❌ 的根因不在 README,在交付形态缺口。
- install.sh 未来切换 release 二进制时,只需把「make build」段换成「curl + 校验和」,其余(用户/目录/unit/引导)不变。
- 关联文档:W8(单二进制)、W6(admin create 引导)、ADR 0011(CLI 引导)、docs/deployment.md。
