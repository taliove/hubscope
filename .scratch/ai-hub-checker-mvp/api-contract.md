# API Contract — Ticket 01 (前后端共同遵守,不得偏离)

Base path: `/api`。所有响应 JSON。成功:`{"data": ...}`;失败:非 2xx 状态码 + `{"error": {"message": "..."}}`。

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
`Endpoint = {"id": number, "model_id": number, "protocol": "anthropic"|"openai", "enabled": boolean}`

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
