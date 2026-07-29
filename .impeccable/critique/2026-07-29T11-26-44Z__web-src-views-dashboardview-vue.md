---
target: "状态板 Dashboard 复测(GH #52-#55 后)"
total_score: 28
max_score: 36
na_heuristics: 5
p0_count: 0
p1_count: 0
timestamp: 2026-07-29T11-26-44Z
slug: web-src-views-dashboardview-vue
---
# 复测 Critique:状态板 Dashboard(改进后)

Method: dual-agent(A:设计评审 source-only 降级声明 · B:机械检测)

## 基线闭环:3/3 P1 + 2/2 P2 全部闭环
- P1 严重度不驱动首屏 → severitySort(组间/组内/flat/disabled 沉底)+ banner 异常 chips 点名
- P1 banner/strip 重复 + 元信息混排 → 计数归 strip,元信息 xs/placeholder 右移;banner = 结论 + display 大数字 + chips
- P1 卡片墙均质化 → StatusBadge md 档、P50 主 P95 次、splitMiddle 保后缀、评分前缀、tag 收敛、minmax 300
- P2 排序口径两套 → SEVERITY_ORDER 单一来源,strip/组头同消费;「本组:」前缀
- P2 下拉 placeholder 当 label → 内联 label;双控登记为有意为之

## Nielsen:22/36 → 28/36(+6,Acceptable 61% → Good 78%)
升:#1 3→4(最严重端点首屏可见+chips 点名)、#3 3→4(选中态浅底,出口可见)、#4 2→3(口径统一)、#6 2→3(label+保后缀)、#7 2→3(严重度排序)、#8 2→3(去重+主从)
持平:#2 3、#9 3、#10 2(dots 无图例,另批);#5 n/a

## 机械检测:0 findings(与基线持平);令牌纪律 0 违规

## 新增问题(改进引入,均 P3)
- P3「评分 N」与评估域「评估总分」词根相邻,跨页消歧负担(tooltip 兜底,后续裁决措辞)
- P3 EndpointDetail hub-scope 边缘「24h 内无探测数据」措辞(有数据不可见 vs 无数据)
- P3「+N」overflow chip 与可点 chips 同形不同性

## 残留(另批)
strip span 无 role/tabindex(a11y 批);KPI 24px 字阶无登记用途;组头计数是未筛选聚合;dots 图例;暗色 section 阶跃。
