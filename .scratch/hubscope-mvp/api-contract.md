# API Contract — Ticket 01 (前后端共同遵守,不得偏离)

Base path: `/api`。所有响应 JSON。成功:`{"data": ...}`;失败:非 2xx 状态码 + `{"error": {"message": "..."}}`。

## Auth(ticket 07)

- `POST /api/auth/login` → `{"data":{"authenticated":true}}`,Set-Cookie session。Body `{"password": string}`;口令错误 → 401。
- `POST /api/auth/logout` → 204,清 session。
- `GET /api/auth/me` → `{"data":{"authenticated": boolean}}`(公开)。
- 口令来自 env `ADMIN_PASSWORD`,启动缺失则拒绝启动。除 `/api/auth/*` 外的**写方法**(POST/PUT/PATCH/DELETE)必须带有效 session,否则 401;GET 全部公开。session 用签名 cookie 实现,无需服务端存储。


## Hubs

- `POST /api/hubs` → 201。Body: `{"name": string, "base_url": string, "token": string}`
- `GET /api/hubs` → `{"data": [Hub]}`
- `PUT /api/hubs/{id}` → `{"data": Hub}`。Body 字段同 POST,省略表示不改;`token` 省略则不更新。
- `DELETE /api/hubs/{id}` → 204;若该 Hub 下已有 Model,返回 409。

`Hub = {"id": number, "name": string, "base_url": string, "token_hint": string, "created_at": string(RFC3339)}`
**任何响应都不含 token 明文**;`token_hint` 仅为末 4 位,如 `"…f0f0"`(token 不足 4 位时为 `"…"`)。

## Models

- `POST /api/models` → 201 `{"data": Model}`。Body: `{"hub_id": number, "model_id": string}`。服务端自动创建两条 Endpoint(anthropic + openai,均 enabled)。同一 Hub 下 model_id 重复 → 409。
- `GET /api/models` → `{"data": [Model]}`

`Model = {"id": number, "hub_id": number, "model_id": string, "origin": "manual", "status": "active", "capability": "chat", "endpoints": [Endpoint]}`
`Endpoint = {"id": number, "model_id": number, "protocol": "anthropic"|"openai", "enabled": boolean, "interval_seconds": number|null}`(`interval_seconds` 为 null 表示用全局默认 300)

## Endpoints

- `PATCH /api/endpoints/{id}` → `{"data": Endpoint}`。Body 字段均可选:`{"enabled": boolean, "interval_seconds": number|null}`——`interval_seconds` 传数字设置覆盖(最小 60),传 null 清除覆盖回退默认。下一轮调度即生效。id 不存在 → 404;interval_seconds < 60 → 400。

## Probes

- `POST /api/endpoints/{id}/probe` → `{"data": {"endpoint_id": number, "results": [ProbeRecord, ProbeRecord]}}`。同步执行一轮 Probe:先非流式、后流式,两条记录都返回。
- `GET /api/endpoints/{id}/probes?limit=50` → `{"data": [ProbeRecord]}`(按时间倒序,limit 默认 50、最大 200)

`ProbeRecord = {"id": number, "endpoint_id": number, "streaming": boolean, "ok": boolean, "http_status": number, "error_summary": string|null, "latency_ms": number, "ttft_ms": number|null, "input_tokens": number|null, "output_tokens": number|null, "created_at": string(RFC3339)}`
非流式的 `ttft_ms` 恒为 null。

## 探测语义(后端实现约定)

- Prompt 固定:user message `"Reply with the single word: pong"`,`max_tokens` 16,单请求超时 60s。
- anthropic 协议:`POST {base_url}/v1/messages`,headers `x-api-key`, `anthropic-version: 2023-06-01`;流式加 `"stream": true`。
- openai 协议:`POST {base_url}/v1/chat/completions`,header `Authorization: Bearer <token>`;流式加 `"stream": true` 与 `"stream_options": {"include_usage": true}`。
- `ok` = HTTP 200 且响应可解析为正常完成;`error_summary` = HTTP 状态 + 上游错误消息,截断 500 字符。网络层失败 `http_status` 记 0。
- 流式 `ttft_ms` = 发出请求到收到第一个含内容的 SSE 事件;token 用量尽量从响应/流末取得,取不到为 null。

## Overview(ticket 03)

- `GET /api/overview` → `{"data": {"generated_at": string, "endpoints": [OverviewEntry]}}`(公开)

