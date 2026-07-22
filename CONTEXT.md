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
对选定 Model 集合执行一个 Suite 的一轮评估,异步执行、状态落库;产出按 Suite/维度聚合的分数。手动触发或定时触发均产生 Run。
_Avoid_: 评估任务、跑分

**Eval Campaign**:
一次"考核批次",将同一轮考核下的一组 Eval Run(通常每 Suite 一个)聚合为一个整体,是报告与排行榜的单位。一键全量评估(所有 chat Model × 所有 Suite)产生一个 Campaign;手动单跑也归入一个 Campaign。
_Avoid_: 批次任务、考核任务

**Task**:
一个有明确起止的后台作业,是任务中心的条目。类型包括 Eval Run、发现同步、rollup 清理等;统一状态机(pending/running/success/failed),带逐行日志(Task Log)。高频探测轮次不是 Task。
_Avoid_: 作业、job(代码中可用 task)

**Leaderboard**:
一个 Campaign 的排行榜:每个 Model 一行,含总分(各 Suite 分的加权平均,0–100)与各 Suite 分,按总分排序;可按模型 category 过滤。已删除 Model 不出现在 Leaderboard,但其历史数据保留并在趋势视图中带「已删除」标记。
_Avoid_: 榜单、排名表

**Report**:
一个 Campaign 的完整报告:Leaderboard + 每 Model 跨 Campaign 趋势 + 探测侧延迟/成功率走势。Report 可生成 Share Link 对外只读分享,可导出图片/PDF。
_Avoid_: 报表、总结页

**Share Link**:
指向某个 Report 的只读链接,带随机 token,无需登录即可打开;可撤销。
_Avoid_: 公开链接、外部分享

**Suite Version**:
Suite 的题集版本。Case 不可变,改题 = 新增 Case + 停用旧 Case,Suite 版本号随之递增;Eval Run 记录所跑的 Suite Version,跨版本的趋势比较标注版本断点。
_Avoid_: 题库快照
