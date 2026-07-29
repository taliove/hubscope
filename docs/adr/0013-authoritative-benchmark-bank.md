# 权威题库替换自造题库(W7 框架内的题库来源决策)

**Status:** accepted(2026-07-28,spec 0014 决策 C,ticket 94 随 MMLU 管线首发落地);cutover 已执行(2026-07-29,GH #15,合并提交 dca691d + 中途态兜底增量,见文末附记)

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
- **维度映射(ADR 0010 五 capability 不变):** 知识→MMLU(mcq)、推理→GSM8K、指令遵循→IFEval、中文→AGIEval 高考中文子集(mcq,见下方附记)、代码→CRUXEval 式输出预测——后四者各配自己的 RuleMode/校验器,ticket 95–98 落地。

## 附记:中文套件选型(ticket 96,2026-07-28)

原计划的 CMMLU 与 C-Eval 经双通道(HuggingFace 数据集卡 + GitHub 官方仓库)核验,数据 license 均为 **CC BY-NC-SA 4.0**(CMMLU GitHub README 原文、C-Eval `LICENSE-DATA`),NonCommercial 条款不允许随 MIT 授权的 HubScope 二进制再分发,用户裁决后转向第三来源。**AGIEval**(microsoft/AGIEval 退役后的正本 github.com/ruixiangcui/AGIEval,commit `84ab72d9`)的高考中文子集(gaokao-chinese/history/geography)为 **MIT**(data/v1_1/LICENSE 首部「Gaokao, SAT MIT License, Copyright (c) Microsoft Corporation」),题量 680 题、答案完整公开、全部四选一单答案,与 lm-eval-harness HF 镜像(hails/agieval-gaokao-*)逐行互验一致;logiqa-zh(CC BY-NC-SA 4.0)与 jec-qa(仅限学术研究、禁止商用)按 license 排除。套件 key `agieval_zh`,判分复用 mcq(其提取模式已覆盖中文惯用语与全角字母,口径与英文套件同一把尺子),prompt 模板换为中文并随 Case 冻结。

## 理由与代价

自造题库缺乏公信力,且 LLM 裁判大面积失败使判分确定性本身成为可信度瓶颈(spec 0014 背景)。权威 benchmark + 确定性规则判分一次解决两者。已接受的 trade-off:公开题库的 contamination 风险(分数系统性偏高)换权威性与免维护;旧 v3 套件在 ticket 99 切换时停用并按「停用套件硬删」决策删除,切换批次涨跌列由既有断点机制显示「题目已变更」,不做新旧分数桥接(绝对分制下口径不同即不可比,ADR 0007 既有语义)。

## 附记:cutover 执行与已知问题(GH #15,2026-07-29)

**执行方式(ticket 99 phase 1,合并提交 dca691d):** 切换由三个同 Open 内依次执行的机制完成——① seedbank 的 benchmark 条目 `retireAtGen` 从 1 改为 0(新库铸入即启用,五套件构成全部轮换);② v3 capability 套件带 `retireAtGen: 4`(高于题库最高 case generation 3,凡收过 v3 seed 的库都会触发代际退役),同 Open 的停用套件 purge(ADR 0012)连带 cases/runs/results 级联删除并铸墓碑防重铸;③ 一次性 settings 键 `benchmark_cutover` 记录的启用迁移(seed 机制只有退役路径没有启用路径,94–98 时代已停用铸入 benchmark 套件的库需要翻转启用;一次性设计使管理员 cutover 后停用某套件时该迁移不复活它,交给 purge)。零套件轮换窗口(切换瞬间全部套件不可用的边缘)由 `SettleEmptyCampaign` 降级为空 done 批次,不触发 zero-run⇒failed 聚合规则。

**中途态兜底(同票增量):** dev 库现实中途经处于「`purged_suite_cap_*` 墓碑已写 + cap_* 行存活且 enabled=1 + `benchmark_cutover=1` 已写」——seedSuites 遇墓碑跳过 v3 bank,代际退役(retireAtGen 4)永不触发,purge 只删 enabled=0,v3 套件将永远存活。兜底 = `retireV3SuitesAtOpen`:每次 Open 无条件把 capabilitySuites bank 常量列出的 v3 键 UPDATE enabled=0(幂等,稳态匹配零行),**不被一次性 settings 键门控**(该键记录的是启用迁移,不是 v3 退役,中途态下它已写),交同 Open 的 purge 级联删除。回归:`TestBenchmarkCutoverMidState`(构造中途态 → 重开收敛)、`TestBenchmarkCutoverIdempotent`(三次 Open 终态不变 + 管理员停用即 purge 不复活)。

**gsm8k 顶格已知问题(登记,不阻塞):** 实测主力模型 gsm8k 原始分 0.96–0.99,100 题冻结子集对强模型区分度不足(顶格效应)。已裁决:nadir 与 80/50 档色阈值均不动,不做校准迁移——分数口径变动 = W7 题库变更,不能静默迁移。后续按 W7 既定路径处理:退役现 gsm8k Suite + 铸新 Suite(更难的子集或更严判分口径),涨跌列由既有断点机制显示「题目已变更」,不做新旧桥接(ADR 0007 语义)。