`OverviewEntry = {"endpoint_id": number, "model_id": string, "protocol": "anthropic"|"openai", "enabled": boolean, "status": "healthy"|"degraded"|"down"|"failing", "status_reason": string, "degrade_causes": ["availability"|"latency", ...], "success_rate_24h": number|null(0~1,无数据为 null), "p50_ms": number|null, "p95_ms": number|null, "last_probe_at": string|null, "family": string, "capability": string, "score": number|null(0~100 稳定性评分,无探测数据为 null), "score_reasons": [string, ...](扣分说明,无扣分项为 []), "dots_24h": [OverviewDot](恒 24 桶,最旧小时在前,空桶保留), "eval_score": number|null(0~100 最近一次评估总分,无评估数据为 null), "baseline_p50_ms": number|null(7 天 P50 延迟基线,与状态机降级判定同一份值,基线样本 <5 为 null)}`

`OverviewDot = {"bucket_start": string(RFC3339,整点), "total": number, "failures": number, "p50_ms": number|null(桶内仅成功探测的 P50;失败探测的延迟是失败耗时、绝不计入,无成功探测为 null)}`

状态判定规则(优先级从高到低):
- `down`(红):最近连续 3 次 Probe 失败
- `failing`(闪烁):最近一次 Probe 失败但未达连续 3 次
- `degraded`(黄):24h 成功率 <0.95,或 24h P95 延迟 > 该端点 7 天 P50 基线的 2 倍(基线数据不足时跳过此项);两条同时命中时并列——`degrade_causes` 同列 `["availability", "latency"]`(可用性在前),`status_reason` 为双片段
- `healthy`(绿):其余
`status_reason` 为人类可读判定依据(如 "连续 3 次失败,最近错误: HTTP 503: No available providers");degraded 双命中时为两个原因片段,按「可用性片段;延迟片段」以中文分号「;」连接。`degrade_causes` 为结构化成因(可用性="availability",延迟="latency"),非降级状态恒为 `[]`(空数组,不为 null),前端展示副标签一律消费此字段,不解析 status_reason。

## Discovery(ticket 05)

- `POST /api/discovery/run` → `{"data": {"added": number, "retired": number, "endpoints_created": number}}`。立即对所有 Hub 执行一次同步(定时任务每小时也会自动跑)。
- 同步语义:拉每个 Hub 的 `/v1/models`;新 model_id → 建 Model(origin="discovered", capability 按名单判断:含 "image"/"embedding"/"tts"/"dall" 等非对话关键词 → "non_chat",否则 "chat")并对双协议各发一次极简请求试通,通的建 enabled Endpoint,不通的建 disabled Endpoint;列表中消失且 origin="discovered" 的 Model → status="retired"(其 Endpoint 停止调度,历史保留);重新出现 → 恢复 "active"。手工添加的 Model(origin="manual")不受下线影响。

## Eval(ticket 08)

- `GET /api/suites` → `{"data": [Suite]}`;`Suite = {"id": number, "key": string, "name": string, "cases": [Case]}`
- `Case = {"id": number, "suite_id": number, "prompt": string, "verdict_type": "rule"|"judge", "rule_config": {"mode": "exact"|"regex"|"contains", "expected": string}|null, "rubric": string|null, "enabled": boolean}`
- `POST /api/cases` → 201。Body 同 Case(无 id)。`PATCH /api/cases/{id}` → `{"data": Case}`(字段均可选)。
- `POST /api/evals` → 202 `{"data": EvalRun}`。Body: `{"suite_id": number, "model_ids": [number,...]}`(model 的数据库 id;含 non_chat 模型 → 400)。异步执行。
- `GET /api/evals` → `{"data": [EvalRun]}`(倒序)
- `GET /api/evals/{id}` → `{"data": EvalRunDetail}`
- `EvalRun = {"id": number, "suite_id": number, "trigger": "scheduled"|"manual", "judge_model": string, "status": "running"|"done"|"failed", "started_at": string, "finished_at": string|null}`
- `EvalRunDetail = EvalRun + {"results": [EvalResult]}`
- `EvalResult = {"id": number, "model_id": string, "case_id": number, "answer_text": string|null, "score": number|null(0~1;裁判失败为 null 即未判分), "verdict_detail": string|null, "latency_ms": number, "input_tokens": number|null, "output_tokens": number|null}`
- 判定:rule 按 mode 对 answer 全文判定(命中 1 否则 0);judge 用 judge_model(默认 claude-opus-4-8,可取 settings.judge_model)按 rubric 打 0~1,解析失败/调用失败 → score=null。

