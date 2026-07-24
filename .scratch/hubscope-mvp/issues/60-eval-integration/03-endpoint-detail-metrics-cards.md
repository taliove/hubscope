# 03 — 端点详情页展示评估分

**What to build:** 端点详情页在状态行下方新增「指标卡片行」，并列展示两个指标卡片。左卡片显示「24h 稳定性 XX/100」（来自探测 score，按三档着色：≥95 绿、<95 黄、0 红、null 灰）。右卡片显示「评估总分 XX/100」+ 各能力维度得分 tag（来自 `GET /api/models/{id}/eval-summary`），如果无评估记录则显示灰色「暂无评估数据」卡片。用户能在详情页一眼看到该模型的稳定性和智商两个维度的表现。

**Blocked by:** 01 — 单模型评估汇总 API（需要调用该 API 获取评估分）

**Status:** done

- [x] `web/src/api/evals.ts` 新增 `getModelEvalSummary(modelId: number)` 方法，调用 `GET /api/models/{id}/eval-summary`
- [x] `web/src/api/types.ts` 新增接口 `ModelEvalSummary`（对应后端 DTO）
- [x] `web/src/views/EndpointDetailView.vue` 在状态行（`.status-row`）后、控件行（`.controls`）前插入 `.metrics-row`
- [x] `.metrics-row` 包含两个 `el-card`：左卡片「稳定性」、右卡片「评估分」或「暂无评估数据」
- [x] 左卡片：显示探测分，按 `availabilityTier` 三档着色（复用 `statusCardSummary.ts` 的 tier 函数）
- [x] 右卡片有评估数据时：显示 `evalSummary.total_score` + `el-tag` 列表（各 suite 的 name + score），底部小字显示评估时间
- [x] 右卡片无评估数据时：灰色背景 + 「暂无评估数据」文本（不显示「评估此模型」按钮，那是下个 ticket）
- [x] 页面加载时调用 `getModelEvalSummary`，失败或返回 null 时 `evalSummary.value = null`
- [x] 实机验证：有评估记录的模型显示两个卡片都有数据、无评估记录的模型右卡片显示灰色空态
