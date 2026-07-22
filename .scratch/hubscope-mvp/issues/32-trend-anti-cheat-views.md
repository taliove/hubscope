# 32 — 趋势与防作假视图(下钻 dialog)

**What to build:** 榜单页增强走势洞察:点 Leaderboard 行弹出「模型 × 批次」趋势 dialog(不做表格行内展开,ui-guidelines §4):每模型 × 每 Suite 的跨 Campaign 分数趋势线,在 Suite 版本变更处标注断点(「vN 起题目变更」);dialog 内并列展示该模型探测侧的延迟/成功率走势(复用 probe rollup 数据),分数稳但延迟暴涨一眼可见。已删除模型的趋势仍可见并带「已删除」标记。API:GET /api/campaigns/{id}/trends 按 Campaign 有序返回分数与 Suite 版本号序列 + 探测侧 rollup。

**Blocked by:** 31 — Campaign 报告与 Leaderboard

**Status:** done

- [ ] 趋势 API:按 Campaign 有序返回分数与版本号;版本变更处数据含断点信息;探测侧 rollup 同时间轴返回
- [ ] 榜单行点击弹趋势 dialog:分数线 + 版本断点标注 + 探测延迟/成功率并列
- [ ] 已删除模型趋势可见且带「已删除」标记
- [ ] dialog 内趋势数据按模型按需拉取,有加载态
- [ ] 黑盒测试:趋势 API 按 Campaign 有序返回分数与版本号
