# 47 — 任务中心记录保留清理

**What to build:** tasks/task_logs 表当前无任何保留清理:ticket 28 后每个定时 rollup(每小时)与 retention cleanup(每天)各产生一个 Task + 若干日志行,长期无界增长。需要给任务中心加保留策略:默认保留 90 天(与 probes 原始数据同周期,走 settings 可调或常量),由现有 rollup worker 的每日清理一并执行,日志记录清理行数。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] 每日清理一并删除 90 天前的 tasks 及其 task_logs(级联或两步删除)
- [ ] 保留期可配(settings 键,默认 90 天)
- [ ] 黑盒测试:假时钟推进,断言过期 Task 被清理、近期 Task 保留
