# 告警聚合窗口:发送与状态判定解耦(W5 扩展)

**Status:** accepted(2026-07-30,spec 0017 ticket 2/5,GH #65)

## 决策

W5(状态机与告警防抖)扩展:endpoint up/down 告警从「状态迁移判定即发送」改为「**判定时刻落库 → 60s 聚合窗口缓冲 → flush 聚合发送**」。

- **判定与发送解耦:** transition 判定时刻立即落 per-endpoint 事件(`sent_ok=false`,含义「投递未确认」)并翻转内存 alerted flag——W5 懒状态重建语义不变,重启不重复告警;迁移随后进入窗口缓冲而非立即发送。
- **聚合窗口:** 首个 pending 迁移开 60s 窗口(固定窗口,非滑动;窗口长度是冻结常量,不做配置面)。flush 时按 kind 分两卡(down 红卡 / recovered 绿卡,好消息坏消息不混卡),卡内按 Hub 分节;down 卡内同 Hub ≥2 端点标注「疑似 Hub 侧故障」(标注只属于故障卡,恢复卡同构分节但不带怀疑措辞)。发送后回写各事件 `sent_ok`(store 新增 `UpdateAlertEventsSentOK`)并落一条 hub-less `batch` 事件(message = 实际发送的聚合文本,sent_ok = 真实投递结果)。发送失败不重试,与既有语义同构。
- **单端点是 N=1 的聚合:** 不保留独立的单端点卡片形态,聚合卡片结构(标题 + 影响端点/涉及 Hub fields + Hub 分节 detail + 签名 note)对所有组大小统一,单端点告警至多延迟 60s(显式行为变更,已经用户确认)。
- **时钟(W4):** 窗口 flush 由可注入 `scheduler.Clock` 驱动,不引入 cron 库。server 新增构造 Option `WithAlertClock`(与 `WithNow` 同族),经 `Evaluator.UseClock` 构造期换钟;生产 RealClock,测试 FakeClock 手动推进,零真实等待。
- **不动的部分:** `prober.AfterRound` 钩子语义(逐端点逐轮同步调用)、单实例单 `mu` 锁、发送在锁内、login_alert 即时 off-lock 路径、score_drop 即时发送(静默门属 ticket 4)、webhook/alert_enabled 在 transition 判定时刻读取(未配置仍不落事件);flush 发送时二次读取 webhook——窗口期改 webhook 则发往新地址,清空则跳过发送、事件诚实停 `sent_ok=false`;`alert_enabled` flush 不重检,窗口期关闭仍会发出已缓冲卡(切点登记,避免 ticket 4 挂静默门时误读)。

## 承重墙四问

1. **为什么必须改:** 单端点即时发送在 Hub 级故障下产生 N 条消息,恢复时再刷 N 条——W5 的存在目的(告警可信度)反而被轰炸磨损。聚合窗口用单端点至多 60s 的延迟换多点故障时「一次故障一条消息」。
2. **影响哪些调用方:** `prober.AfterRound` 钩子语义不变;`Evaluator.HandleRound` 内部从「判定即发送」改为「判定落库 → 窗口缓冲 → flush 发送」;store 新增 `UpdateAlertEventsSentOK` 与 `batch` kind;server 新增 `WithAlertClock` Option;`GET /api/alerts` 事件流出现 `batch` kind(hub-less,仅 super_admin 的 *All 视图可见,与 score_drop 先例一致)。login_alert 与 score_drop 路径零改动。
3. **有无替代方案:** ① Lark 消息编辑跟进——否决(消息量未实质减少,API 复杂);② 长窗口(3min)——否决(急性故障感知变慢);③ 事件延迟到 flush 才落库——否决(窗口期重启会重复告警,直接违反 W5);④ 窗口期崩溃后补发——否决(事件已落库,补发=重复;`sent_ok=false` 诚实读作「投递未确认」,可审计)。
4. **回归测试什么:** 既有生命周期测试(TestLarkAlertingLifecycle / TestAlertSkippedWithoutWebhook / TestAlertSendFailureRecorded / TestAlertRestartDoesNotRepeat / 两个 image lifecycle / TestTestLarkDoesNotPolluteAlertState)在假时钟下改写,断言语义等价(迁移一次一条、持续沉默、恢复一条、失败落事件不重试、重启不重复、test 事件不污染状态重建);新增黑盒:单端点窗口延迟(推进前 0 条)、跨 Hub 聚合(1 卡 + 疑似 Hub 侧故障标注)、恢复聚合(1 绿卡)、down/恢复同窗分卡(2 卡)、窗口内重启丢缓冲不重报、batch 事件内容与 sent_ok 回写、login 爆破即时性(auth_bruteforce_alert_test 未动,回归绿)。

## 理由与代价

事件落库时机提前到判定时刻是整张设计的承重点:它让「重启不重复告警」完全不依赖发送是否发生,窗口缓冲因此可以安全地做成易失的(进程重启丢缓冲,`sent_ok=false` 留下诚实审计痕迹)。已接受的代价:① 单端点告警至多 60s 延迟(显式行为变更);② 窗口期重启丢一条本应发出的消息(事件在,可审计,不重报——与「发送失败不重试」同为宁可漏发不可轰炸的取舍);③ 历史表事件量约翻倍(每窗口每 kind 多一条 batch 事件)——这是「告警历史能看到实际发出原文」(spec 0017 故事 31)的必要成本。

## 为后续 ticket 预留的扩展点

flush 管线形状 = `transition 落库 → bufferLocked(append + 首个 pending 开窗口)→ flushLocked(取缓冲 → 按 kind 分组 → buildBatchMessage → 发送 → 回写 → 落 batch)`。ticket 3(分组告警)在 transition 后加组评估、向窗口投递 group 语义条目(flush 分组渲染时组卡为主、未入组端点按 Hub 分节为次,`group_key` 列随票迁移);ticket 4(静默时段)在 flushLocked 发送处与 score_drop 发送处加静默门,静默边界检查复用同一注入时钟。本票不实现两者的任何功能。
