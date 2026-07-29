# 0015 — 生产线性能与系统加固(压缩 / 缓存 / overview 快照 / 容器与系统层)

> 来源:2026-07-29 生产线「访问慢」排查。确认链路 公网 → Caddy(443,终结 TLS)→ 127.0.0.1:8080 → Docker 容器(Go 二进制,embed SPA)全链无压缩、无缓存头,且静态分析定位到 `/api/overview` 为高频热点。本 spec 覆盖四个层面:A 高频访问链路、B Go 服务健壮性、C 容器/系统层、D 观测性。

## Problem Statement

生产线读者(状态板公开读者、管理台用户)感知访问慢。排查确认四类成因:

1. **传输未压缩:** Caddy 站点块只有 `reverse_proxy` + XFF,Go 端裸 `http.FileServer`;约 1MB 的 `index-*.js` 与全部 /api JSON 原样传输。
2. **静态资源零缓存:** embed.FS 文件 ModTime 为零值,`http.FileServer` 连 `Last-Modified` 都不发——浏览器连 304 条件请求都做不了,**每次打开页面全量重下所有资源**。
3. **高频热点 overview:** 状态板每个打开的标签页每 10s 轮询 `/api/overview`;每次请求对每端点执行 5 条 SQL,其中 7 天延迟基线查询每次拉取该端点 7 天全部原始探测行算 P50;全部跑在 W2 单连接上,与 prober 写入、rollup 聚合串行竞争。标签页/读者数线性放大负载。
4. **系统层隐患:** 容器 json-file 日志无轮转(磁盘慢性风险);响应头宣告 HTTP/3 但 UDP 443 放行未验证;容器无资源上限;http.Server 零超时;无慢请求观测手段。

## Solution

分层施治,小步可独立发布:

- **边缘层(随下一 tag 生效):** Caddy `encode zstd gzip` 覆盖静态资源与 /api JSON(已在 feature/release-gzip 分支落地,见实现决策 1);docker 日志轮转与资源上限随下次换容器生效。
- **缓存层:** 内容带 hash 的 `/assets/*` 配一年 immutable 缓存,HTML 配 no-cache,复访静态资源零下载。
- **热点层:** overview 响应内存快照化——探测轮次推进后失效、读路径单飞重建,handler 变 O(1) 内存读;快照带 ETag,未变时轮询回 304。
- **前端层:** 标签页隐藏时轮询降频/暂停,回前台立即刷新;echarts 模块化引入 + vendor 拆 chunk,首屏 bundle 减半。
- **系统层:** 验证 UDP 443(HTTP/3 实际可用性)、内核版本与 BBR 可行性、磁盘水位进巡检输出。
- **观测层:** 慢请求日志(>200ms warn),低成本高信号,为后续优化提供实测依据。

## User Stories

