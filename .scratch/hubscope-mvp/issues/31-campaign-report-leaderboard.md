# 31 — Campaign 报告与 Leaderboard(评估榜单页)

**What to build:** /eval 重写为「评估榜单」纯消费页(2026-07-22 重组定稿,spec 0002 前端章节):顶部批次切换器(默认最新 done Campaign)+ 批次元信息;Hero 为 Leaderboard 条形排行——每模型一行,总分 0–100(各 Suite 加权平均,默认等权,权重在设置页可调),带较上一批次涨跌箭头;可切单个 Suite 查看、按模型 family 过滤、切换排序(总分/各 Suite)。已删除模型不上榜(沿用 26 的口径)。API:GET /api/campaigns/{id}/report 返回 Leaderboard 聚合数据与各 Suite 明细。行暂不可点(趋势下钻由 32 落地)。

**Blocked by:** 44 — 评估运营与题库挪入管理台(先腾空 /eval 再重写)

**Status:** done

- [ ] report API:总分加权(自定义权重生效)、family 过滤、排序切换;已删除模型不上榜
- [ ] /eval 榜单页:批次切换器 + Leaderboard 条形排行(DesignArena 式),废弃 ScoreMatrix/EvalTrendChart
- [ ] Suite 权重设置(默认等权)生效于总分;设置页可改
- [ ] 评审条件:跨 Suite 版本断点不显示涨跌箭头(占位灰 +「题目已变更」);等待中/运行中批次榜单区呈进度态不显示半成品名次,失败批次错误态 + 原因;仅选中未完成批次时轮询
- [ ] 分数统一 0–100,formatScore 集中 utils/format.ts,组件禁自写 toFixed(含 44 挪入管理台的运营 tab 一并迁移)
- [ ] /eval 走 16px 消费档密度(读者是看板消费者,与登录态无关)
- [ ] 黑盒测试:report API 的总分加权、过滤与排序正确
