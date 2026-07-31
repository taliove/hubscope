---
version: 1
slug: "web-src-views-endpointdetailview-vue"
primary_target: "web/src/views/EndpointDetailView.vue"
related_targets: ["web/src/components/TimeSeriesChart.vue","web/src/components/ProbeRecordTable.vue","web/src/components/StatusBadge.vue","web/src/components/StatusShareDialog.vue","web/src/utils/protocol.ts"]
---

# EndpointDetail 端点详情(v2)— 表面简报

> **v2 更新(2026-08-01,GH #122):** 外壳迁 AppSidebar(GH #112),详情面板(GH #116)承担速览后本页为深钻终点;新视觉 = 页面标题档 + 轻容器指标卡 + 图表 Apple Health 化(GH #114)。

## 范围与模式
- 模式:**Operate**(公开排障面);route `/endpoints/:id`,未登录可达;壳内渲染。
- 读者:排障中的运维/状态板读者下钻(列表行 → 详情面板 → 「打开完整详情」→ 本页)。任务:30 秒回答「什么时候坏的、坏成什么样、是不是只有我」。

## 页面构成(自上而下)
1. **标题行:** h1 模型名(对象名 h1——「页面 h1 = 侧边栏标签」惯例不管辖深链页)+ 协议 tag(`protocolTagType` 集中映射)+ Hub 名 + 登录态「分享」按钮(StatusShareDialog 单模型入口,含完整版/紧凑版切换,紧凑版 = 端点小卡,规格见 share-materials 简报)
2. **状态行:** StatusBadge(显示层三态,`reason` 经 title 提供判定依据——detail 契约不提供 `degrade_causes` 字段,本页不显示成因副标签,与列表/面板同映射不同字段集)+ 已停用 tag + status_reason
3. **指标卡区:** 24h 可用率 KPI(固定 24h 口径,GH #56——复用 overview entry dots_24h 聚合,**不随窗口控件漂移**;`availabilityRateTier` *-text 阶着色;null 注「24h 内无探测数据」)+ 评估总分卡(最近 settle 批次;suite tags 封顶展示)
4. **窗口/mode 控件行:** 24h/7天/30天 × 合并/流式/非流式(只驱动图表)+ 登录态「评估此模型」主按钮
5. **时序图表区(TimeSeriesChart × 3):** 延迟(P50/P95)/ TTFT(非流式模式隐藏)/ 成功率
6. **近期失败表(ProbeRecordTable,最近 20 条)**

## 组件规格
- **协议 tag / StatusBadge:** 集中映射复用(ui-guidelines §5),禁止本地另造。
- **TimeSeriesChart(GH #114 轻量化):** ECharts,色板走 `utils/chartColors.ts`(LIGHT 单份,暗色键位预留);**null 断线纪律(GH #56 起生效):** connectNulls:false——无探测时段断线,不插值编造数据点;平滑走 `utils/monotoneSmooth.ts` 单调插值(不发明极值;样式口径 = 线宽 2、无逐点圆点、y 轴无线无刻度 + 虚线网格、x 轴标签稀疏,tooltip 保真值;入场绘制 1000ms chartMotion 门控)。30 天窗口(5761 点)超 animationThreshold 自动关动画(已登记)。
- **稳定性 KPI:** 固定 24h 口径(见上),窗口/mode 控件只驱动图表。
- **匿名/登录信息边界:** 探测与评估数据公开;管理动作(评估触发、分享)按会话门控;匿名态跳过 suites/models 请求。

## 错误恢复(GH #56 起生效,沿置)
加载失败 alert 带原因 + 重试按钮(重跑首载加载链);eval summary 加载失败渲染「评估数据加载失败 · 重试」(中性 secondary,非 danger——评估卡是辅助信息,danger 会读作端点事故),与「暂无评估数据」真空态明确区分(**失败不冒充空态**);header/metrics 区首载 skeleton。

## 退役登记
AppHeader / PublicFooter 随 GH #112 退役;本页导航与登录入口由 AppSidebar 承担。速览职责迁 ModelDetailPanel(GH #116),本页保持深链可用(书签与分享链接不失效,spec 0018 §10 story 37)。

## 未决(另立批次)
- 排障动线重构(证据上移 + 三图收敛)、状态轮询与「更新于」、同 Hub 邻接上下文(「是不是只有我」零覆盖)、视图状态 URL 深链。
