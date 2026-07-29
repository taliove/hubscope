---
version: 1
slug: "web-src-views-endpointdetailview-vue"
primary_target: "web/src/views/EndpointDetailView.vue"
related_targets: ["web/src/components/TimeSeriesChart.vue","web/src/components/ProbeRecordTable.vue"]
---

# EndpointDetail 端点详情 — 表面简报

## 范围与模式
- 模式:**Operate**(公开排障面);route `/endpoints/:id`,未登录可达。
- 读者:排障中的运维/状态板读者下钻。任务:30 秒回答「什么时候坏的、坏成什么样、是不是只有我」。

## 页面构成
1. AppHeader(公开侧形态,同 Dashboard 简报)
2. 标题行:模型名 + 协议 tag + StatusBadge(含降级成因副标签)+ status_reason + 登录态操作(评估此模型、分享)
3. 指标卡区:稳定性 KPI + 评估总分卡
4. 窗口/mode 控件行(24h/7天/30天 × 合并/流式/非流式)
5. 时序图表区(TimeSeriesChart:成功率/延迟/TTFT——非流式模式隐藏 TTFT 图)
6. 近期失败表(ProbeRecordTable,最近 20 条)
7. PublicFooter

## 组件规格
- **协议 tag / StatusBadge:** 与 Dashboard 简报同源复用,禁止本地另造映射。
- **TimeSeriesChart:** ECharts 折线,色板走 utils/chartColors.ts 亮暗双镜像,主题切换重渲染。**null 断线纪律(GH #56 起生效):** connectNulls:false——无探测时段断线,不插值编造数据点;smooth:false——延迟尖峰可见,曲线是证据。
- **稳定性 KPI(GH #56 起生效):** 固定 24h 口径(复用 overview entry dots_24h 聚合,批 59 纯函数),窗口/mode 控件只驱动图表,KPI 数值不随控件漂移。
- **匿名/登录信息边界:** 探测与评估数据公开;管理动作(评估触发、分享管理)按会话门控;匿名态跳过 suites/models 请求。

## 错误恢复(GH #56 起生效)
加载失败 alert 带原因 + 重试按钮(重跑首载加载链);eval summary 加载失败渲染「评估数据加载失败 · 重试」,与「暂无评估数据」真空态明确区分(失败不冒充空态);header/metrics 区首载 skeleton。

## 体检基线与已排改进
- critique 基线 22/40(2026-07-29):KPI 口径说谎(P1)、排障动线倒置(P1)、connectNulls 插值(P2)、状态无轮询(P2)、错误恢复残缺 + eval 失败冒充空态(P2)。
- 已排票:#56(诚实度 + 错误恢复修正,本简报相关条款已按修后口径登记)。

## 未决(另立批次)
- 排障动线重构(状态结论区 + 复用 24h 分段条 + 证据上移 + 三图收敛)、状态轮询与「更新于」、同 Hub 邻接上下文(「是不是只有我」零覆盖)、视图状态 URL 深链。
