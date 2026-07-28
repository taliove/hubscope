# 84 — 代理头信任修复:部署模板替换 XFF + TRUST_PROXY 注入

**What to build:** 按 spec 0011 决策 2 修复代理头信任链。**生产线**(`.claude/skills/deploy-prod/scripts/deploy.sh`):① Caddy 站点块模板(`ensure_caddy_site`)的 `reverse_proxy` 内加 `header_up X-Forwarded-For {remote_host}`——Caddy 默认对 XFF 是**追加**,客户端伪造的最左跳会成为应用 `clientIP` 的权威值(代码注释已警告),必须替换;② `init` 的占位 Caddyfile 同样修正;③ 所有 `docker run` 处注入 `-e TRUST_PROXY=true`;④ **存量迁移路径**:现有 `ensure_caddy_site` 幂等逻辑对已有站点块直接跳过,模板修正不会生效——需要检测存量块缺 `header_up` 时重写站点块(或明确报错提示人工),不能静默漏过。**测试线**(`.claude/skills/deploy-test-101/scripts/deploy.sh`):先核实 192.168.1.101 前面是否有反代(docs/deployment.md 的 nginx 样例是否实际在用)——有反代则同样修正 nginx 配置(`proxy_set_header X-Forwarded-For $remote_addr;` 替换语义)并注入 `TRUST_PROXY=true`;**若容器直接绑 LAN IP 无反代,则保持 `TRUST_PROXY=false`,绝不注入**(直接暴露时信任 XFF = 客户端可任意伪造限流键与审计 IP,比不开更糟),并在脚本注释中写明这个判断。`docs/deployment.md` 环境变量表补充:TRUST_PROXY 两条线的实值与判断依据。

**Blocked by:** 无 — 可立即开工(与 85 并行)

**Status:** done(3 commit:09aa0ec → 2cc40ad → d407672;bash -n 双脚本通过;零应用代码改动;check 三维度 PASS 2026-07-28,2 项 LOW 观察登记不处理:迁移判定为文件级 grep 未限定站点块内、caddy validate 以 root 落残留存储目录)

## 执行顺序(单 commit ≤8 文件)

1. **生产线 commit**:Caddy 站点块模板 + 占位 Caddyfile 加 `header_up`;`ensure_caddy_site` 增加存量块检测与迁移;`docker run` 注入 TRUST_PROXY
2. **测试线 commit**:核实反代存在性(问用户或实测),按结论改脚本 + 注释写明判断依据
3. **文档 commit**:`docs/deployment.md` 更新;`.env.prod.example` 如需同步占位符

## 验收清单

- [x] 生产线 deploy.sh 写出的 Caddyfile 站点块含 `header_up X-Forwarded-For {remote_host}`,替换语义成立(Caddy 官方文档:header_up 无前缀 = overwriting any existing values;init 占位 Caddyfile 无 reverse_proxy,按备注裁决 2 标注为无工项)
- [x] 存量已部署站点的迁移路径存在且幂等(三态分支:无块→新模板创建;有且含 header_up→跳过;有但缺→时间戳备份 → 整文件重写 → `caddy validate` → 失败自动还原备份并 exit 1 → 通过则 reload;重复执行落入跳过态)
- [x] 生产线所有 `docker run` 路径注入 `TRUST_PROXY=true`(cmd_tag + cmd_rollback;rollback 按备注裁决 3 纳入)
- [x] 测试线按「有无反代」实测结论处理:无反代(5 条实证,2026-07-27,见备注节),`start_container` 与 `rollback` 加注释写明判断与证据日期,确认未注入 TRUST_PROXY
- [x] `docs/deployment.md` 已同步(TRUST_PROXY 行扩写两线实值与依据 + nginx 样例补 `proxy_set_header X-Forwarded-For $remote_addr;` 并写明禁用 `$proxy_add_x_forwarded_for` 的理由);`.env.prod.example` 按备注裁决 4 不动(TRUST_PROXY 走 docker run 字面量);公开仓库零敏感值(IP/端口/用户仍为占位符)
- [x] `bash -n` 两个 deploy.sh 通过;未改应用代码(本票无 `make test` 影响面)

## 风险登记

1. **改 Caddyfile 的生产风险**:本票只改模板与脚本,不触碰运行中的生产机(实机生效归 ticket 87);重写存量站点块的路径必须先备份原文件再改,失败可回滚
2. **TRUST_PROXY 误开的危害大于不开**:测试线结论必须以实测为准,不允许「大概有个 nginx」就注入
3. **spec 0008 脱敏纪律**:deploy-prod 脚本是公开仓库内容,改动不得引入生产实值

## 备注(阶段 1 影响分析裁决登记,2026-07-28)

- **事实修正(Caddy 版本行为)**:plan 经官方文档 + master 源码双证——「Caddy 默认追加 XFF」自 v2.7(2023)起不成立,现代 Caddy 默认忽略/删除客户端自带 XFF 再 Set 直连对端 IP(生产 Caddy ≥2.10,伪造洞默认已不存在)。显式 `header_up X-Forwarded-For {remote_host}` 仍正确且保留:替换语义成立、版本无关、防未来 trusted_proxies 配置漂移;注释登记未来陷阱——接 CDN 配 trusted_proxies 时应改 `{client_ip}`。spec 0011 第 14 行的过时陈述由文档 commit 一并修正(裁决 1:同意)。
- **init 占位 Caddyfile(裁决 2:确认无工项)**:现状是 `:80 { respond ... }`,无 reverse_proxy,不需要修。
- **cmd_rollback 纳入(裁决 3:同意)**:rollback 也起容器,其 docker run 同样注入 `-e TRUST_PROXY=true`(ticket 正文只写了 tag 路径,属遗漏)。
- **.env.prod.example 不动(裁决 4:确认)**:TRUST_PROXY 走 docker run 字面量注入,不进 env 文件。
- **测试线结论:无反代,绝不注入 TRUST_PROXY(5 条实证)**:容器直绑 LAN IP:8080;.101 本机无 80/443 监听;nginx 不存在;/healthz 返回 HubScope 自身安全头无反代附加头;docs nginx 样例 upstream 为 127.0.0.1:8080 与测线绑定方式矛盾(纯通用示例)。deploy-test-101 只加注释写明判断与证据日期,零行为变更。
- **docs/deployment.md nginx 样例(裁决 5:同意)**:样例现状根本没设 X-Forwarded-For,补 `proxy_set_header X-Forwarded-For $remote_addr;`——不能用 `$proxy_add_x_forwarded_for`(追加语义,可伪造)。
- **存量迁移路径(采纳推荐 A1)**:ensure_caddy_site 改三态分支——无站点块→新模板创建;有且含 header_up→跳过;有但缺→时间戳备份 → 整文件重写 → `caddy validate` → 失败自动还原并 exit 1 → 通过则 reload。幂等性有三态论证;选整文件重写(脚本本就托管该文件全量写)而非 sed;备选「报错转人工」违反 spec 0011「经 deploy.sh 路径」决策,否。
- **范围外登记**:install.sh 原生安装路径同样缺 TRUST_PROXY,超本票范围,不追加工单。
