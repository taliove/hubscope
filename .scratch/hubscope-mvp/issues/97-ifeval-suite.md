# 97 — IFEval 指令遵循套件(程序化校验器库 + Case 校验参数列)

**What to build:** 按 spec 0014 决策 C,落地指令遵循维度——五套件中工程量最大的一票。行为:① **Case 表新增 JSON 校验参数列**(W2 纯新增列,符合只加不删)——IFEval 每题携带结构化校验参数(instruction 类型 id + 参数,如「关键词:出现词 X」「长度:≥N 句」「格式:JSON 输出」);② **IFEval 官方校验器的确定性移植**——按官方 verification 函数逐类实现(关键词存在/频率、长度约束、格式约束、大小写约束、标点约束、起始/结尾短语等,范围以 100 题子集实际用到的 instruction 类型为准,不做全量移植),全部校验通过=对,任一失败=错,零 LLM 裁判;③ 100 题按 instruction 类型分层子集,sample_count=1;④ 数据文件 + seed + ATTRIBUTION 复用 94 机制;⑤ 端到端:campaign 含 IFEval 套件(capability=instruction)跑通,报告出现指令遵循维度分数。**TDD at W1**:stub Hub 输出逐类校验器的命中/不命中用例。

**Blocked by:** 94(权威题库管线首发:MMLU 知识套件端到端)

**Status:** ready-for-agent

## 验收清单

- [ ] Case 表新增 JSON 校验参数列,迁移安全(既有库无脑升级,旧数据列空值不影响)
- [ ] 子集覆盖的每类 instruction 校验器:命中/不命中正反例经 eval run 端到端断言
- [ ] 一题多指令时全部通过才判对(任一失败即错)
- [ ] IFEval 套件(capability=instruction)seed 后含 100 题、sample_count=1,instruction 类型分层
- [ ] campaign 端到端跑通,报告出现指令遵循维度分数
- [ ] ATTRIBUTION 增补 IFEval(Apache-2.0 核对);`make test` 全绿;check 三维度 PASS

## 风险登记

1. **移植保真**:校验器语义以官方实现为准逐类核对(官方 Python 代码为参照),移植偏差=榜单口径偏差;每类校验器备注官方出处
2. **子集驱动范围**:只移植子集实际用到的 instruction 类型,未覆盖类型在数据层不得出现(seed 时校验,出现即报错 fail-closed)
3. **schema 变更走 database skill**:新增列迁移 + 保留字核查 + 单连接约束
