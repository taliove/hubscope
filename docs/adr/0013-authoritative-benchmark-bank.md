# 权威题库替换自造题库(W7 框架内的题库来源决策)

**Status:** accepted(2026-07-28,spec 0014 决策 C,ticket 94 随 MMLU 管线首发落地)

## 决策

题库来源从自造题目切换为公开权威 benchmark 的冻结子集,由 HubScope 自己的 harness 经 Hub 端点执行。W7 的不可变性与绝对分制不动:子集一经铸入不可变,改子集 = 退役旧 Suite + 铸新 Suite;全部规则判分,零 LLM 裁判。

**形态(以 MMLU 为模板,ticket 95–98 同构复制):**

- **数据交付:** 子集为 JSONL 数据文件(每行一题:id/subject/question/choices/answer),经 `go:embed` 进单二进制(`internal/store/benchmark/<name>/subset.jsonl`),同目录 ATTRIBUTION 文件登记来源 URL、精确 revision(commit hash)、license 结论与署名、子集抽取方法与种子/步长——运行时零随机、零外部依赖(离线可用,W8)。
- **来源留痕:** 抽取自官方源 pinned revision,双通道(官方 parquet + 官方 datasets-server JSON)逐行互验后固化;数据文件记 SHA-256。禁止凭记忆编造题目——防作假根基是数据真实性。
- **seed:** 复用 seedbank generation 幂等机制(重复 Open 不重复铸题、不覆盖管理员编辑)。新套件经 `retireAtGen: 1` **铸入即停用**:ticket 99 统一切换前不进全量 sweep 与周批次的轮换,手动触发与管理员启用可用,启用状态跨重启粘滞(generation 语义保证不重蹈停用)。
- **判分:** 既有 `rule` verdict 分派接缝扩展新 RuleMode。MMLU 用 `mcq`——选项字母提取 + 精确匹配:答案先过 ADR 0008 v2 归一化管道(NFKC 折叠全角字母),再从「B」「答案是 b」「The answer is B.」「(B)」「选B」等措辞中提取字母;**保守原则**——提取不到或提取到冲突字母一律判 0,宁可误判为答错也不猜。提取失败仍是已判分(score 0),不是未判分(nil)——模型确实作答了,只有作答调用失败才未判分(W7「裁判失败不计 0 分」条款不受影响:规则判分没有裁判失败路径)。
- **判分口径版本:** mcq 是新 RuleMode,不改变 exact/contains/regex 的既有管道行为,历史 Run 的尺子未动,故 VerdictProfile 维持 v2、结果照常记录 v2;新增 profile 版本无收益只有断点成本。
- **prompt 模板:** 要求模型只回复选项字母的英文模板,随 Case 铸入即冻结(改模板 = 退役 + 铸新)。
- **采样:** sample_count=1,题量(100 题)替代重复采样。
- **nadir:** 暂按题型既有口径置值(四选一 MCQ 随机猜测下限 0.25),ticket 99 按测试线实测分布统一校准。
- **维度映射(ADR 0010 五 capability 不变):** 知识→MMLU(mcq)、推理→GSM8K、指令遵循→IFEval、中文→CMMLU/C-Eval、代码→CRUXEval 式输出预测——后四者各配自己的 RuleMode/校验器,ticket 95–98 落地。

## 理由与代价

自造题库缺乏公信力,且 LLM 裁判大面积失败使判分确定性本身成为可信度瓶颈(spec 0014 背景)。权威 benchmark + 确定性规则判分一次解决两者。已接受的 trade-off:公开题库的 contamination 风险(分数系统性偏高)换权威性与免维护;旧 v3 套件在 ticket 99 切换时停用并按「停用套件硬删」决策删除,切换批次涨跌列由既有断点机制显示「题目已变更」,不做新旧分数桥接(绝对分制下口径不同即不可比,ADR 0007 既有语义)。
