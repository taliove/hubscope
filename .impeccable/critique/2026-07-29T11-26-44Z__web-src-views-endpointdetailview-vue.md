---
target: "EndpointDetail 复测(GH #56 后)"
total_score: 25
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 0
timestamp: 2026-07-29T11-26-44Z
slug: web-src-views-endpointdetailview-vue
---
# 复测 Critique:EndpointDetail 端点详情(改进后)

Method: dual-agent(A:设计评审 source-only 降级声明 · B:机械检测)

## 基线闭环:scope 内 1 P1 + 2 P2 全部闭环(动线重构/轮询/邻接另批,不评)
- P1「24h 稳定性」标签说谎 → KPI 固定 24h 口径 endpointAvailability24h,改词「24h 可用率」+ formatPercent,禁 buckets 回落
- P2 connectNulls+smooth → false/false,断线为证据
- P2 错误恢复残缺+eval 冒充空态 → 全链重试、evalError 独立态中性色、header/metrics skeleton

## Nielsen:22/40 → 25/40(+3,55% → 62.5%)
升:#2 2→3(标签不再说谎)、#9 2→3(全链重试+可区分)、#10 1→2(KPI 自解释)
持平:#1 2(无轮询,另批)、#3 2(无返回路径)、#7 2、#8 2(三图等权,动线批)

## 机械检测:0 findings;令牌纪律 0 违规

## 残留(另批)
动线重构(状态结论区/证据上移/三图收敛/同 Hub 邻接)、状态轮询与更新于、KPI 24px 字阶、status_reason 多处重复。
