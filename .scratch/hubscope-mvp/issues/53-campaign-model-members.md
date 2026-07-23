# 53 — Campaign 模型成员表(进度网格全量模型)

**What to build:** ticket 52 遗留:进度网格的模型成员推导自 eval_results,运行中批次里尚未产出首个结果的模型暂时不进网格。建 campaign ↔ model 成员表(Campaign 创建时快照应跑模型全集),网格从首个 run 排队起即展示全量模型的「等待中」状态。触 W2(schema 迁移)与 W3(模型口径),开工前书面回答承重墙四问。

**Blocked by:** None — can start immediately

**Status:** pending

- [ ] Campaign 创建时落模型成员快照(与一键全量/手动单跑两路径同语义)
- [ ] 进度网格从批次开始即显示全量模型(未跑到 = 等待中)
- [ ] 迁移幂等,存量 Campaign 成员可从既有 run/结果回填
- [ ] 黑盒测试覆盖(HTTP 接缝)
