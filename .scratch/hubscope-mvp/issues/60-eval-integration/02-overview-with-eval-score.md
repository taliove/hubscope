# 02 — Overview API 带评估分

**What to build:** `GET /api/overview` 响应的每个 endpoint 新增 `eval_score` 字段（类型 `*float64`，无评估记录时为 `null`）。该字段值来自该 endpoint 对应 model 的最近一次评估总分。Dashboard 和分组分享能拿到评估分数据，但本 ticket 不改展示逻辑（保持现状），只确保数据管道打通。

**Blocked by:** 01 — 单模型评估汇总 API（复用其 store 层查询逻辑）

**Status:** done

- [x] `internal/server/overview.go` 的 DTO struct 新增字段 `EvalScore *float64 \`json:"eval_score"\``
- [x] 在 `handleGetOverview` 中，查询所有 model 的最新评估总分（批量查询，避免 N+1），填充到每个 endpoint 的 `eval_score` 字段
- [x] 复用 ticket 01 的 `GetLatestCampaignForModel` 或新增批量版本 `GetLatestEvalScoresForModels(modelIDs []int64) map[int64]float64`
- [x] 用 curl 验证：`GET /api/overview` 返回的每个 endpoint 有 `eval_score` 字段，有评估记录的为数字、无记录的为 `null`
- [x] 前端 `web/src/api/types.ts` 的 `OverviewEntry` 接口新增 `eval_score: number | null`（确保类型安全）
- [x] Dashboard 页面仍正常渲染（不展示 eval_score，但数据已在）

## 实现细节

- 新增 `internal/store/campaign.go::GetLatestCampaignsForModels` 批量查询方法
- 新增 `internal/server/overview.go::getEvalScoresForModels` 辅助函数，复用评估报告计算逻辑
- 所有测试通过，API 验证成功（有评估分的显示数字，无评估分的显示 null）

