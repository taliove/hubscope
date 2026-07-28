# 91 — 覆盖率门槛:后端报告口径(不完整模型不参与排名)

**What to build:** 按 spec 0014(docs/specs/0014-authoritative-benchmark-eval.md)实现决策 A,堵住「缺考状元」。行为:报告聚合时判定每个模型的**完整性**——全部启用 suite 的 AVG(score) 非 nil(每维度至少判上 1 题)才算完整。完整模型:照旧计算总分(AD 0005 加权 + ADR 0009 nadir)并按总分降序排名。不完整模型:total 为 null、rank 为 null,排在所有完整模型之后(不完整组内次级序沿用 model_id 字典序既有约定);涨跌为 null(无总分即无 delta,既有口径自然覆盖)。三个消费端点同口径:`GET /api/campaigns/{id}/report`、`GET /api/public/eval/board`、`GET /api/shared-reports/{token}`。API 契约:报告行新增完整性字段(如 `complete: bool` + 不完整维度计数,具体形状由 plan 影响分析定稿后进 api-contract.md)。全部模型不完整时榜单照常渲染(全体 rank null),不空态、不报错。W7「裁判失败不计 0 分」条款不动——维度分仍为 nil 而非 0,只是 nil 维度不再被静默剔除出资格判定。运行中批次(live 模式)语义不动:本门槛只对已 settle 批次生效。**TDD at W1**:黑盒用例先红后绿。

**Blocked by:** 无 — 可立即开工

**Status:** done(c362bed,2026-07-28;merge 423d4c6;契约冻结:`complete: bool` 仅 settle 批次行出现 + `missing_suites: number` 仅 complete=false 时出现,M=有启用 case 的 suite 数;TDD 3 条黑盒用例全绿,check 三维度 PASS;偏离登记:门槛分母=有启用 case 的 suite、baseline 批次侧同套门槛;遗留观察:一轮 internal/server 整包 FAIL 未复现,ticket 83 同类 flake)

## 验收清单

- [ ] 某模型一个维度全部未判分时:报告端点返回其 total=null、rank=null,排在全部完整模型之后(其余维度分数照常返回)
- [ ] 不完整模型不影响完整模型的名次(完整组排名与无不完整模型时一致)
- [ ] 多个不完整模型时组内按 model_id 字典序
- [ ] 全部模型不完整时端点正常返回(全体 rank/total null),不 500、不空数组
- [ ] 公开榜单端点与分享端点同口径(同一响应形状 + 同一排序语义)
- [ ] 涨跌字段:不完整模型为 null
- [ ] api-contract.md 同步登记新字段
- [ ] `make test` 全绿;check 三维度 PASS

## 风险登记

1. **契约是三处前端的依赖**:字段形状定稿即冻结,ticket 92 按此实现
2. **排序语义是 W7 相邻承重**:排序变更只动「不完整沉底」,完整组内部口径(降序/null 沉底/字典序)零改动
3. **live 模式不混入**:运行中批次的半成品语义(名次 `–`、字典序)与 settle 后门槛是两条路径,不得互相渗漏
