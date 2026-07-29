---
target: /board 公开榜单页(纯源码)
total_score: 26
max_score: 36
na_heuristics: 5
p0_count: 0
p1_count: 2
timestamp: 2026-07-29T08-27-34Z
slug: web-src-views-boardview-vue
---
# Critique:/board 公开榜单页

Method: single-agent, source-only(无实机截图/浏览器证据,降级已声明;detect.mjs 未运行)

## Design Health Score:26/36(Good 72%,heuristic 5 n/a)

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3 | 三态齐全;但无数据时效可见性——不跑批次时完全不显示批次号与完成时间 |
| 2 | Match System / Real World | 2 | 「批次 #N」「系列」「判分不完整,缺 N/M 维度」是内部口径直接外泄 |
| 3 | User Control and Freedom | 3 | 只读页无被困路径 |
| 4 | Consistency and Standards | 4 | 与 /eval、分享页同组件同口径;零违规 |
| 6 | Recognition Rather Than Recall | 3 | 维度列头截断藏全名于 hover;涨跌含义依赖远处的 baselineNote;无图例 |
| 7 | Flexibility and Efficiency | 3 | 列头排序+family 筛选+键盘可达;状态不进 URL |
| 8 | Aesthetic and Minimalist | 4 | 中性轨道+档色唯一跳色,纪律执行到位 |
| 9 | Error Recovery | 3 | 空态三口径分层是真功夫 |
| 10 | Help and Documentation | 1 | 零解释:0-100 是什么、档色、▲▼比谁、维度是什么,公开读者无入口 |

## Design Specificity
榜单本体高度产品化(ScoreCell 档色、恒刻度条、判分不完整水印——换产品照搬即语义错误);页面外壳近乎匿名(h1 + 一张卡片 + 页脚,任何产品可用)。产品最独特的资产(评估可信度叙事)停在卡片边界之内。

## Priority Issues
- **[P1] 页面不标注批次身份与数据时效**(BoardView.vue):report.id/finished_at/trigger 已在 payload 未渲染;防作假立身之本的产品不答「这数据是什么时候的」;EvalCard 导出物有批次 chip + 生成于页脚,导出物比来源页面更自明。Fix:标题行下 meta 行「批次 #N · 定时/手动 · 完成于时间」。
- **[P1] 公开读者零解释层**(BoardView/Leaderboard):无图例无副标题;Jordan 第 10 秒放弃。Fix:标题下一句话副标题 + 工具条 info 图标弹 popover 图例(与 ScoreCell tooltip 同源,不造第二口径)。
- **[P2] 涨跌列理解依赖记忆桥**(Leaderboard):列头与 baselineNote 隔整张表。Fix:图例 popover 统一承载(被 P1-2 覆盖)。
- **[P2] 列头排序无可发现性**(Leaderboard .h-sortable):唯一提示是 hover。Fix:常驻弱化排序指示。
- **[P3] 视图状态不进 URL**(BoardView L63-64):刷新即丢,不可深链过滤视图。

## Persona 红旗
- Jordan(最高风险):5 秒能答「谁第一」,答不了「这是什么榜、谁评的、什么时候的」;「判分不完整,缺 2/5 维度」无上下文读作「模型坏了/网站坏了」。
- Sam:大体良好;warning 档 #d97706 白底 ≈3.9:1 低于 AA 4.5:1——令牌层取值问题(semantics.css),非本页独有,登记。
- Alex:排序无 affordance;视图不可深链。

## 本轮 scope 裁定(2026-07-29 grill)
顺收修正类:P1-1 批次 meta 行。另立批次:P1-2 解释层(副标题+图例)、P2 排序可发现性、P3 URL 深链。附记:「系列」vs 状态板分组 chip 词表「厂商」两处不一致,迁移线统一词表时一并裁决。
