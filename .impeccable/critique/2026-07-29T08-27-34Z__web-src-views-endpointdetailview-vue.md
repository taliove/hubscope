---
target: EndpointDetail 端点详情页(纯源码)
total_score: 22
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
timestamp: 2026-07-29T08-27-34Z
slug: web-src-views-endpointdetailview-vue
---
# Critique:EndpointDetail 端点详情页

Method: single-agent, source-only(无实机截图/浏览器证据,降级已声明;detect.mjs 未运行)

## Design Health Score:22/40(Acceptable 下沿)

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 2 | 状态是页面加载时快照,无轮询无「更新于」;Dashboard 有 10s 轮询详情页反而没有 |
| 2 | Match System / Real World | 2 | 「24h 稳定性」标签说谎:该分随窗口/mode 控件漂移,标签不变 |
| 3 | User Control and Freedom | 2 | 无返回路径(无面包屑/返回链接) |
| 4 | Consistency and Standards | 3 | 组件纪律好;但 connectNulls 与 TrendChart「null 断线」纪律相左;指标 24px 不在字阶登记用途 |
| 5 | Error Prevention | 3 | 只读为主,唯一写操作走确认 |
| 6 | Recognition Rather Than Recall | 3 | 窗口/mode 不进 URL,视图不可恢复不可分享 |
| 7 | Flexibility and Efficiency | 2 | 无键盘、无深链、无「同 Hub 其他端点」跳转 |
| 8 | Aesthetic and Minimalist | 2 | 三张同形全宽图等权平铺;status_reason 三重重复 |
| 9 | Error Recovery | 2 | alert 无重试按钮(违反 §6 三态);eval 加载失败静默冒充空态 |
| 10 | Help and Documentation | 1 | 「24h 稳定性」口径零解释;仅 TTFT 一处行内注释 |

## Design Specificity
品类可互换的 Grafana-lite 下钻页;产品性格只在 StatusBadge 与分享入口;Dashboard 卡片上的识别元素(24h 分段条、sparkline、基线虚线)在本页全部缺席——从卡片点进来反而丢失视觉锚点。

## Priority Issues
- **[P1]「24h 稳定性」KPI 与筛选控件隐性耦合,标签说谎**(EndpointDetailView L27-33/L188-204):由 buckets 现算,切窗口/mode 后口径静默漂移;且是前端近似。Fix:固定 24h 口径当锚,或标签随选择改名;二选一不允许现状。防作假语义在自家页面的反面。
- **[P1] 排障动线倒置**(L18-24/L77-94):状态无锚点(reason 一行小字)、证据(失败表)沉三张 240px 图之后、「是不是只有我(同 Hub 邻接状态)」零覆盖。Fix:状态行升级结论区(复用 24h 分段条)、失败上移、三图收敛。
- **[P2] connectNulls:true + smooth:true**(TimeSeriesChart L55-62):插值编造数据点、平滑隐藏尖峰,违反 null 断线纪律同精神。Fix:connectNulls:false、smooth:false。
- **[P2] 状态快照不自更新**(L261-292):零轮询零新鲜度指示。Fix:复用 visibilityPoll 低频轮询或「更新于」时间戳。
- **[P2] 错误恢复残缺 + eval 失败冒充空态**(L69-75/L218-227/L288-290):alert 无重试;loadEvalSummary catch 后渲染「暂无评估数据」与真空态不可区分;无 skeleton。

## Persona 红旗
- 排障运维读者(项目特有):30 秒三问(什么时候坏的/坏成什么样/是不是只有我)全不及格;整 Hub 挂与单端点挂在本页看起来一模一样。
- Alex:视图状态不进 URL;无同 Hub 邻接跳转。
- Sam:三张 ECharts 对屏幕阅读器是黑洞;hs-blink 无 prefers-reduced-motion 衰减(登记)。

## 本轮 scope 裁定(2026-07-29 grill)
顺收修正类:P1-1 KPI 口径(固定 24h 或标签随窗口)、P2 connectNulls/smooth、P2 错误恢复/eval 空态冒充。另立批次:P1-2 动线倒置重构、P2 状态轮询。
