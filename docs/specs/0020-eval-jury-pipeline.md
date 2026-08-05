# 0020 — 评估管线重构：刺探门控、模型登记表、三裁判中位数、三段异步管线

**Status：** accepted（2026-08-03，原型验证 + 用户四条决策确认）
**ADRs：** 0016（裁判团中位数判分 + 答案持久化，含承重墙四问）
**Prototype：** `internal/evaluator/prototype/`（`make proto-eval`，决策验证后按流程入 throwaway 分支留档）
**Tickets：** 见文末拆分草案（GitHub Issues 派发前以本文为准）

## 背景

现行评估（`internal/evaluator/`）四个结构性痛点：

1. **无前置实测**：一键触发直接开跑，模型不通要靠逐 case 失败 + 熔断才发现，烧调用换信息；预检只读状态板缓存（fail-open），无速度/稳定性概念。
2. **单裁判**：一个 judge_model 定全部 judge 型 case 的分，个体偏差与单点故障直接进分数；裁判不可达按 Hub 发生（GH #155），全局单值配置天然不匹配多 Hub。
3. **考试与判分熔合**：`evalSample` 内同步走完「答题→裁判」，慢裁判阻塞答题节奏，裁判与答题共用同一并发预算，崩溃即丢已付费答案。
4. **无成本与速度口径**：结果只有 latency/tokens 原始值，无 TPS、无费用估算，运营无法回答「这场评估花了多少钱、哪个模型快」。

## 决策总览

| 域 | 决策 | 依据 |
|---|---|---|
| 刺探 | 触发时先实测：k=3 轮 × 16 token，得通断/成功率/TPS/延迟；subject 不可达 → 跳过不烧题；结果进 task log，**不进 probes 表**（W5 口径不污染） | 用户决策 + 原型 |
| 登记表 | 内置静态模型特性表（IQ 档 + 输入/输出价 USD/1M），settings 覆盖项修正；未知模型 IQ/价格记 nil | 用户决策 |
| 裁判团 | 每 Hub 选 3 裁判，策略四选一（balanced/speed/iq/cost，默认 balanced）；备选 ≥3 时踢出被评模型；不足 3 个降级照跑 | 用户决策 1、2 |
| 判分 | 样本分 = 裁判票中位数（3→中位、2→均值、1→自身、0→null，W7）；case 分 = 样本中位数均值（沿用今 avg-of-samples）；口径升级为 Verdict Profile v3，断点照 ADR 0008 | 用户决策 1 |
| 管线 | 三段解耦：exam 池（答题）→ judge 池（每答案 3 票）→ aggregate（纯计算）；**答案先落库再入裁判队列**；崩溃后裁判队列从库重建 | 用户决策 3 |
| 覆盖 | judge 型 case 全量三裁判，不抽样；成本阀门 = 裁判策略、sample_count、case 设计 | 用户决策 4 |
| 成本 | 按调用时价格表逐调用估算，run 级累计（exam/judge 分项）；价格 nil → 显示「价格未登记」，不猜 | 本 spec |
| 速度 | TPS = output_tokens、latency（非流式口径，无 TTFT；流式另立 spec） | 本 spec |

## 刺探门控（Probe Gate）

- **时机**：一键触发与定时周批次均在执行前刺探；对象是本次被评模型 + 同 Hub 裁判候选（enabled chat 端点）。
- **形态**：每模型 k=3 轮串行小调用（16 output token，预算同 prober），模型间并行；产出 reachable、success_rate、avg_tps、avg_latency。
- **门控**：被评模型不可达 → 该模型 cell 跳过，task log 一条 warn，不产死行（沿用 GH #154 retired 先例形态）；全部被评模型不可达 → campaign 按 GH #153 all-dead 语义中止。
- **与状态板预检关系**：刺探是主动实测、优先；状态板 fail-open 预检保留为兜底（刺探调用本身异常时不误伤）。
- **隔离**：刺探结果只进 task log 与 run 执行上下文，**不写 probes 表、不驱动状态机/告警**（W5：eval 刺探语义 ≠ 监控探测）。

