# 29 — Eval Campaign + 一键全量评估

**What to build:** 引入 Eval Campaign:一次"考核批次"聚合一组 Eval Run。一键全量触发 = 所有 capability=chat 的模型 × 全部 Suite,每 Suite 一个 Run,全部归入一个 Campaign;手动单跑一个 Suite 也自动归入一个单 Run 的 Campaign(模型统一)。每周定时全量改为产出一个 Campaign。Campaign 本身可在任务中心跟踪(整体进度 = 各 Run 进度聚合)。API:GET /api/campaigns、GET /api/campaigns/{id}(含各 Run 状态与进度)。评估中心运行列表改为按 Campaign 分组展示。

**Blocked by:** 27 — Task 中心:任务实体 + Eval Run 接入

**Status:** ready-for-agent

- [ ] 一键全量:一个 Campaign 挂每 Suite 一个 Run,覆盖全部 chat 模型(non_chat 自动排除,含手动登记模型)
- [ ] 手动单 Suite 触发也产生 Campaign(单 Run),数据模型统一
- [ ] 每周定时全量产出一个 Campaign(假时钟可验证)
- [ ] Campaign 进度/状态可查;运行列表按 Campaign 分组
- [ ] 黑盒测试:一键触发后断言 Campaign 结构、各 Run 归属与最终聚合状态
