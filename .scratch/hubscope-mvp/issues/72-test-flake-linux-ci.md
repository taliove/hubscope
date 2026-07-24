# 72 — 测试套件平台相关 flake 治理(CI 三连败同一 suite 不同测试)

**What to build:** 治理 internal/server 黑盒套件在 Ubuntu CI runner 上的高 flake 率——连续 4 次 CI 运行每次挂 1-3 个**不同**测试(TestSchedulerProbeTimeout / TestHubSyncFailurePersistsStatus / TestFullSweepCampaign / TestDeletedModelStaysOffLiveMemberBoard / TestScoreDropAlertSweepConsolidatesSuites / TestSeedGen3NadirSnapshot),本地 macOS 15+ 连跑全绿、本地 -race 另暴露 TestAuditPrunedByRollupWorker 10s 超时。失败签名分三类:① TempDir RemoveAll cleanup「directory not empty」(SQLite WAL 文件被 async 残留操作持有);② 「disk I/O error (5898)」;③ 10s waitFor 超时(CI 2 核 + 慢磁盘放大调度窗口)。根因待定位,候选方向:(a) discovery.StartSync 的 context.Background() goroutine 与测试生命周期解耦——测试 db.Close() 后 goroutine 仍在写(W1 接缝的结构性缝隙,怀疑主犯);(b) modernc.org/sqlite 在 Linux 的 WAL checkpoint 时机与 macOS 不同;(c) waitFor 10s 硬编码超时不适合共享 runner。治理验收:CI 连续 5 次运行全绿(可 gh run rerun 刷),且不得靠 t.Skip 或放大超时到荒谬值(>60s)蒙混;若改 waitFor 一律改条件等待语义而非粗暴加时。

**Blocked by:** 无

**Status:** ready-for-agent

- [ ] 定位根因:在 Linux 容器/VM 复现至少一类失败(本地 Docker 或 CI debug 分支),确认 (a)/(b)/(c) 各自贡献
- [ ] 修复 discovery.StartSync goroutine 生命周期(若证实):注入可等待的完成信号或改用测试可驱动时钟,黑盒不断言内部状态的前提下让测试能确定性等待 sync 收尾
- [ ] 修复 WAL/TempDir 冲突(若证实):测试 db.Close 前 checkpoint 或换 journal_mode=DELETE 于测试路径
- [ ] `go test -race ./internal/server/` 全绿(当前 TestAuditPrunedByRollupWorker 挂)
- [ ] CI 连续 5 次全绿(gh run list 截图证据贴 ticket)
- [ ] `make test` 全绿
