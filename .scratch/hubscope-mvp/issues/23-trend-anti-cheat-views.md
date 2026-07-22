# 23 — 趋势与防作假视图

**What to build:** 报告页增强走势洞察:每模型 × 每 Suite 的跨 Campaign 分数趋势图,趋势在 Suite 版本变更处标注断点(「v2 起题目变更」);报告页并列展示该模型探测侧的延迟/成功率走势(复用 probe rollup 数据),分数稳但延迟暴涨一眼可见。已删除模型的趋势仍可见并带「已删除」标记。

**Blocked by:** 21 — 题库专业化:扩题 + Suite 版本化 + 采样;22 — Campaign 报告与 Leaderboard

**Status:** ready-for-agent

- [ ] 每模型 × 每 Suite 跨 Campaign 趋势线,版本断点有标注
- [ ] 报告页并列展示探测延迟/成功率走势
- [ ] 已删除模型趋势可见且带「已删除」标记
- [ ] 黑盒测试:趋势 API 按 Campaign 有序返回分数与版本号