## 模型特性与费用登记表（Model Registry）

- 代码内置静态表：model ID 精确/前缀匹配 → `iq_tier`（1–10）、`price_in`、`price_out`（USD/1M tokens）。价格为公开牌价快照，随发版更新。
- settings `model_registry_overrides`（JSON）允许管理员修正价格、登记未知模型；覆盖优先于内置。
- 未知模型：`iq_tier = nil`（策略排序排尾）、`price = nil`（成本列显示「价格未登记」，绝不按 0 或猜测值计入）。
- 登记表是纯查询模块，无状态、无 I/O，供裁判团选择与成本估算两处复用。

## 裁判团（Jury）

- **选择**：逐 Hub 进行（GH #155 教训：裁判调用走被评模型的 Hub）。候选 = 该 Hub enabled chat 端点 ∩ 刺探 reachable。按策略加权归一化打分（IQ、速度、便宜度，权重随策略，balanced = 0.4/0.3/0.3，speed = 0.1/0.7/0.2，iq = 0.8/0.1/0.1，cost = 0.15/0.15/0.7），取前 3。settings `jury_policy` 默认 balanced。
- **自我裁判排除**：除 subject 外候选 ≥3 → subject 踢出自己的裁判团（自偏好偏差，原型实测 FINAL 0.568 → 0.627 虚高）；不足 3 个 → 允许入团 + task log warn 标注偏差风险。
- **降级**：裁判团 <3 照常执行，不拒跑（用户决策 1）；中位数规则随票数退化：3→中位、2→均值、1→自身、0→该样本 null（W7，裁判失败绝不计 0）。
- **快照**：`eval_runs` 新增 `jury_models` 列（JSON：策略 + 3 槽位模型 ID），替代 `judge_model` 单值语义；旧列保留只读兼容存量，新 Run 不再写入单值语义依赖。
- **口径**：裁判团中位数是新的判分尺子 → Verdict Profile 升 `v3`；存量 v1/v2 不回刷，趋势/告警断点照 ADR 0007/0008 机制。

## 三段异步管线

```
            exam 池(= eval_concurrency)        judge 池(= judge_concurrency)
cases ──► [exam 队列] ──答题──► eval_answers 落库 ──► [judge 队列] ──3 票──► eval_judge_scores 落库 ──► aggregate(中位数 → eval_results)
```

- **答案先落库**（用户决策 3）：每个 sample 答题成功即写 `eval_answers`，再入裁判队列；崩溃重启后扫描「答案在、票不全」的行重建裁判队列，已付费答案不丢。exam 段本身不恢复（run 失败语义同今）。
- **池与背压**：exam 池大小 = `eval_concurrency`；judge 池独立设置 `judge_concurrency`（默认同值，上限同 MaxEvalConcurrency）；队列有界，写库经 W2 单连接自然串行化。
- **取消与预算**：CancelCampaign（GH #152）停投喂两队列、in-flight 完成；campaign 预算（GH #153 guard）罩住两段，同点求值。
- **熔断**：exam 段保留每模型 5 连败熔断 + 全灭中止；裁判调用不重试、不熔断（W7），失败槽位记 null。
- **聚合**：样本 3 票齐（含 null）→ 中位数；sample 均值 → case 分写入 `eval_results`（对外行不变，verdict_detail 记逐裁判分）；新增 TPS、成本聚合到 run。
- **rule 型 case 不走裁判团**：归一化管道现状不变，零成本，直接出分进 `eval_results`。

## Schema 变更（ADR 0016，只加不删）

- `eval_runs`：`+ jury_models TEXT`（JSON，NULL = 旧单裁判 Run）、`+ estimated_cost REAL`（NULL = 含未登记价格）。
- 新表 `eval_answers`：run/model/case/sample_no/answer_text/latency_ms/input_tokens/output_tokens/status/created_at。
- 新表 `eval_judge_scores`：answer_id/slot/judge_model/score（NULL = 失败）/latency_ms/created_at。
- 迁移幂等，旧库无脑升级；存量 `eval_results` 不动。

## 成本与速度透出

