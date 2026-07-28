# 96 — 中文套件:CMMLU/C-Eval 选型与落地

**What to build:** 按 spec 0014 决策 C,落地中文能力维度。行为:① 选型定夺——对 CMMLU 与 C-Eval 做数据质量核验(题量、学科覆盖、答案完整性、license),二选一,结论与理由入票备注;② 复用 ticket 94 的 MCQ 判分(中文卷同样是选项字母提取,注意全角/半角与中文回答惯用语归一,如「答案是 B」「选 B」);③ 100 题按学科分层子集,sample_count=1;④ 数据文件 + seed + ATTRIBUTION 复用 94 机制;⑤ 端到端:campaign 含中文套件(capability=language)跑通,报告出现中文维度分数。**TDD at W1**。

**Blocked by:** 94(权威题库管线首发:MMLU 知识套件端到端)

**Status:** ready-for-agent

## 验收清单

- [ ] 选型结论(CMMLU 或 C-Eval)+ 数据质量核验记录 + license 核对结论入票备注;license 不允许再分发则升级回报用户,不擅自换第三个来源
- [ ] 中文 MCQ 判分:全角/半角字母、「选 B」「答案是 B」等中文惯用语变体判对;错选判错
- [ ] 中文套件(capability=language)seed 后含 100 题、sample_count=1,学科分层
- [ ] campaign 端到端跑通,报告出现中文维度分数
- [ ] ATTRIBUTION 增补;`make test` 全绿;check 三维度 PASS

## 风险登记

1. **维度词表不变**:capability=language 沿用 ADR 0010 既有维度,词表「中文能力」不动
2. **中文归一化**:判分归一化 profile 与英文 MCQ 分开配置(全角字符、中文标点),不混用