1. As a 状态板公开读者, I want 首屏 JS/CSS 经 zstd/gzip 压缩传输, so that 弱网/跨网访问首屏不再等 1MB 原样下载。
2. As a 状态板公开读者, I want /api JSON 响应同样被压缩, so that 轮询的数据传输量也下降。
3. As a 回访读者, I want 带 hash 的静态资源被浏览器长效缓存, so that 第二次打开页面几乎零下载。
4. As a 回访读者, I want 发布新版本后 HTML 立即更新, so that 我不会拿到旧 HTML 引用已不存在的 hash 资源。
5. As a 状态板读者, I want overview 数据没变时轮询只回 304, so that 常挂状态板的传输开销最小。
6. As a 运维者, I want overview 响应由内存快照直出, so that 读者数与标签页数增长不再线性放大 SQLite 压力。
7. As a 运维者, I want 快照在探测轮次推进/写操作后自动失效重建, so that 状态板数据新鲜度与现状一致(不引入可感知滞后)。
8. As a 运维者, I want 快照构建遵循 W4 时钟注入, so that 假时钟测试语义不被破坏。
9. As a 挂着状态板大屏的用户, I want 标签页隐藏时轮询自动降频, so that 无人观看时不白打服务器。
10. As a 挂着状态板的用户, I want 切回标签页时立即刷新一次, so that 我看到的永远是新数据。
11. As a 管理台用户, I want 批次 3s 轮询在标签页隐藏时暂停, so that 后台标签页不制造无效请求。
12. As a 首屏访问者, I want echarts 按需引入且与业务代码分 chunk, so that 首屏 bundle 显著变小。
13. As a 回访用户, I want echarts/element-plus 等 vendor 与业务代码分 chunk 缓存, so that 业务改版不使我重下 vendor。
14. As a 运维者, I want 容器日志有大小与份数上限, so that json-file 不再无限增长威胁磁盘。
15. As a 运维者, I want 容器有内存上限, so that 应用异常不会拖垮同机的 Caddy。
16. As a 运维者, I want 确认 UDP 443 已放行, so that 响应头宣告的 HTTP/3 真实可用而非空握手回退。
17. As a 运维者, I want 确认生产机内核版本以评估 BBR, so that 长 RTT 链路吞吐有量化结论(能开则开,不能开则记录)。
18. As a 运维者, I want 巡检输出带磁盘水位, so that 备份/日志/数据库三块磁盘消耗一目了然。
19. As a 运维者, I want 超过阈值的慢请求有日志, so that 后续优化有实测热点依据而非纯静态分析。
20. As a 运维者, I want http.Server 有基础超时配置, so that 异常连接不无限占用资源。
21. As a 部署者, I want 存量生产机的 Caddyfile 迁移走备份+校验+失败自动还原, so that 压缩上线本身不制造事故。
22. As a 维护者, I want 所有优化不改 W1–W8 承重语义, so that 回归网与部署文档依然有效。

## Implementation Decisions

