# 98 — CRUXEval 代码套件(输出预测 + 离线答案预计算)

**What to build:** 按 spec 0014 决策 C,落地代码维度——免执行形态。行为:① **离线答案预计算工具脚本**(一次性,不进交付物,不进二进制)——对 CRUXEval 题目在开发机执行标准函数得到「输入→输出」标准答案,结果固化进数据文件;脚本可复跑、产出确定性一致;② 判分——模型预测输出与标准答案精确匹配(ADR 0008 归一化 profile:首尾空白/引号风格/换行归一,列表/字典/字符串等 Python 字面量形态归一),纯规则零 LLM 裁判;③ 100 题随机子集(固定清单),sample_count=1;④ 数据文件 + seed + ATTRIBUTION 复用 94 机制;⑤ 端到端:campaign 含 CRUXEval 套件(capability=coding)跑通,报告出现代码维度分数;**运行时零沙箱、零 Python 依赖**(W8 与安全边界不动,执行只发生在离线制题时)。**TDD at W1**。

**Blocked by:** 94(权威题库管线首发:MMLU 知识套件端到端)

**Status:** done(2026-07-28;CRUXEval license 核对结论:MIT,允许随二进制再分发子集,署名与许可声明已随数据文件入仓 internal/store/benchmark/cruxeval/ATTRIBUTION.md;来源 github.com/facebookresearch/cruxeval @ 190faf16(官方仓库为权威源,HuggingFace 镜像未取用),数据集 SHA-256 8368b810…0c96;子集固定步长 rows[::8] 零随机,100 题,SHA-256 6bfe34e5…0e94 留痕;离线制题脚本 scripts/cruxeval_subset.py(开发机制题工具,不进交付物/不进二进制,用法 python3 scripts/cruxeval_subset.py [--input 本地副本],PYTHONHASHSEED=0 自 re-exec,复跑字节级一致,800/800 行执行验证通过);运行时无任何代码执行路径(check 审查确认:internal/ 与 cmd/ 无 os/exec/plugin/Python 调用,判分为纯字面量解析比较);check 三维度 PASS,MEDIUM-1 元组分组口径已随票修复)

## 验收清单

- [x] 离线预计算脚本产出 100 题标准答案并固化进数据文件;脚本可复跑产出一致;脚本路径与用法入票备注,不进交付物
- [x] 输出匹配判分:正确答案(含空白/引号/换行变体)判对;错值/非字面量输出判错;Python 字面量归一边界入测试
- [x] CRUXEval 套件(capability=coding)seed 后含 100 题、sample_count=1
- [x] campaign 端到端跑通,报告出现代码维度分数;运行时无任何代码执行路径(审查确认)
- [x] ATTRIBUTION 增补 CRUXEval;license 核对结论入票备注
- [x] `make test` 全绿;check 三维度 PASS

## 风险登记

1. **字面量归一化是判分口径核心**:Python repr 形态多样(`'a'` vs `"a"`、`[1, 2]` vs `[1,2]`),归一规则保守且全量入测试;提取不到合法字面量=判错
2. **prompt 模板要求模型只输出预测值**(不含解释/代码块),模板随 Case 铸入冻结,与官方评测口径一致
3. **离线脚本的执行安全**:只在开发机跑可信的标准答案函数,与生产运行时严格隔离
