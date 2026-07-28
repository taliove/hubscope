# 86 — 爆破 Lark 告警(TDD at W1)

**What to build:** 按 spec 0011 决策 4,补「出事不知道」:实例级口径,**10 分钟内全站登录失败 ≥10 次**触发一次 Lark 告警,**冷却 30 分钟**(冷却期内继续失败不重复发)。复用 server 持有的**唯一 `alerter.Evaluator` 实例**(W5:全进程单实例,不建第二条告警链路),新增认证域告警方法,挂在 85 落地的登录失败观察点上;webhook 配置沿用现有 settings(与端点告警同源)。告警内容:失败次数 + 窗口 + 被尝试最多的用户名 Top3 + 来源 IP Top3(与审计同级信息);**绝不含密码**;webhook 地址不进任何日志(W6);发送失败记应用日志,不重试轰炸。防抖用内存冷却(计数本就内存态,W5 懒重建/落事件语义针对端点状态机,认证域明确不适用,spec 已登记)。**TDD at W1**:黑盒断言告警副作用(Lark 发送方调用次数/内容),先例 `alerts_test.go`;阈值/窗口/冷却经 server option 注入(同 85 延迟表先例),测试用毫秒级参数。

**Blocked by:** 85(登录失败观察点与 option 注入模式先行,两票同改 handleLogin 失败路径,串行避免冲突)

**Status:** done(实现 TDD 合入,2026-07-28;make test 全绿;决策点 3 项由 main 裁决:沿用 alert_enabled 总开关、不落 alert_events、MaxEntries 满时丢最旧)

## 执行顺序(TDD,小步)

1. **红灯**:spec 测试决策 4 的黑盒用例——失败次数达阈值时发送方被调用恰好一次;冷却期内继续失败不重复调用;冷却过后再次达阈值可再发;告警文本含次数/Top 用户名/Top IP、不含密码字样
2. **绿灯**:Evaluator 认证域告警方法 + 失败观察点挂接 + 冷却状态(内存)+ option 注入
3. **回归**:端点告警与评估告警既有用例全绿(Evaluator 改动不得影响 HandleRound/HandleCampaign 路径);`make test` 全绿

## 验收清单

- [x] 10 分钟全站失败 ≥10 次 → Lark 发送方被调用恰好一次
- [x] 冷却 30 分钟内继续失败不重复发送;冷却过后再达阈值可再发
- [x] 告警文本:失败次数 + 窗口 + Top3 用户名 + Top3 IP;grep 断言不含密码、不含 webhook 地址
- [x] 复用唯一 Evaluator 实例,无第二条告警链路;端点/评估告警既有用例回归通过
- [x] 发送失败只记应用日志,不重试、不阻塞登录响应路径(告警发送不得拖慢登录)
- [x] `make test` 全绿;check 三维度 PASS(check 由 main 统一派,结果待回填)

## 风险登记

1. **告警发送阻塞登录路径**:Lark 发送是网络调用,必须异步或保证超时上界,登录响应延迟预算不含告警耗时
2. **Evaluator 单实例语义(W5)**:新增方法不得引入第二个 sender 或绕过既有 webhook 配置解析;HandleRound/HandleCampaign 行为零改动
3. **信息边界**:Top 用户名可能包含真实管理员用户名——与审计同级、发到自己运维的 Lark 群属可接受(spec 已登记),但不得包含密码或会话信息
4. **无 webhook 配置时**(未配 Lark):告警路径静默跳过(与端点告警同语义),不得报错刷屏

## 备注(阶段 1 影响分析裁决登记,2026-07-28)

- **异步发送方案(关键)**:现有 Evaluator 是 `e.mu` 全局锁内**同步**发送(靠 sender 10s 超时兜底)——后台钩子可接受,登录路径绝不能照搬。定为两层:① 计数/冷却在 server 侧 `loginAlertTracker`(与 loginDelayer/ipLimiter 同族,自带锁、nil 零值禁用)——放 Evaluator 共用 `e.mu` 会让探测告警把登录堵 10s,锁冲突是决定性理由;② Evaluator 新方法只管发送且内部异步:不取 `e.mu`、不碰 `e.alerted`,goroutine 内读 settings → 静默跳过/构建消息 → Send → 失败 slog 不重试;**context 必须 context.Background() 派生**(禁止 r.Context(),handler 返回即取消),10s 上界由 LarkSender 自带 timeout 兜底。
- **冷却在判定越阈当下即消费**(不论 webhook 是否配置、发送成败)——否则未配置 webhook 时持续攻击会每失败一次刷一行日志。
- **挂点次序**:`failLogin` 内 `audit → tracker.record(username, clientIP) → penalize 睡眠 → 401`——计数必须在退避睡眠**之前**,否则越阈那次要等 2-8s 才触发告警;record 纯内存 µs 级,不持 DB 连接。
- **开关口径(裁决 1)**:沿用 `alert_enabled` 总开关,不新增 setting(spec「沿用现有 settings」最简解读)。
- **不落 alert_events(裁决 2)**:端点域落事件为跨重启懒重建,认证域内存态防抖零贡献;spec 决策 4 只要求应用日志。若未来要告警页可见,另开票加 `AlertKindLoginBruteForce`。
- **MaxEntries 满时丢最旧(裁决 3)**:保留告警能力;备选「停止计数」与 loginDelayer 同构但方向不利(攻击者喷满表后告警失明),否。
- **补充登记**:异步 goroutine 无 shutdown 挂钩(in-flight 最坏 10s 被 timeout 终止,接受界);用户名入队前截断 64 runes(廉价内存防御)。
- **测试 setup 坑**:须同时用零值 option 关掉 per-IP 限流与登录延迟,防干扰;TopN 断言用 WithTrustProxy + XFF 制造不同来源 IP。
