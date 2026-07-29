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
`status_reason` 为人类可读判定依据(如 "连续 3 次失败,最近错误: HTTP 503: No available providers");degraded 双命中时为两个原因片段,按「可用性片段；延迟片段」以中文全角分号「；」连接。`degrade_causes` 为结构化成因(可用性="availability",延迟="latency"),非降级状态恒为 `[]`(空数组,不为 null),前端展示副标签一律消费此字段,不解析 status_reason。

## Discovery(ticket 05)

- `POST /api/discovery/run` → `{"data": {"added": number, "retired": number, "endpoints_created": number}}`。立即对所有 Hub 执行一次同步(定时任务每小时也会自动跑)。
- 同步语义:拉每个 Hub 的 `/v1/models`;新 model_id → 建 Model(origin="discovered", capability 按名单判断:含 "image"/"embedding"/"tts"/"dall" 等非对话关键词 → "non_chat",否则 "chat")并对双协议各发一次极简请求试通,通的建 enabled Endpoint,不通的建 disabled Endpoint;列表中消失且 origin="discovered" 的 Model → status="retired"(其 Endpoint 停止调度,历史保留);重新出现 → 恢复 "active"。手工添加的 Model(origin="manual")不受下线影响。

## Eval(ticket 08)

- `GET /api/suites` → `{"data": [Suite]}`;`Suite = {"id": number, "key": string, "name": string, "cases": [Case]}`
- `Case = {"id": number, "suite_id": number, "prompt": string, "verdict_type": "rule"|"judge", "rule_config": {"mode": "exact"|"regex"|"contains"|"mcq"|"output_match", "expected": string}|null, "rubric": string|null, "enabled": boolean}`
- `POST /api/cases` → 201。Body 同 Case(无 id)。`PATCH /api/cases/{id}` → `{"data": Case}`(字段均可选)。
- `POST /api/evals` → 202 `{"data": EvalRun}`。Body: `{"suite_id": number, "model_ids": [number,...]}`(model 的数据库 id;含 non_chat 模型 → 400)。异步执行。
- `GET /api/evals` → `{"data": [EvalRun]}`(倒序)
- `GET /api/evals/{id}` → `{"data": EvalRunDetail}`
- `EvalRun = {"id": number, "suite_id": number, "trigger": "scheduled"|"manual", "judge_model": string, "status": "running"|"done"|"failed", "started_at": string, "finished_at": string|null}`
- `EvalRunDetail = EvalRun + {"results": [EvalResult]}`
- `EvalResult = {"id": number, "model_id": string, "case_id": number, "answer_text": string|null, "score": number|null(0~1;裁判失败为 null 即未判分), "verdict_detail": string|null, "latency_ms": number, "input_tokens": number|null, "output_tokens": number|null}`
- 判定:rule 按 mode 对 answer 全文判定(命中 1 否则 0);judge 用 judge_model(默认 claude-opus-4-8,可取 settings.judge_model)按 rubric 打 0~1,解析失败/调用失败 → score=null。

## Settings & Alerts(ticket 06,本波不实现,先占位契约)

- `GET /api/settings` → `{"data": {"lark_webhook_url": string, "alert_enabled": boolean, "score_drop_alert_enabled": boolean, "judge_model": string, "default_sample_count": number, "suite_weights": object, "eval_concurrency": number}}`(写操作;GET 公开但 lark_webhook_url 原样返回,内部工具不脱敏)
- `PUT /api/settings` → `{"data": ...同上}`(字段均可选;`default_sample_count` ∈ [1,10]、`eval_concurrency` ∈ [1,16],越界 400)
- `POST /api/settings/test-lark`(super_admin,ticket 100)→ body `{"webhook_url": string}`(必填,绝对 http/https URL,否则 400)→ `{"data": {"sent_ok": boolean, "error": string|null}}`。测试目标是 body 里的地址(非已保存设置),不受 alert_enabled 开关影响;每次尝试(成功/失败)落 alert_events(kind="test", endpoint_id=null);error 不含 webhook URL(W6)。
- `GET /api/alerts?limit=50` → `{"data": [AlertEvent]}`,`AlertEvent = {"id": number, "endpoint_id": number|null, "kind": "down"|"recovered"|"score_drop"|"score_drop_skipped"|"test", "message": string, "sent_ok": boolean, "created_at": string}`
- 飞书告警消息以消息卡片形态发送(ticket 101,`msg_type: "interactive"`,legacy card JSON):颜色标题栏(down/登录爆破 = red、score_drop = orange、recovered = green、test = turquoise)+ 双列字段(模型/协议/错误等)+ 时间备注行;`alert_events.message` 仍落纯文本,告警历史表渲染不变。

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

