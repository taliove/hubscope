# 30 — 题库专业化:扩题 + Suite 版本化 + 采样

**What to build:** 让评估分数可信、趋势可比:Case 变为不可变——PATCH 修改题目时创建新 Case 并停用旧 Case(历史 Run 的旧题结果仍指向旧题),Case 增删/停用导致所属 Suite 版本号递增;Eval Run 记录所跑的 Suite 版本。内置 4 个 Suite 的种子题扩到每 Suite 10~20 题、难度分层(基础/中等/困难),规则题保确定性、裁判题考开放能力;种子仍只对空 Suite 生效,不回滚管理员改动。每 Case 可配采样次数(默认 1),得分取多次采样平均;设置页可配全局默认采样次数。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] Case 不可变:PATCH = 新 Case + 停用旧 Case;历史 Run 结果仍渲染旧题原文
- [ ] Suite 版本随 Case 增删/停用递增;eval_runs 记录 suite_version,经 API 可读
- [ ] 每 Suite 10~20 道种子题、难度分层;已有题目的 Suite 不被种子覆盖
- [ ] 采样次数可配(默认 1),N 次作答取平均;裁判失败该次记未判分,与现有口径一致
- [ ] 黑盒测试:版本递增与 Run 版本记录、采样平均、旧题结果可追溯