## Settings & Alerts(ticket 06,本波不实现,先占位契约)

- `GET /api/settings` → `{"data": {"lark_webhook_url": string, "alert_enabled": boolean, "score_drop_alert_enabled": boolean, "judge_model": string}}`(写操作;GET 公开但 lark_webhook_url 原样返回,内部工具不脱敏)
- `PUT /api/settings` → `{"data": ...同上}`(字段均可选)
- `GET /api/alerts?limit=50` → `{"data": [AlertEvent]}`,`AlertEvent = {"id": number, "endpoint_id": number|null, "kind": "down"|"recovered"|"score_drop", "message": string, "sent_ok": boolean, "created_at": string}`

## Endpoint Detail & Series(ticket 04)

- `GET /api/endpoints/{id}` → `{"data": EndpointDetail}`。`EndpointDetail = Endpoint + {"model_id_str": string, "hub_name": string, "status": string, "status_reason": string}`(status 判定与 Overview 同规则;degraded 双命中时 status_reason 同规则变双片段,但 detail 不提供 degrade_causes 字段,详情页不显示成因副标签)。
- `GET /api/endpoints/{id}/series?hours=24&streaming=all` → `{"data": [SeriesBucket]}`。hours 默认 24、范围 1~2160(90 天);streaming ∈ all|streaming|non_streaming,默认 all(合并统计)。`SeriesBucket = {"bucket_start": string(RFC3339,整点), "total": number, "failures": number, "p50_ms": number|null, "p95_ms": number|null, "avg_ttft_ms": number|null}`,按时间正序,无数据的桶可省略。
- `GET /api/endpoints/{id}/probes` 增加可选参数 `ok=true|false` 过滤(与 limit 组合,用于"近期失败"列表)。
- 数据生命周期:每小时把 1 小时前的原始 probes 聚合进 probe_rollups(endpoint_id, streaming, bucket_start, total, failures, p50_ms, p95_ms, avg_ttft_ms,幂等 INSERT OR REPLACE);每天删除 90 天前的原始 probes;series 查询 = rollups + 未聚合的原始尾部合并,保证 rollup/清理后历史曲线完整可查。

## Eval 补充(ticket 09)

- `GET /api/evals/latest` → `{"data": [LatestScore]}`;`LatestScore = {"suite_id": number, "suite_key": string, "model_id": string, "model_db_id": number, "score": number|null, "eval_run_id": number, "finished_at": string}`。每个 (suite × model) 取最近一次 done 的 run 的聚合分。
- 裁判模型来源:settings.judge_model(默认 claude-opus-4-8),evaluator 每次 run 开始时读取。
- 每周定时:每周日凌晨(本地时区)对所有 active 且 capability=chat 的模型 × 全部 enabled suite 各跑一次(trigger="scheduled")。
- 分数大跌告警:某 (suite × model) 本次聚合分较上次 done run 下跌超过 0.2 且 settings.score_drop_alert_enabled=true → 经飞书通道发 score_drop 告警并落 alert_events(kind="score_drop", endpoint_id=null)。

## Task Center(ticket 18)

- `GET /api/tasks?type=&status=&page=1&page_size=20` → `{"data": {"items": [Task], "total": number, "page": number, "page_size": number}}`(倒序,page_size 上限 100;type/status 精确过滤,可空)。读遵循监控数据分级:需登录。
- `GET /api/tasks/{id}` → `{"data": TaskDetail}`;`TaskDetail = Task + {"logs": [TaskLog]}`(按时间正序)。未知 id → 404,非法 id → 400。
- `Task = {"id": number, "type": "eval_run", "source": "manual"|"scheduled", "status": "pending"|"running"|"success"|"failed", "entity_type": "eval_run", "entity_id": number, "started_at": string|null, "finished_at": string|null, "duration_ms": number|null(终态才有), "created_at": string}`
- `TaskLog = {"id": number, "at": string, "level": "info"|"warn"|"error", "message": string}`
- 每次 Eval Run(手动 POST /api/evals 或每周定时)执行时注册一个 Task:开始 → running,逐 Case 写进度日志(完成带分数、裁判失败 warn),Run 终态映射 success/failed;进程重启时遗留 pending/running 的 Task 启动即置 failed。探测轮次不是 Task。