## Eval 执行器并发与失败补救(GH #26/#27/#28)

- 执行单元为 (suite × model) cell(GH #26):批次开始即建好全部 suite 的 run(进度网格语义不变),cell 进有界工作池并发执行,并发数 = settings.eval_concurrency(默认 4,clamp [1,16]);cell 内 case 仍串行(单模型内时序与 Hub 压力可控)。全部 cell 完成后统一 settle,AfterCampaign 恰好一次;ctx 取消后不再领取新 cell。单 suite 手动触发(POST /api/evals 带 suite_id)同样走 cell 池(单 run × 多 model)。
- answer 调用失败自动重试 1 次(GH #27,共 2 次尝试,立即不退避——120s 超时本身已是等待;judge 调用不重试,RequestTimeout 120s 不变):两次都失败 → score=null,verdict_detail 形如 `answer call failed after 2 attempts: <末次原因>`;重试成功正常判分,detail 注明第 2 次尝试成功,不谎称一次成功。
- `POST /api/campaigns/{id}/retry-failed`(GH #28;hub scoped 写组,与 evals.create 同组)→ 202 `{"data": CampaignDetail}`(同 POST /api/evals 响应形状),异步执行。前置校验:批次须 done/failed 且存在 score IS NULL 的结果,否则 409 `campaign has no failed results to retry` / `campaign is not settled`;批次不存在 404;匿名 401;跨 hub 写按既有组口径。执行语义:批次状态 done/failed → running(**仅本路径可回迁**,store 层 WHERE 守卫),同事务把仍含 null 分结果的成员 run 一并回迁 running(GH #39;干净 done run 不动);retry 每完成一个 run 的全部重评 cell 即标 done(ctx 取消→未完成的 run 标 failed,与正常执行同语义:null 分是结果不是 run 失败),批次据此重新 settle;对每个含 null 分结果的 (run × model),仅删除该单元 score IS NULL 的结果行(删除条件 `score IS NULL` 硬编码,构造上不可能误删已判分结果——已判分结果逐字节不动,W7 同向),重评这些 case 并插入新结果(仍走 answer 自动重试);全部完成后走既有 SettleCampaign 统一 settle。已知并接受:AfterCampaign 会再触发一次(单次 retry-settle 一次,不连发),分数大跌告警以重跑后数据重新对比。
- `CampaignReport` 新增 `failed_results: number`(GH #28)——批次全部 run 中 score IS NULL 的结果行数;settle 批次 `failed_results > 0` 是前端「重跑失败项」按钮的渲染条件。公开榜单与分享报告同形状返回(失败计数是运行元数据,与覆盖率水印同源信息)。

## Task Center(ticket 18)

- `GET /api/tasks?type=&status=&page=1&page_size=20` → `{"data": {"items": [Task], "total": number, "page": number, "page_size": number}}`(倒序,page_size 上限 100;type/status 精确过滤,可空)。读遵循监控数据分级:需登录。
- `GET /api/tasks/{id}` → `{"data": TaskDetail}`;`TaskDetail = Task + {"logs": [TaskLog]}`(按时间正序)。未知 id → 404,非法 id → 400。
- `Task = {"id": number, "type": "eval_run", "source": "manual"|"scheduled", "status": "pending"|"running"|"success"|"failed", "entity_type": "eval_run", "entity_id": number, "started_at": string|null, "finished_at": string|null, "duration_ms": number|null(终态才有), "created_at": string}`
- `TaskLog = {"id": number, "at": string, "level": "info"|"warn"|"error", "message": string}`
- 每次 Eval Run(手动 POST /api/evals 或每周定时)执行时注册一个 Task:开始 → running,逐 Case 写进度日志(完成带分数、裁判失败 warn),Run 终态映射 success/failed;进程重启时遗留 pending/running 的 Task 启动即置 failed。探测轮次不是 Task。

## Eval Campaign 实时动态(issue #17,仅控制台)

- `GET /api/campaigns/{id}/live-feed?since_id=N&limit=M` → `{"data": [LiveFeedEntry]}`。运行中批次的题目级判分动态流,游标增量拉取;**需登录会话,不进 publicReadPattern,分享页(share token)与公开榜单不提供本数据**(spec 0004 半成品边界)。
- 游标语义:只返回 `id > since_id` 的记录(严格大于),`since_id` 缺省 0;按 `id` 升序;空增量返回空数组 `[]`。`since_id` 非整数或为负 → 400。`limit` 与 probes 同口径:默认 50、上限 200(超出截断),非法值回退默认。
- 可见范围(hub 隔离,与 `GET /api/campaigns` 列表同口径):super_admin 可见全部批次;hub 用户仅可见成员模型含本 hub 模型的批次——他 hub 批次与不存在批次同答 404 `campaign not found`(无枚举预言);匿名 401。
- `LiveFeedEntry = {"id": number, "model_id": string, "suite_key": string, "suite_name": string, "case_id": number, "case_prompt": string, "verdict_type": "rule"|"judge"|""(case 已被清理时为 ""), "score": number|null(0~1 原始分,裁判失败为 null;0-100 换算在前端), "latency_ms": number, "created_at": string(RFC3339)}`
- 已 settle 批次同接口返回其最终快照(不再增长);作答原文/裁判理由等单题详情不在本接口范围(另议)。

## Eval Campaign Report 覆盖率门槛(ticket 91,spec 0014 决策 A)

- 三个端点共用同一报告形状与排名口径:`GET /api/campaigns/{id}/report`(session)、`GET /api/public/eval/board`(公开,report 字段即最新 settle 批次报告)、`GET /api/shared-reports/{token}`(token 门控)。
- 报告行 ReportRow 新增两个字段(ticket 92 前端按此消费,**定稿即冻结**):
  - `complete: boolean` — 完整性判定,**仅 settle(done/failed)批次的行出现**;运行中/等待中批次的 live 行不含此键。`true` ⟺ 该模型在批次覆盖且当前仍有启用 case 的全部 suite 上 AVG(score) 非 null(每个有题维度至少判上 1 题;W7「裁判失败计 null 不计 0」口径不变,维度分本身不受门槛影响)。
  - `missing_suites: number` — **仅 `complete=false` 时出现**,值为未判上分的 suite 数(前端「判分不完整 N/M 维度」水印的 N;**M = 批次覆盖且当前有启用 case 的 suite 数**,即门槛分母,而非报告 suites 数组长度——零启用 case 的 suite 不参与判定,见下方边界)。
- 排名口径(settle 批次):`complete=false` 的行 `total_score=null`、`total_delta=null`,排在全部 `complete=true` 行之后——**与 sort 参数(总分或任一 suite 列)无关**;不完整组内按 `model_id` 字典序。完整组内部口径不变(所选列降序、null 沉底、`model_id` 字典序 tiebreak)。全部模型不完整时正常 200 返回(行保留、全体 total/rank 为 null),不空态、不报错。
- 涨跌口径:不完整模型无 delta(无总分即无 delta);baseline 批次侧同一门槛——模型在 baseline 批次不完整时其 baseline 总分视为不存在,当前批次同样不给 delta。
- 边界:零启用 case 的 suite 不参与完整性判定(无题维度是题库配置而非判分缺口,其分数仍照常展示、照常从总分剔除——既有「未判分维度从分子分母剔除」口径不变);live(运行中批次)语义零改动,门槛只对 settle 批次生效。
