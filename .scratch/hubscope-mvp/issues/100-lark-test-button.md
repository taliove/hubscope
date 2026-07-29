# 100 — 飞书告警通道「发送测试」按钮 + test 事件类型

**What to build:** 在 /admin 告警设置区加「发送测试」按钮,让管理员当场验证飞书 webhook 链路,并在告警历史留下证据。背景:现网排查发现 `alert_events` 长期为空——开关默认开(`DefaultAlertEnabled=true`),但 webhook 从未保存,`alerter.transition()` 在 `webhook == ""` 时连事件都不落库,历史页无从分辨「没告警」与「没配置」。三条已定决策:① 测试目标是**表单输入框里的地址**(不依赖先保存);② 测试消息**落 alert_events,新增 kind="test"**(endpoint_id=null,成功/失败都落,失败 sent_ok=false);③ 测试**不受 alert_enabled 开关影响**(显式手动动作,用途是先验证再开开关)。落地范围:

- **契约先行**(api-contract.md):新增 `POST /api/settings/test-lark`(super_admin),body `{"webhook_url": string}` 必填,响应 `{"data": {"sent_ok": boolean, "error": string|null}}`;`AlertEvent.kind` 枚举增 `"test"`。
- **后端**:`alerter.Evaluator` 增 `SendTest(ctx, webhookURL)`——经既有 LarkSender 发固定文案「【HubScope】测试消息:告警通道配置成功。」并落 test 事件(发送失败也落、返回 error);不触碰 alerted 状态机(test 不进 LatestDownRecoveryEvent 口径,既有 SQL 已按 down/recovered 过滤,补回归断言);发送路径与 login_alert 同例走 off-lock(无 alerted 状态要保护)。handler 校验 `webhook_url` 非空且为 http/https 绝对 URL(否则 400);audit 记 `settings.test_lark`,**不含 URL 与返回体**(W6)。
- **前端**(SettingsPanel):飞书 Webhook 输入框行尾加「发送测试」按钮(请求中 loading + 禁用,空地址前端先提示不发请求);成功 ElMessage.success / 失败 ElMessage.error 带原因;发送后刷新告警事件表;`KIND_LABELS` 增 `test: '测试'`,tag 用中性 info 色(测试不是健康度信号);发送列沿用成功/失败映射。
- **TDD at W1**:httptest stub webhook——200 → 接口 sent_ok=true 且事件落库 kind=test;500 → 接口报错且事件落库 sent_ok=false;空 URL / 非法 scheme → 400;alert_enabled=false 时仍可发送;test 事件不影响 down/recovered 状态重建口径。

**Blocked by:** 无 — 可立即开工

**Status:** done(2026-07-28;实现随并行会话 merge 入 38974e6,audit 断言补强 64d770e;check 三维度 PASS;LOW-1 audit 无 URL 断言已随票补上,LOW-2 LarkSender 错误带响应体 snippet 登记知悉、非本票引入;偏离登记:失败时 HTTP 恒 200 + sent_ok=false,与契约 {sent_ok, error} 形态一致)

## 验收清单

- [ ] 输入合法 webhook 点「发送测试」,飞书群收到测试消息,告警历史出现 kind=test、发送=成功 的记录
- [ ] webhook 返回非 2xx 时接口报错,历史出现 kind=test、发送=失败 的记录
- [ ] 空地址 / 非 http(s) 地址 → 400,不落事件
- [ ] alert_enabled=false 时测试仍可发送(开关只管自动告警)
- [ ] test 事件不干扰宕机/恢复告警状态机(回归)
- [ ] audit 与日志中不出现 webhook URL;`make test` 全绿;check 三维度 PASS

## 风险登记

1. **W6 凭证边界**:webhook URL 含 bot token——不落日志、不进 audit、错误消息不得回显 URL(LarkSender 已剥 url.Error,保持)
2. **SSRF 面**:super_admin 可令服务端向任意 http(s) URL POST 固定文案——内部工具 + 管理员限定,登记为接受;scheme 校验挡掉 file:// 等
3. **W5 单实例纪律**:发送必须经全进程唯一 `alerter.Evaluator`,handler 不得自造 sender
4. **状态机隔离**:test 事件若漏过滤会污染 `LatestDownRecoveryEvent` 的懒重建——既有 SQL 已按 kind 白名单过滤,需回归测试钉死
