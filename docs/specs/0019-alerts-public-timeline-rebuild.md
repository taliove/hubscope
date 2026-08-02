# spec 0019:故障记录公开化与时间线重构

> 2026-08-02 用户 grill 落锤(五项裁决)+ main 补充落锤(三项)。规格权威分层:业务语义边界 → ui-guidelines 附录第 16 项与 §5 故障记录条;页面构成规格 → `web/.impeccable/surfaces/web-src-views-alertsview-vue.md`;本文件 = 决策记录与票拆分。spec 编号自 0019 起(ui-guidelines 附录第 12 项,与 spec 0018·端点退役 撞车不重排)。

## 背景

故障记录页(/alerts,GH #117 时间线重建)当前是纯登录面:路由无 `meta.public`,`/api/alerts` 不在 `publicReadPattern`(auth.go:332)。用户裁决把它放给未登录读者,并做四项体验重构 + 一张独立版本显示小票。

**顺带关闭的潜伏越权(plan 影响分析发现):** `alerts.go` 匿名分支(`u == nil`)走 `ListAlertEventsAll`,含 super_admin 级 hub-less 事件(score_drop/batch 等)——只是恰好被中间件拦住,`alerts.go:36` 的「Public (read)」注释与行为脱节。本次把匿名分支改为构造性安全的四类视图。

## 裁决(2026-08-02 用户落锤)

1. **公开边界 = 故障叙事四类。** 匿名只见 down / recovered / group_down / group_recovered;运维管道七类(test / batch / quiet_summary / score_drop / score_drop_skipped / retire_pending / retired)保持登录可见。**分叉在服务端 API 层**:匿名 payload 构造性不含七类与 hub-less 事件,不靠前端隐藏。匿名视图全局(不过 hub 滤),与公开 overview 匿名全局同口径。
2. **RecentEvents 不同步开放。** 状态概览四卡保持登录渲染;匿名事故叙事入口 = 侧栏「故障记录」。这是对 GH #132「alerts API 是会话保护面」登记的部分推翻。
3. **时间线月 > 周 > 日三级嵌套,月/周可折叠,轨道线跨日跨周连续不断**;跨年月标签带年,不另立年层级。
4. **事件行内展开详情**(EvalLiveFeed 先例,否掉弹窗/面板):完整 message、完整时间戳、事件/端点 ID、配对恢复事件与时长、投递明细;端点事件附「查看端点详情」链接。
5. **筛选 URL 深链。** model / kind / range 三参数 `router.replace` 镜像,照 ModelStatusList 五参数先例(200ms 防抖、默认值不入参、URL 优先、编解码纯函数 + vitest)。

## 补充落锤(同日)

- **Hub 名暴露接受并登记(用户):** group 事件 message 含 Hub 名分段是新增匿名暴露面——Hub 名 = 租户标签,与 overview 匿名全局同层级;alerter 文案与历史行零改动。
- **sent_ok 不分叉(用户):** 投递状态全员可见——事故证据链一部分,非敏感。
- **折叠默认态与周标签(main):** 当月及其各周默认展开、更早月折叠、会话内保持不持久化;周标签 = 周一始日期区间「M/D–M/D」(否掉「第 N 周」)。

## 关键实现决策(plan 影响分析结论)

- **过滤在 store 层,不在 handler。** handler post-filter 会让 `limit` 窗口被隐藏类稀释(匿名拉 50 条可能只渲 20 条可见),违反「空态不冒充清白记录」。store 新增只读查询变体(kinds 参数化,复用 `listAlertEvents` 共享实现,无 schema 变更,W2 不动);**四类白名单常量放 server 层**(与 publicReadPattern 同文件,信息边界单点可读)。
- **同端点按会话分叉,不新建 /api/public/alerts**——与 overview/endpoints/probes 的既有公开面机制一致。
- authed 三分支(super_admin 全量 / hub admin 本 Hub / 无 hub 空列表)一行不动,isolation sweep 保绿即为证据。

## W6 承重墙四问(摘要)

- **为什么必须改:** 故障叙事是状态板公信力的组成部分,与已公开 overview/probes 同层级;且现状匿名分支是注释与行为脱节的潜伏越权,早改早关闭。
- **影响哪些调用方:** security_test(/api/alerts 自 protectedPaths 移出,新建「public-filtered」断言类,lookalikes 补 /api/alertsx、/api/alerts/)、isolation_test(增匿名断言)、RecentEvents(零变化)、sidebarNav/router/AlertsView。
- **替代方案:** 前端隐藏七类(裁决否决)、独立公开端点(双口径,否)、handler post-filter(窗口稀释,否)。
- **回归测试:** 匿名 200 且 payload 构造性不含七类(响应体 grep 七类 kind 字串缺席 + alertGlobalMarker 探针缺席);匿名跨 Hub 可见(全局口径钉死);authed 三角色逐字不变;message 文案全部生成点已 grep(evaluator/group/quiet/score_drop/test_alert),四类文案不含 webhook/token/投递细节。

## 票拆分(六票,依赖序)

| 票 | 内容 | 依赖 |
|---|---|---|
| 1(后端) | publicReadPattern + alerts 加 alerts;store 层 kinds 过滤查询;匿名四类视图;陈旧注释修正;W1 黑盒(匿名四类/全局口径/authed 不变/security_test 断言迁移) | — |
| 2(前端) | /alerts meta.public;sidebarNav public 三项;匿名类型筛选器只列四类;vitest 更新 | 票 1 |
| 3(前端) | model/kind/range 深链三参数 + 编解码纯函数 + vitest | 票 2 |
| 4(前端) | 事件行内展开详情(复用 pairIncidentDurations,禁第二配对口径) | 票 2 |
| 5(前端) | 月>周>日三级分组纯函数 + 折叠 + 轨道连续化(日界裁剪作废,裁剪只在月组边界) | 票 2 |
| 6(前端,独立) | AppSidebar shortVersion 的 dev- 分支(dev-g<短哈希>,title 留全串;deploy.sh 不动) | — |

票 3/4/5 同触 AlertsView.vue,串行实现;票 6 独立可随时插入。

## 验收口径

- 匿名打开 /alerts:页面可达,时间线只含四类事件,类型筛选器只有四个选项;翻 limit 窗口不被隐藏类稀释(可见事件计满窗口)。
- 登录打开 /alerts:十一类全量与现行行为逐字不变。
- 深链:改筛选 → URL 200ms 内镜像(默认值净 URL);带参打开 → 筛选还原;坏值回落默认。
- 详情:行点击/Enter/Space 就地展开,aria-expanded;内容五件齐(完整 message/完整时间戳/ID 对/配对恢复+时长/投递明细);端点事件链接可达。
- 折叠:月/周组可折叠,折叠不丢事件不重新拉取;轨道线跨日跨周连续,只在月组边界收口;默认当月展开、更早月折叠。
- 版本:dev 构建侧栏显 `dev-g<短哈希>`,title 悬停见完整串;release 串行为不变。