1. **压缩固定在 Caddy 边缘层,Go 二进制不做压缩。** 站点块加 `encode zstd gzip`(zstd 优先——现代浏览器 Accept-Encoding 已带,gzip 兜底;Caddy 自动处理协商与 Vary)。一行同时覆盖 embed SPA 资源与全部 /api JSON。**已落地**(feature/release-gzip 分支):deploy.sh 两处模板(新建/迁移)已更新,跳过条件收紧为「有 XFF 且有 encode」,存量机器下次 `deploy.sh tag` 自动走迁移分支(备份 → 全文件覆写 → `caddy validate` → reload → 失败还原,与 spec 0011 XFF 迁移同构)。
2. **静态资源 Cache-Control 按内容寻址分级:** `/assets/*`(Vite 产物,文件名带内容 hash)→ `Cache-Control: public, max-age=31536000, immutable`;`index.html` 与 SPA 路由回落 → `Cache-Control: no-cache`(发版即生效,新 HTML 引新 hash 带动资源更新);favicon 等根级静态文件 → 短缓存(如 1h)。依据:embed.FS 零 ModTime 导致 FileServer 不发 Last-Modified,当前连 304 都不存在。
3. **overview 内存快照化,读路径懒重建 + 单飞。** 快照 = 当前 handler 的完整 DTO 计算结果,进程内单例(W5 单 evaluator/alerter 先例)。失效模型:**写路径只置脏标记,handler 遇脏单飞(singleflight)重建**——写路径包括 prober.AfterRound(手动/调度同语义,沿用 W5 钩子)、endpoint 增删/启禁、模型同步、Hub 变更。不采用「每轮探测后主动重建」:状态板无人访问时白算;懒重建保证「任何读者看到的都新鲜」且并发标签页折叠为一次计算。快照构建用注入时钟 `s.now()`(W4 兼容)。W5 语义不变:状态机推导仍以探测历史为准,快照只是读取路径的预计算,不回写、不影响告警。
4. **ETag/304 仅先行覆盖 overview。** 快照内容 hash 作 ETag;`If-None-Match` 命中回 304。与决策 3 天然共生(快照不变则 hash 不变)。其他轮询端点(批次进度等)响应小,暂不推广。
5. **前端轮询可见性感知。** `document.hidden` 时:状态板 10s 轮询降频为 60s;批次类 3s 轮询(AppHeader 批次进度、报告页、榜单)隐藏时暂停;`visibilitychange` 回前台立即触发一次刷新再恢复周期。实现收敛在轮询 composable(如 useOverview)与各处 setInterval 封装,不散落;ui-guidelines §6 轮询条目补充本约定。setInterval 配对清理纪律不变。
6. **echarts 模块化 + vendor 拆 chunk。** `import * as echarts from 'echarts'` 改 `echarts/core` + 按需注册(两个图表组件用到的 chart/component/renderer);vite `manualChunks` 把 echarts、element-plus 拆为独立 vendor chunk(配合决策 2 的 immutable 缓存,业务改版不失效 vendor)。图表组件异步化(defineAsyncComponent)留作设计评审可选项,不强制。
7. **http.Server 基础超时。** ReadHeaderTimeout ~10s、ReadTimeout/WriteTimeout ~30s、IdleTimeout ~120s(回源链路是本机 Caddy→容器,30s 写超时对 1MB 资源富余;SSE/WebSocket 不存在,无长连接冲突)。
8. **docker run 加固(随下次换容器生效):** `--log-opt max-size=50m --log-opt max-file=3`;`--memory 1g`(值可在 ticket 评审时按实机内存调整;容器常态为 Go 二进制 + SQLite,远低于此)。
9. **系统层验证项(运维检查,非代码变更):** ① 腾讯云安全组 + 宿主防火墙放行 UDP 443,`curl --http3` 实测;② `uname -r` 确认内核 ≥4.9 则开 BBR(`net.core.default_qdisc=fq` + `net.ipv4.tcp_congestion_control=bbr`),3.10 则记录结论不换内核;③ `hubscope-status` 输出追加磁盘水位一行(df 数据目录/备份目录)。
10. **慢请求日志。** Go 侧请求计时中间件:耗时 >200ms 的 /api 请求打 warn 日志(带路径与耗时,不打 body)。高频正常请求零噪音;overview 优化前后可对比。
11. **发布批次(建议,票序以 /to-tickets 为准):**
    - 批 1(纯 deploy.sh,随下一 tag):决策 1(gzip,已落地)+ 决策 8(日志轮转/内存上限)。
    - 批 2(小代码):决策 2(Cache-Control)+ 决策 7(超时)+ 决策 10(慢请求日志)。
    - 批 3(中代码):决策 3+4(快照 + ETag)+ 决策 5(visibility 轮询)。
    - 批 4(前端构建):决策 6(echarts 减肥)。
    - 批 5(运维):决策 9 验证三项,可穿插任何时候。

## Testing Decisions

- **测试原则:** 只测外部可观察行为(HTTP 响应头、状态码、响应体语义),不断言内部快照结构;快照化前后 overview 的 W1 用例应原样通过(行为不变是本重构的核心断言)。
- **后端全部走 W1 既有接缝**(httptest + stub Hub + 假时钟 + 真 SQLite 临时库),不新增接缝:
  - Cache-Control:断言 `/assets/*` 响应 immutable 一年、SPA 路由/index.html no-cache。
  - overview 快照:假时钟推进探测轮次(AfterRound)后 overview 反映新状态;endpoint 启禁/模型变更后 overview 首请求反映变更(脏标记生效);连续两请求结果一致(快照复用)。
  - ETag:同数据 `If-None-Match` 回 304;数据变化后 ETag 改变。
  - 慢请求中间件:超阈值请求产生日志(可断言到注入的 logger,或降级为代码审查项)。
  - 先例:现有 `internal/server/*_test.go` 的 overview/auth/spa 测试模式。
