# 100 — 测试异步泄漏根治:结构性同步接缝 + drain 纪律

**What to build:** 根治「测试触发异步工作但不排空 → TempDir RemoveAll 竞态失败」的系统性 flake(check 终审 CRITICAL-1 + 沉淀建议 1,spec 0014 集成期实测约 1/6 全量跑失败)。现场证据:/tmp/flake_run_3.json(TestRoleWriteMatrix,eval goroutine 尾部写)、/tmp/flake2_8.json(TestRateLimitWrites FAIL;TestCampaignMembersMigrationBackfills 的 evaluator goroutine 在 DB 关闭后仍 persist;discovery async sync 与 task center 日志同类)。根因模式:测试的 drain 条件(或根本没有 drain)早于 goroutine 最后一个写——eval 的尾部写是 task.succeed → SettleCampaign → AfterCampaign 钩子(run 轮询 done 看不到,check 已实证 waitEvalDone 不够);discovery 的 hub 创建后 async sync 无任何 drain 手段;task center 异步日志同理。本票交付:① 为 discovery async sync(与 eval 的 WithSyncEval 同族,server option 注入,先例 WithRateLimits/WithCaptchaPolicy)提供测试用同步执行接缝;② 审计全部触发异步工作的测试(server 测试集),共享 helper(newTestAPIServer 等)默认走同步接缝,真正需要异步语义的测试(轮询/进度/调度器)显式自建异步 server 并注释;③ 纪律沉淀:AGENTS.md 测试三层节(或 .claude/rules)明文「凡触发异步 eval/discovery/task 的测试,drain 必须覆盖 goroutine 全部尾部写或走结构性同步点」+「flake/时序类修复必须 `-count=1` 连跑 N 次附证据」(check 沉淀建议 2);④ 评估同步接缝是否并入 W4 条款(承重墙四问,若并入则 ADR 登记)。注意:ticket 91 的 check 曾观察到一轮未归因整包 FAIL,spec 0014 集成期频率升至 ~1/6(五套件 100 题落地后 eval goroutine 更长)——频率会随题库规模继续上升,本票宜尽快做。

**Blocked by:** 无 — 可立即开工(与 ticket 99 无文件冲突时可并行)

**Status:** ready-for-agent

## 验收清单

- [ ] discovery async sync 有测试用同步接缝(server option 注入族);eval 的 WithSyncEval(38974e6 后已在 main) semantics 对齐
- [ ] 触发异步工作的测试全审计:共享 helper 默认同步;需异步语义的测试显式自建并注释理由
- [ ] `go test ./internal/... -count=1` 连跑 ≥10 轮零 FAIL(证据入票备注)
- [ ] 纪律明文落 AGENTS.md 或 .claude/rules(drain 覆盖尾部写 + `-count=1` 连跑证据)
- [ ] W4 条款评估四问完成(并入则 ADR,不并入则记录理由)
- [ ] `make test` 全绿;check 三维度 PASS

## 风险登记

1. **共享 helper 改默认行为面大**:逐个跑受影响测试,断言「触发后立即可见 running 态」的测试需保留异步并显式标注
2. **同步执行不得进生产路径**:option 默认关闭,生产装配(cmd/)零改动,审查确认
3. **W4 是承重墙**:条款修订必须四问 + ADR,不轻动

## 实施备注(write agent,2026-07-28)

### 接缝设计

- **新增 `WithSyncDiscovery()`(server option 注入族,与 WithSyncEval 同族同构)**:hub 创建自动同步与 `POST /api/hubs/{id}/sync` 走 `startHubSync` 分派——生产恒走 `StartSync`(异步),开启 option 的测试 server 走新增的 `discovery.Syncer.SyncHubNow`(同一 in-flight guard、同一 syncing 落标顺序,内联执行,返回即全部尾部写落地)。裁决:**两个独立 option 而非一处总管**——保留异步 eval 语义的测试(中态观察)仍需要 discovery 同步,合一会迫使这类测试同时失去 discovery 接缝;注释已写在两个 option 的 doc 里。
- HTTP 语义对齐生产:同步路径下 sync 失败仍落 hub 状态、POST /sync 仍 202;hub 创建响应在同步路径重读 hub 携带终态。
- 生产装配零改动:`grep WithSync cmd/` 无任何命中,option 默认关闭。

### 异步触发点全审计(server 测试集)

goroutine 挂点全仓只有 5 处:`evals.go:439`(eval run)、`campaigns.go:47`(full sweep)、`discovery.go:146`(StartSync)、`scheduler.go:153`(调度探测 worker)、`login_alert.go:39`(爆破告警发送)。

**改同步(共享 helper 默认开启 WithSyncEval + WithSyncDiscovery):**
- `newTestAPIServer`(scheduler_test.go)——覆盖 auth/discovery/hub_sync(2 个)/tasks_jobs/security 部分等全部使用方
- `setupEvalEnv`(eval_test.go)——约 80 个调用点全量默认同步;`triggerEval` 的 202 响应断言放宽为 `running|done`(注释说明两态皆合法)
- 直接 `server.New` 且经 API 建 hub 的站点逐个加 `WithSyncDiscovery`:integration、security(TestRateLimitWrites,flake2_8 实证票)、alerts(2)、overview(4)、overview_global(3)、overview_score(3)、overview_grouping、endpoint_detail(3)、tasks_jobs、rollup_retention、role_test(在既有 WithSyncEval 上补)
- worker 驱动的自建 server(tasks_test:271、eval_campaign_test:322/410、eval_campaign_edge_test:29、eval_weekly_test:95/183)加 `WithSyncDiscovery` 并注释:eval 异步来自 worker 而非 server option,drain = cancel + 等 `worker.Run` 返回(RunCampaign 在 Run 内同步执行,结构性覆盖全部尾部写)

