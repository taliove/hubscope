# 94 — 权威题库管线首发:MMLU 知识套件端到端 + W7 题库来源 ADR

**What to build:** 按 spec 0014 实现决策 C 的 tracer bullet——用 MMLU(知识维度)一次打穿整条权威题库管线,为 95–98 建立同构模板。行为:① 新增 MCQ 判分——选项字母提取 + 精确匹配,走既有 rule verdict 分派接缝扩展新 RuleMode,ADR 0008 归一化 profile 配套(大小写/空白/「答案是 X」类前缀归一),纯规则零 LLM 裁判;② 题库数据交付机制——子集为 JSONL 数据文件经 `go:embed` 进二进制,generation 幂等 seed(沿用 seedbank 机制:不重复铸题、不覆盖管理员编辑),Case 的 prompt/expected 从数据文件铸入;③ MMLU 100 题冻结子集——按学科分层比例抽取,子集清单固化进数据文件,一经铸入不可变(改子集=退役+铸新,W7);④ sample_count=1;⑤ license 核对与署名机制——数据集来源、license、署名要求登记入仓(随数据文件同目录的 ATTRIBUTION),MMLU license 核对结论写入本票备注;⑥ 端到端可跑:创建 campaign 含 MMLU 套件,stub Hub 返回精心构造的模型输出,断言判分正确(正例命中/反例不命中/归一化边界)。同时提交 **ADR:权威题库替换自造题库(W7 框架内的题库来源决策)**。**TDD at W1**。

**Blocked by:** 无 — 可立即开工

**Status:** ready-for-agent

## 验收清单

- [ ] MCQ 判分:stub Hub 输出「B」「答案是 b」「The answer is B.」等变体均判对;错选项/无选项输出判错;全程零 LLM 裁判调用
- [ ] 数据文件 go:embed 进二进制,`store.Open` seed 后 MMLU 套件(capability=knowledge)含 100 题、sample_count=1、学科分层比例可见于子集清单
- [ ] seed 幂等:重复 Open 不重复铸题;管理员编辑过的 Case 不被覆盖(既有 generation 语义)
- [ ] campaign 端到端:含 MMLU 套件的批次跑通,报告出现知识维度分数
- [ ] ATTRIBUTION 文件登记来源与 license;MMLU license 允许随二进制再分发子集的结论入票备注
- [ ] ADR 落盘 docs/adr/;`make test` 全绿;check 三维度 PASS

## 风险登记

1. **子集抽取的可复现性**:分层抽样用固定种子/固定清单,抽一次固化进数据文件——运行时不做任何随机抽取
2. **归一化边界**:MCQ 提取规则宁可保守(提取不到=判错=未命中),不做模糊猜测;边界 case 全进测试
3. **本票是 95–98 的模板**:数据文件布局、seed 机制、ATTRIBUTION 格式、判分器接入方式定稿后,后续套件同构复制
4. **MMLU 题面版权**:学术 benchmark 再分发一般允许但须署名;若核对结论不允许,升级回报用户换知识维度来源(C-Eval/ARC),不擅自替换
