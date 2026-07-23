# 53 — Campaign 模型成员表(进度网格全量模型)

**What to build:** ticket 52 遗留:进度网格的模型成员推导自 eval_results,运行中批次里尚未产出首个结果的模型暂时不进网格。建 campaign ↔ model 成员表(Campaign 创建时快照应跑模型全集),网格从首个 run 排队起即展示全量模型的「等待中」状态。触 W2(schema 迁移)与 W3(模型口径),开工前书面回答承重墙四问。

**Blocked by:** None — can start immediately

**Status:** done

- [x] Campaign 创建时落模型成员快照(与一键全量/手动单跑两路径同语义)
- [x] 进度网格从批次开始即显示全量模型(未跑到 = 等待中)
- [x] 迁移幂等,存量 Campaign 成员可从既有 run/结果回填
- [x] 黑盒测试覆盖(HTTP 接缝)

## 承重墙四问(W2/W3,2026-07-23 实现时回答)

1. **为什么必须改 W2:** run 不携带模型成员信息,网格成员只能从 eval_results 推导,未跑到的模型在运行中批次里完全不可见——操作者无法区分「还没跑到」与「不在本批」。读端临时推导会把现在时模型状态混进历史批次视图(批次后新增模型假性出现、删除/禁用模型假性消失),只有创建时快照表能记录真实应跑全集。
2. **影响哪些调用方:** `CreateCampaign` 全部 4 个调用方(一键全量、手动单跑、周批次、存量库 staging 测试);读侧 `liveReportRows`/`writeCampaignReport`;`store.Open` 迁移路径。
3. **替代方案:** (a) 继续 results 推导 + union runs——不可能,run 无模型维度;(b) 读时现算 ListActiveChatModelIDs——非历史快照,手动单跑的选择不可恢复;(c) 成员冗余存每个 eval_run——同一事实 N 份拷贝。选定:(campaign_id, model_id) 成员表,与建 Campaign 同事务写入。
4. **回归测试:** 功能层 = 4 个新黑盒测试 + 改写 1 个前提失效旧测试;关联层 = internal/server 全量;闭环层 = `go test ./internal/...` 全绿。