- 每次 answer/judge 调用按当时登记表价格估算成本，run 级累计 exam/judge 分项；任一方价格 nil → `estimated_cost = NULL`，UI 显示「价格未登记」。
- TPS = output_tokens、latency_ms × 1000，按 run × model 聚合均值；报告 API 透出，与探测侧 latency 走势并列（口径注明：评估负载，非探测负载）。

## 报表与 UI（2026-08-03 原型评审：用户裁定 A+B+C 三视角全保留，不是三选一）

原型（`web/src/components/proto/`，？proto=A|B|C，throwaway 分支留档）产出三个结构性变体，用户裁定全部采纳，各居其位：

- **A「管线指挥舱」→ 运行中批次视图**：四节点流水线（刺探门控→考试池→裁判池→中位数聚合）带队列深度 + 模型监控表（刺探/考试进度/裁判进度/中位数分/TPS/成本）+ 任务日志流。定位为批次运行中的监控面，与 Progress Grid 的关系在 ticket 8 设计评审时裁定（替换或并存于「评估运营」tab）。
- **B「裁判席」→ 判分质量审计面**：左栏裁判团卡片（策略 + 选择理由条形 + 各裁判成本/失败数），主体逐 case 表（3 裁判分列 + 中位数 + spread 分歧高亮 >0.12 标红）。定位为 Run/case 级详情视图，从报告与运行监控两处下钻可达。
- **C「账本」→ 成本与性价比面**：KPI 条（总成本/exam/judge 分项/平均 TPS/未登记数）+ 性价比榜（每分成本排序，价格未登记显式标注）。定位为批次报告新区块（settle 后）与运营复盘入口。

- 模型行：平均 TPS、估算成本（exam/judge 分项）；半成品榜单口径不变，刺探/队列信息进 task log。
- 前端改动过 design-owner 评审后回写 ui-guidelines；原型代码不直接进生产（ticket 8 重写）。

## Testing Decisions

沿用唯一接缝（HTTP API 黑盒 + stub Hub + 假时钟 + 真 SQLite；异步排空走 WithSyncEval 族结构同步点，禁 sleep）。核心断言：

- **刺探**：stub Hub 拒答 → subject 被跳过、无死行、task log 有记录；全灭 → campaign 中止；刺探结果不进 probes 表（状态机不受影响）。
- **登记表**：内置匹配正确；覆盖项优先；未知模型价格 nil → 成本 NULL。
- **裁判团**：四策略选择结果符合权重预期；备选 ≥3 踢出 subject；不足 3 个降级 + warn；逐 Hub 隔离（Hub A 的裁判不服务 Hub B）。
- **中位数**：3 票中位、2 票均值、1 票自身、0 票 null；裁判失败不计 0（W7）；结果 profile = v3，趋势断点出现。
- **管线**：答案先落库后判；杀进程（模拟崩溃）后裁判队列重建、已判票不重判；取消停投喂、in-flight 完成；预算罩两段；熔断只在 exam 段。
- **隔离**：新 list 接口按 hub_id 过滤，登记 `isolation_test.go` sweep。

## Ticket 拆分草案（依赖序）

| # | 内容 | Blocked by |
|---|---|---|
| 1 | 模型登记表模块 + `model_registry_overrides` 设置项 | — |
| 2 | Schema 迁移：`jury_models`/`estimated_cost` + `eval_answers` + `eval_judge_scores`（ADR 0016 四问） | — |
| 3 | 刺探阶段 + 门控（含 task log、全灭中止） | — |
| 4 | 裁判团选择器（四策略/自我排除/降级/快照写入） | 1、3 |
| 5 | 三段管线重构（exam/judge 池、背压、取消/预算、崩溃恢复） | 2、4 |
| 6 | 中位数聚合 + Verdict Profile v3 + `eval_results` 写入切换 | 5 |
| 7 | 报告 API 透出（逐裁判分、TPS、成本分项） | 6 |
| 8 | 前端：逐裁判分值展示、成本/速度列（design-owner 评审） | 7 |
| 9 | 测试线验证 + 口径断点回归 + 原型入 throwaway 分支留档 | 8 |
