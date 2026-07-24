# 01 — 单模型评估汇总 API

**What to build:** 后端新增 `GET /api/models/{id}/eval-summary` 接口，返回指定模型最近一次评估的汇总数据。响应包含总分、各能力维度得分、评估时间和所属 campaign ID。如果该模型从未被评估过，返回 `{"data": null}`（200 OK，不是 404）。前端可以调用此接口验证数据结构，为后续端点详情页和分享卡提供数据源。

**Blocked by:** None — can start immediately

**Status:** done

- [x] 新增 `internal/store/campaign.go` 方法 `GetLatestCampaignForModel(modelID int64)`，返回该模型参与的最新 campaign + 该 campaign 中该模型的各 suite 得分（`[]CampaignSuiteScore`）
- [x] 新增 `internal/server/models.go` handler `handleGetModelEvalSummary`，路由为 `GET /api/models/{id}/eval-summary`
- [x] 响应 DTO 包含字段：`model_id`（int64）、`model_id_str`（string）、`campaign_id`（int64）、`campaign_created_at`（ISO timestamp）、`total_score`（float64，加权平均）、`suite_scores`（数组，每项含 suite_id/suite_name/score/version）
- [x] 总分计算复用 `campaign_report.go` 的加权平均逻辑（suite 权重默认等权）
- [x] 模型不存在返回 404；模型存在但无评估记录返回 `{"data": null}` 200 OK
- [x] 在 `internal/server/server.go` 注册路由 `r.Get("/models/{id}/eval-summary", s.handleGetModelEvalSummary)`
- [x] 用 curl 或前端手动验证：有评估记录的模型返回完整数据、无评估记录的模型返回 null、不存在的模型返回 404
