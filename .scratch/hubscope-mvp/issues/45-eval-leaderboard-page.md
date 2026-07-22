# 45 — /eval 重写为评估榜单页

**What to build:** 按 spec 0002 前端章节与 ui-guidelines §5(Leaderboard 组件登记),把 /eval 从「分数矩阵 + 趋势图」重写为「评估榜单」纯消费页:顶部批次切换器(默认最新 done Campaign)+ 批次元信息;Hero 为 Leaderboard 条形排行,复用 ticket 31 已合入的 report API(GET /api/campaigns/{id}/report)与条形榜样式;每行带较上一批次涨跌箭头(跨 Suite 版本断点不显示箭头,占位灰 +「题目已变更」,ADR 0007);废弃 ScoreMatrix/EvalTrendChart(趋势下钻由 32 落地,行暂不可点)。/eval 走 16px 消费档密度。已合并的基础:report API(00cb11f)、CampaignReportView 报告页(727706f,可作组件来源)、0–100 分制统一(2af2f0f)。

**Blocked by:** None — 31 的 API 与组件已合 main,44 已腾空 /eval

**Status:** done

- [ ] /eval = 批次切换器 + Leaderboard 条形排行(复用 31 的 report API)
- [ ] 较上一批次涨跌箭头;跨版本断点占位灰 +「题目已变更」
- [ ] ScoreMatrix/EvalTrendChart 废弃移除(确认无其他引用)
- [ ] 批次切换器空态/榜单空态/运行中进度态/失败错误态 + 仅未完成批次轮询(ui-guidelines §6)
- [ ] /eval 16px 消费档密度
- [ ] 黑盒测试:复用 31 的 report API 测试;如需「较上一批次」数据,report API 扩展 delta 字段并补黑盒断言
