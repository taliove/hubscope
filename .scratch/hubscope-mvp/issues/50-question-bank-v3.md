# 50 — 题库 v3:能力点重构与轮换机制

**What to build:** 按 ADR 0010 / spec 0004 重构题库:Suite 按能力点(Capability)组织,取消「中文」语言维度;每能力点 30–50 题、分 basic/intermediate/hard 三档,每题声明能力点、verdict 方式与 nadir 标定依据;新题型:结构化抽取、多步推理、代码理解、长文本摘要(全落现有 rule/judge 判分器,不做多轮/工具调用)。seed 走 gen 3(旧题停用不删、不覆盖管理员编辑,沿用 seed_gen 幂等机制);judge 题 sampleCount=3、rule 题保持 1,judge 占比 ≤40%。轮换机制:季度退役+铸新约 30%,轮换即 suite_version 递增。管理台题库 tab 支持按能力点浏览与审题流转(候选→审→入库)。能力点清单与权重开票时随设计评审定稿。承重墙关联:W7——四问已在 ADR 0010 书面回答。

**Blocked by:** None — can start immediately(题目内容生产可与 49 并行)

**Status:** pending(机制部分已完成 2026-07-23:能力点 Suite + gen 3 seed + nadir 存储 + 首发题 + 管理台能力点浏览;题目大批量生产与审题流转待续)

- [ ] 能力点清单与权重定稿(过 design-owner + 评审)——机制已按五能力点(instruction/reasoning/coding/language/knowledge)落地,权重仍等权,定稿待评审
- [x] gen 3 seed 幂等入库:旧题停用不删、管理员编辑不被回滚
- [ ] 每能力点 30–50 题、三档难度、每题带能力点与 nadir 依据——已落地每能力点 10 道 LLM 生成首发题(seed 注释标记 pending human review),30–50 题人工审题生产待续
- [x] judge 题 sampleCount=3 取均值,rule 题保持 1
- [ ] 能力点维度聚合正确(ticket 51 评分侧);轮换(停用+铸新)使版本递增,历史 Run 渲染旧题原文——后者已完成
- [ ] 题库 tab 按能力点浏览 + 审题流转——按能力点筛选浏览已完成;审题流转(候选→审→入库)待续
- [ ] 黑盒测试覆盖上述全部——机制部分全覆盖(seed 幂等/旧 Suite 停用/能力点与 nadir/judge 采样 3 次/能力点过滤/轮换版本递增/全量评估排除停用 Suite)
