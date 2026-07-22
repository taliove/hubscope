# 31 — Campaign 报告与 Leaderboard

**What to build:** 每个完成的 Campaign 有报告页:Leaderboard 柱状图(每模型一行,总分 = 各 Suite 分的加权平均,0–100,默认等权;权重在设置页可调),可切换到按单个 Suite 查看分数,可按模型 category 过滤,排序列可切换(总分/各 Suite)。已删除模型不上榜(沿用 17 的口径)。API:GET /api/campaigns/{id}/report 返回 Leaderboard 聚合数据与各 Suite 明细。

**Blocked by:** 26 — 评估视图隐藏已删除模型;29 — Eval Campaign + 一键全量评估

**Status:** ready-for-agent

- [ ] Leaderboard:总分柱状排行 + 分 Suite 视图,样式对标 DesignArena 式条形图
- [ ] 按 category 过滤、按总分/各 Suite 排序切换
- [ ] Suite 权重设置(默认等权)生效于总分;设置页可改
- [ ] 已删除模型不出现在 Leaderboard
- [ ] 黑盒测试:report API 的总分加权、过滤与排序正确
