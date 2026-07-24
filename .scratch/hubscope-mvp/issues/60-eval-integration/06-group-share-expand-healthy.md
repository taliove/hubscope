# 06 — 分组分享卡展开正常端点

**What to build:** 分组分享卡的正常端点从一行汇总改为展开列表。列表最多显示 15 个正常端点，每条显示：名称 + 24h 可用率 + 评估分（如果有）+ 24h 打点条（8px 高，无轴标）。超出 15 个的正常端点用一行兜底汇总（「另有 X 个正常端点，24h 可用率 xx%–xx%，评估分 xx–xx」）。异常明细保持 cap 10，每条第一行末尾新增评估分文本（如果有）。用户能在分组分享卡里看到每个正常端点的具体表现和评估分，不再被汇总行吞掉信息。

**Blocked by:** 02 — Overview API 带评估分（需要 overview 的 eval_score 字段）

**Status:** done

- [x] `web/src/components/StatusCardDetail.vue` 将 `healthyRangeText` 汇总行逻辑改为展开列表 + 可选汇总行
- [x] 新增 computed `healthyEntries`（`entries.filter(e => e.status === 'healthy')`）、`topHealthy`（`healthyEntries.slice(0, 15)`）、`healthyOverflow`（超出 15 的数量）
- [x] template：`topHealthy` 每项渲染一个 `.detail-item-healthy`（类似异常行，但不带 status badge），包含第一行（名称 + 可用率 + 评估分）和第二行（24h 打点条）
- [x] 第一行：`.row-name`（模型名 · 协议）+ `.row-rate av-ok`（可用率）+ `.row-eval`（评估分，`v-if="entry.eval_score !== null"`，显示「评估 XX」）
- [x] 第二行：`.row-dots`（24 格打点条，8px 高，复用异常行的样式）
- [x] `healthyOverflow > 0` 时渲染 `.healthy-overflow` 汇总行：「另有 X 个正常端点，24h 可用率 yy%–zz%，评估分 aa–bb」（计算超出部分的可用率和评估分范围）
- [x] 异常明细（`topAbnormal`）的第一行末尾新增 `.row-eval`（与正常端点同样式），显示评估分
- [x] 样式：`.detail-item-healthy` 与 `.detail-item`（异常行）保持一致的间距和对齐；`.row-eval` 字号 `--hs-text-xs`、颜色 `--hs-text-secondary`
- [ ] 实机验证：分组分享卡（如 gpt 组）正常端点全部列出（≤15 时）或前 15 + 汇总行（>15 时），每条带评估分和打点条；异常行也带评估分
