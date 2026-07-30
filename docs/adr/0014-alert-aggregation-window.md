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

## ticket 3 落地补记(GH #66,2026-07-30)

**other 排除裁决(main 裁决):** family 为 `other`(未分类兜底桶)或空串的模型不参与组评估——「other」不是厂商责任边界,其桶内多数故障是 Hub 侧故事,已由窗口聚合的「疑似 Hub 侧故障」标注覆盖;spec 0017「仅厂商维度」的意图是上游责任边界,兜底桶不满足。

**吸收兜底语义的收敛链(两轮对抗审查,HIGH-1 → MEDIUM-1 → 定稿):** 「判定时刻标记 + flush 兜底」双层机制中,判定时刻的 `absorbed = groupWasOpen || groupOpenNow` 本身完备,漏洞全在兜底层。兜底语义经三步收敛:① family 全窗口过滤(HIGH-1:吞掉组 open 区间之外的迁移,「该发的没发」)→ ② 位置/区间语义(反证:触发前已自愈端点的迁移在区间内但组卡没讲它的故事)→ ③ **故事覆盖集**:只吸收「其故事已被本窗口组卡冻结快照讲述」的迁移(键 = {hubName, modelID, protocol},hubName 消歧同 model_id 跨 Hub),recovered 永不走兜底。③ 仍有残余(MEDIUM-1):裸并集不消耗——绿卡讲过「已恢复」的成员同窗再 down 被误吞。定稿 = **覆盖集按 groupPending 顺序重放**(group_down 并入 faulty,group_recovered 减除其「已恢复」名单)——**重放才是完整语义**;已知代价:被绿卡闭环成员的触发前 down 会渲染端点卡与红卡重复(**过度报告是安全方向**:宁可重复,不可吞没)。此结论是后续任何 flush 兜底机制(ticket 4 静默判定同处)的开工前必读。

## ticket 4 落地补记(GH #67,2026-07-30)

**同构过滤自查结论(check 预警的回应):** 静默门不做「期内发生过什么」的全窗口判定。flush 的静默判定只看窗口**预定触发时刻**是否落在静默时段(瞬间判定,非区间过滤);摘要候选集的推导锚在**逐端点/逐组的事件序列**上(最新事件为 down 即仍 open——与 isAlerted 懒重建同一语义),不引入第二种「期内故事」口径。HIGH-1 教训(按 family 全窗口吞并)在此无同构形态。

**摘要候选 = 仍 open 且锚事件 sent_ok=false(本票核心裁决):** 仍 open 由事件推导(重启安全,不用内存态),再叠加「锚事件投递未确认」过滤——三重收益:① 静默前已投递的告警(锚 sent_ok=true)不被摘要重复报告;② 摘要回写锚事件 sent_ok=true 后,**端点路径**下一静默周期自然不再重复(连推 24h+ 不重复发摘要,不靠内存去重);③ 重启后 sent_ok=false 的仍 open 端点自动进入下一次摘要——顺带愈合窗口缓冲重启丢失(ADR 上文已登记的易失性)留下的投递缺口。自愈端点(尾事件为 recovered)与已确认锚天然出局;被吸收进组卡的成员事件保持 sent_ok=false,其故事由组行承担(组锚同规则),成员不重复列出(每个故事讲一次)。

**②的收窄(check GH #67 HIGH-1,接受行为 + GH #76 跟进):** 「跨周期不重复由构造保证」仅端点路径成立;组吸收路径存在两个已登记例外——场景 A:静默前已投递组卡(组锚 true)的被吸收成员(锚 false、仍 open)会以个体身份进摘要,与静默前组卡已讲述的故事重复;场景 B:组在静默内开、摘要确认组锚后故障持续跨夜,第二夜起组行跳过(锚 true)、成员以个体身份重复进摘要。根因:锚 sent_ok 无法区分「故事已被组卡冻结快照讲述的被吸收成员」与「组 open 后新加入、故事从未讲述的成员」——与 GH #66 HIGH-1 同根(区间/覆盖语义按扁平标志位塌缩)。两个例外的产物都是重复(**安全方向**:宁可重复,不可吞没),故接受行为并以黑盒钉住(TestQuietHoursSummaryListsAbsorbedMembersOfDeliveredGroup / TestQuietHoursSummaryRelistsAbsorbedMembersNextNight);正确修法(摘要感知组卡覆盖集:折叠已被组卡快照讲述的成员、列出组开后新加入的成员)是设计迭代,由 GH #76 承载。

**窗口起点侧门与已确认锚跳过(check GH #67 MEDIUM-1,已修):** flush 静默门加判窗口起点侧——`contains(firesAt) || contains(firesAt.Add(-alertWindow))` 任一成立即 hold,起点在窗内的窗口整体让给静默结束摘要(否则 06:59:30 的迁移会既得 07:00 摘要条目、又在 07:00:30 补发卡,双报);两次判定仍是窗口自身两个端点的瞬间判定,不构成「期内发生了什么」的区间过滤,同构纪律不破。配套:flush 渲染前丢弃已被摘要确认的迁移(锚 sent_ok=true 即其故事已被摘要讲述,store 新增 ConfirmedAlertEvents)——消除「判定恰在 07:00:00 整点、窗口起点落在大声侧而摘要已覆盖」的残余同刻竞争;起点侧门负责窗口级整体让位,已确认锚跳过负责整点同刻的逐迁移兜底,两个 goroutine 任意交错结果一致。

**中途关闭静默的搁浅登记(check GH #67 LOW-1,不改行为):** 静默期内延迟的 score_drop 内存队列在静默被中途关闭后搁浅,直到下次启用并度过静默期才随摘要投递;重启则丢失(事件在库,诚实停 sent_ok=false——与窗口缓冲易失性同一取舍)。端点/组候选纯事件推导可自愈,score_drop 队列不可,差异登记于此。

**窗口预定触发时刻语义:** flush 的静默门评估 `windowFiresAt`(武装时记录)而非处理器运行时刻——真实时钟下两者仅差毫秒,但假时钟跨边界推进时,迟到的 flush goroutine 仍按窗口到期那一刻的静默状态判定,消除测试时序竞态(两个 goroutine 任意交错结果一致)。

**边界检查器形态:** 单一 boundary timer 链,在每个告警判定点(transition 缓冲/flush/HandleCampaign)与边界处理器自身重武装到下一个窗口边(start 或 end);处理器按**当前时刻**评估状态(进入静默/被禁用/被粗粒度推进整段跳过则不发),一次 fire 至多一条摘要。**在飞 fire 守卫(-race 下暴露并修复):** 已到期(boundaryAt ≤ now)的武装中 timer 不得被重武装抢先替换,否则已入 channel 未消费的 fire 被守卫判 stale 而吞掉边界穿越——到期即交由边界处理器自行重武装。

**score_drop 延迟语义:** 静默期内 HandleCampaign 照常评估,事件落库 sent_ok=false(投递未确认),冻结文本入内存队列随摘要投递并回写;队列与窗口缓冲同为易失(重启丢失,事件在库,诚实停 sent_ok=false——同一取舍,不重复登记)。login_alert 完全豁免,off-lock 路径零改动。**设置变更生效点:** 静默三键在判定点重读,无告警活动时不武装边界 timer——此时亦无内容可摘要,语义自洽(登记为已知切点,与 ADR 上文 alert_enabled 切点先例同列)。