- **前端走 vitest 既有接缝:** 可见性轮询 composable(hidden 降频/暂停、回前台即刷、卸载清理);echarts 改动靠既有图表组件测试 + 构建产物体积对比(typecheck/build 硬门禁)。
- **ops 项走部署时人工验证清单**(spec 0008 先例,不引入 shell 测试框架):`caddy validate`、`curl -H "Accept-Encoding: zstd"` 断言 Content-Encoding、`docker inspect` 日志配置、`curl --http3`、`uname -r`。验证清单写进 deploy-prod skill。
- **承重墙约束:** W1 接缝定义、W2 单连接、W4 时钟注入、W5 状态机/告警语义均不变;快照是读取路径优化,状态机输入不变。

## Out of Scope

- **SQLite WAL / 连接池重构:** 当前单连接串行读写在 overview 快照化后不再是读热点;WAL 收益被 MaxOpenConns(1) 设计抵消,列为观察项不动 W2。
- **BBR 换内核(elrepo):** 仅评估记录;3.10 内核不换。
- **预压缩产物(vite 生成 .gz/.zst + Go 直出):** 省边缘 CPU 的进阶项,Caddy encode 已够用,列为后续可选。
- **SSE/WebSocket 替代轮询:** 快照化 + ETag 后轮询成本已极低,不引入新推送通道。
- **CDN 接入:** 单机房单域名,无计划。
- **API ETag 全面推广:** 仅 overview;其他端点响应小。
- **测试线(deploy-test-101)对齐:** 测试线裸 HTTP 无 Caddy,压缩不适用;Cache-Control/快照/超时等应用层改动天然两条线同享。

## 票登记(2026-07-29 /to-tickets,GitHub Issues)

| 票 | 内容 | 批次 | 阻塞 |
|---|---|---|---|
| #18 | 边缘加固:docker 日志轮转 + 容器内存上限(决策 8) | 批 1 | — |
| #19 | 静态资源 Cache-Control 分级(决策 2) | 批 2 | — |
| #20 | http.Server 超时 + 慢请求日志(决策 7+10) | 批 2 | — |
| #21 | overview 内存快照化(决策 3,核心票) | 批 3 | — |
| #22 | 前端轮询可见性降频(决策 5) | 批 3 | — |
| #23 | echarts 模块化 + vendor 拆包(决策 6) | 批 4 | — |
| #24 | 运维验证:UDP 443 / BBR / 磁盘水位(决策 9) | 批 5 | — |
| #25 | overview ETag/304(决策 4) | 批 3 | #21 |

## Further Notes

- **承重墙四问(概答):** ① 为什么必须改——生产线可感知慢 + 磁盘慢性风险,非过早优化(热点有代码级证据:overview 每请求 5N 查询含 7 天原始行基线);② 影响哪些调用方——overview 消费方(Dashboard/HealthBanner/StatusCard 快照源)行为不变,W1 用例应原样通过;③ 替代方案——短 TTL 请求缓存(更简单但仍有打库路径)、WAL(动 W2,否),快照化与 W5 单例先例最同构;④ 回归测试什么——overview 现有 W1 用例 + 新增快照失效/ETag/Cache-Control 断言。
- **快照化最大风险是失效漏挂**(某写路径忘置脏 → 状态板滞后)。防御:脏标记收敛为单一 `InvalidateOverview()` 入口,读路径遇脏重建(不依赖每路径记得主动重建);W1 测试覆盖「探测后/变更后 overview 反映新状态」。
- **gzip 分支现状:** feature/release-gzip 分支落后 main(无独立提交),deploy.sh 改动未提交;本 spec 落地时先提交该分支改动再并线。
- **效果预估(静态分析,上线后以慢请求日志复核):** 首屏传输 1MB → ~300KB(zstd 更优);复访静态资源 → 0 下载;overview 打库压力 → 与读者数解耦。
