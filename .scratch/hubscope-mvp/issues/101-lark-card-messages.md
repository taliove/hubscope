# 101 — 飞书告警消息卡片化(interactive card)

**What to build:** 把 LarkSender 的纯文本 `text` 消息升级为飞书**消息卡片**(`msg_type: "interactive"`,legacy card JSON——自定义机器人稳定支持的形态)。两条用户已定决策(2026-07-28,附 ASCII 预览确认):① 形态 = 消息卡片(颜色标题栏 + 双列字段 + 灰色备注行),否掉富文本 post(无颜色区分,告警/恢复/测试长一个样);② 范围 = **全部消息类型**(test/down/recovered/score_drop/登录爆破),否掉只换测试消息。落地范围:

- **卡片结构**:header(template 色 + 「{标题} · HubScope」)→ div 双列字段(lark_md,`**字段名**\n值`)→ hr → note(时间 + 「HubScope 服务监控」);`config.wide_screen_mode: true`。
- **颜色映射**(与 ui-guidelines §3 语义同向,飞书 template 名):down → `red`,登录爆破 → `red`,score_drop → `orange`,recovered → `green`,test → `turquoise`(电波青品牌色)。
- **架构**:引入结构化消息——`Send(ctx, url, msg)` 其中 msg 携 `Text`(纯文本,继续落 alert_events.message,历史表不变)与卡片载荷(标题/颜色/字段);各告警点(buildMessage/score_drop/login_alert/test_alert)从「拼一整段文本」改为「文本 + 字段」双产出,发送走卡片、落库走文本。保持全进程唯一 Evaluator 经手(W5),off-lock 语义不变。
- **契约**:无 API 变更(对外行为是飞书侧呈现);api-contract.md 的告警段落补一句「消息以卡片形态发送」。
- **TDD**:stub webhook 断言 `msg_type=interactive`、header.template 按类型取色、字段含模型/协议等关键值;alert_events.message 仍为纯文本(历史表回归);W6 不变(URL 不入日志/审计/错误)。

**Blocked by:** 无 — 可立即开工

**Status:** in_progress(2026-07-28 开工;形态与范围已经用户确认)

## 验收清单

- [ ] 测试消息在飞书群呈现为 turquoise 标题栏卡片(标题「测试消息 · HubScope」类)
- [ ] down/recovered/score_drop/登录爆破分别为 red/green/orange/red 标题栏卡片,字段结构化(模型、协议、错误、跌幅等)
- [ ] alert_events.message 仍是纯文本,告警历史表渲染不变(回归)
- [ ] stub 断言卡片 JSON 结构(msg_type/header/字段);`make test` 全绿;check 三维度 PASS

## 风险登记

1. **飞书卡片格式兼容**:用 legacy card JSON(config/header/elements),不用卡片 2.0 schema——自定义机器人兼容性最稳
2. **lark_md 注入**:字段值含模型名/错误摘要等外部文本,直接插值进 lark_md 即可(飞书侧转义),不引入 HTML
3. **W5 语义不变**:只改呈现层,告警触发时机/防抖/落库口径零改动,transition() 逻辑不动
4. **双产出漂移**:文本与字段两份内容须同源生成(同一函数产出),禁止两处各写一份文案
