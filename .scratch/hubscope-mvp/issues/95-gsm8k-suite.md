# 95 — GSM8K 推理套件(数值提取判分)

**What to build:** 按 spec 0014 决策 C,在 ticket 94 打穿的管线上落地推理维度。行为:新增 GSM8K 判分——从模型输出提取最终数值(优先「#### N」标记,其次末行最后一个数字;千分位逗号/负号/小数归一)与标准答案精确匹配,走 rule verdict 分派接缝,纯规则零 LLM 裁判;100 题随机子集(固定清单固化),sample_count=1;数据文件 + seed + ATTRIBUTION 全部复用 94 的机制;license 核对结论入票备注。端到端:campaign 含 GSM8K 套件(capability=reasoning)可跑通,报告出现推理维度分数。**TDD at W1**:stub Hub 输出正例(「#### 42」、末行数字)、反例(错误数值、无数值)断言判分。

**Blocked by:** 94(权威题库管线首发:MMLU 知识套件端到端)

**Status:** done(90c0d59+ce0212e+90135ca,2026-07-28;merge 344c7fb;GSM8K license 核对结论:MIT,来源 openai/gsm8k @ 740312ad,parquet 与 datasets-server 双通道互验一致;固定步长零随机子集,SHA-256 留痕,ATTRIBUTION 入仓;numeric RuleMode + 8 条黑盒用例全绿)

## 验收清单

- [ ] 数值提取判分:带「#### N」标记与纯末行数字输出均判对;错值/无数值判错;千分位与小数归一边界入测试
- [ ] GSM8K 套件(capability=reasoning)seed 后含 100 题、sample_count=1,子集清单固化
- [ ] campaign 端到端跑通,报告出现推理维度分数
- [ ] ATTRIBUTION 增补 GSM8K;license 核对结论入票备注
- [ ] `make test` 全绿;check 三维度 PASS

## 风险登记

1. **提取规则保守原则**:提取不到=判错,不做语义猜测;GSM8K 标准解本身带「####」标记,prompt 模板须要求模型同格式输出(题面 prompt 含格式指令,与官方评测口径一致)
2. **prompt 模板是判分口径的一部分**:模板随 Case 铸入即冻结,改模板=退役+铸新