**保留异步(显式标注 drain 依据):**
- `setupAsyncEvalEnv`(新 helper,无 WithSyncEval、保留 WithSyncDiscovery)——8+1 个中态观察测试迁入,逐个注释「观察什么中态 + drain 依据」:campaign_members(3)、campaign_progress、campaign_report(TestCampaignReportRunningBatchListsMembers)、report_completeness(2:GateSettledBoard、GateMultipleIncomplete)、share_link(TestSharedReportHidesUnfinishedBoard)、eval_campaign(TestCampaignStatusWhileRunning)。drain = 释放 stub gate + `waitCampaignStatus(done)`——无 webhook 时 campaign-done 覆盖全部尾部写(task 日志与 settle 在其前,AfterCampaign 钩子仅读 settings 不落写)
- `TestHubSyncEndpointConflictAndRerun`(hub_sync_test)——409 中态冲突在同步接缝下不可观察,显式自建异步 server;drain 加深:`waitForHubSyncStatus`(覆盖 sync 结果写)+ `waitTasksByType(discovery_sync, success, 2)`(覆盖其后的 task 日志尾部写)
- 调度器探测(scheduler.go:153)与 login 爆破告警(login_alert.go:39)不动:前者 `Run` 内 `workers.Wait()` 结构性 drain(startScheduler cleanup 等 Run 返回);后者测试经 `waitForLarkMessages` 观察发送动作,DB 读严格先于发送,且 webhook 未配置时 goroutine 仅读不写,无 TempDir 竞态面

**10 轮连跑第一轮揪出的两个存量缺陷(已修):**
- `TestDeletedModelStaysOffLiveMemberBoard` 完全没有 drain(释放 gate 后直接结束,sweep goroutine 与 TempDir 竞态——正是本票根因模式的活样本);补 `waitCampaignStatus(done|failed)` 终态 drain。
- `TestReportCompletenessGateMultipleIncomplete` 的中态步进靠「轮询到 suite done 再改 broken 标志」,轮询滞后于 sweep 时 unbreak 晚一个 suite(实测 2/10 失败);改为确定性 gate:foxtrot(每 run 首个模型)用 `blockModelAfter(套件 case 数)` 冻结在下一套件首个 case,换装原子原语 `moveModelGateAfter`(eval_stub_test 新增,release+re-arm 在 stub 锁内一步完成,无滑窗),窗口不再依赖调度时序。rule case sample_count=1(gen-3 seed 钉死),foxtrot 每 rule suite 调用数 = enabledCaseCount(经 API 读,不硬编码)。

### W4 四问评估(结论:不并入,无需 ADR)

1. **为什么必须改?** 不需要改。W4 的语义是「周期作业走可注入 Clock,调度行为可确定性测试」,本票的同步接缝管的是另一个正交轴——goroutine 完成时序的确定性,不动时钟、不动调度器、不动 cron 语义。W4 条款文本零改动。
2. **影响哪些调用方?** 无。W4 未被修改,调度器与其测试集(scheduler_test/interval/patch)行为不变(10 轮连跑覆盖)。
3. **有无替代方案?** 三个候选:①并入 W4——否,时钟注入与 goroutine 同步是正交关注,并入会稀释 W4「为何承重」(换 cron 库则调度不可测)的论证;②新立 W9——否,同步接缝是测试基建约定(server option 族),不是撑起系统语义的结构,其纪律归 AGENTS.md 测试节(已落两条明文)与代码 doc;③扩写 W1——否,W1 是黑盒 HTTP 接缝模式,option 族是 server.New 构造面,纪律已在 AGENTS.md 与 option doc 双重落点。
4. **回归测试什么?** 本票交付物即回归网:`go test ./internal/... -count=1` 连跑 10 轮(证据见下)+ `make test` 全绿;AGENTS.md 新增的两条纪律防回潮。

### 验证证据

- **`go test ./internal/... -count=1` 连跑 10 轮:10/10 零 FAIL**(worktree 实机,2026-07-28;server 包每轮 ~29.5s)。过程记录:修复后的第一轮 10 连跑 7/10(TestReportCompletenessGateMultipleIncomplete ×2、TestDeletedModelStaysOffLiveMemberBoard ×1,均为存量中态测试缺陷,见上节),修复后第二轮 10/10;两个修复测试单独 `-count=20` 加跑亦全绿。
- **`make test` 全绿**:gofmt/vet + 后端全部测试 + `vue-tsc --noEmit` + 前端 build(5.12s)+ install_test.sh 38/38。
- **生产零改动核对:`grep -rn "WithSyncEval\|WithSyncDiscovery\|SyncHubNow" cmd/` 无任何命中**,option 默认关闭。
