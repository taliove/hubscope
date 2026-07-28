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
