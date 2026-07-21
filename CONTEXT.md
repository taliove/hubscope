# AI Hub Checker

对内监控网站:定期对 AI HUB(模型网关)上接入的所有模型做可用性探测与质量评估。

## Language

**Hub**:
公司自建的 AI 模型网关(当前实例 `https://ai-claude-code-hub.jetmobo.com`),向上游多家供应商转发请求,对外同时暴露 Anthropic 与 OpenAI 两套 API。Hub 实例(base URL、凭证)在管理后台维护、存于数据库,可配多个;配置文件不含 Hub 信息。
_Avoid_: AI HUB 平台、网关、中转站(口头可用,文档与代码中统一用 Hub)

**Model**:
Hub 上可被调用的一个模型,以其模型 ID 标识(如 `claude-opus-4-8`、`kimi-k3`)。主要来源于 Hub 的 `/v1/models` 列表,也允许手工登记列表之外的 ID(如带 `[1M]` 后缀的变体)。
_Avoid_: 渠道、供应商(那是 Hub 上游的概念,不属于本系统)

**Protocol**:
调用 Hub 时使用的 API 协议,取值 `anthropic`(`POST /v1/messages`)或 `openai`(`POST /v1/chat/completions`)。同一 Model 在两种 Protocol 下的可用性可能不同。

**Endpoint**:
一个 (Model × Protocol) 组合,是可用性监控的最小单位,各自独立出状态、独立统计指标。
_Avoid_: 接口、路由

**Probe**:
对单个 Endpoint 执行的一轮可用性检查,包含一次非流式请求和一次流式请求。记录成败、HTTP 状态码、错误信息、总延迟、TTFT、token 用量。
_Avoid_: 打点、探活(口头可用,代码与文档统一用 Probe)

## Language — 评估

**Suite**:
一个能力评估套件,按维度组织(基础/指令遵循、推理/数学、代码能力、中文能力),内含一组 Case,是评估执行与结果聚合的单位。
_Avoid_: 题库、 benchmark(口头可用)

**Case**:
Suite 中的一道评估题,包含 prompt、判定方式与期望标准。一次作答产出 0~1 分。
_Avoid_: 题目、样本

**Verdict**:
对 Case 作答的判定,两种方式:规则判定(精确匹配/正则/包含)或 LLM 裁判按 rubric 打分(裁判模型可配置)。
_Avoid_: 判分、批改

**Eval Run**:
对选定 Model 集合执行一个(或多个)Suite 的一轮评估,每周定时全量一次,也可手动触发;产出按 Suite/维度聚合的分数。
_Avoid_: 评估任务、跑分
