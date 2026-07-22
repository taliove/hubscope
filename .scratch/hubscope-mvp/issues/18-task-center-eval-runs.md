# 18 — Task 中心:任务实体 + Eval Run 接入

**What to build:** 引入一等 Task 实体:`tasks` 表(类型、触发来源 manual/scheduled、状态 pending/running/success/failed、起止时间、关联实体类型与 ID)与 `task_logs` 表(逐行日志:时间戳、级别、消息)。每次 Eval Run(手动或每周)创建时注册为 Task,执行中逐 Case 写进度日志(开始/完成/得分/裁判失败),Run 终态同步到 Task。新增任务中心页(新路由):任务列表(类型/状态/来源/耗时/关联实体链接)、按类型与状态过滤、点开看逐行日志。API:GET /api/tasks(过滤、分页)、GET /api/tasks/{id}(含日志)。探测轮次不是 Task,不出现。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] tasks 与 task_logs 随迁移存在,状态机统一(pending/running/success/failed)
- [ ] 手动与每周触发的 Eval Run 都注册为 Task,状态随执行流转到 success/failed
- [ ] 执行日志逐行可查:至少含每 Case 完成与裁判失败事件
- [ ] 任务中心页:列表、过滤、日志查看;写操作/管理动作仍需登录,读遵循现有读权限分级
- [ ] 黑盒测试:触发 run 后任务出现、日志逐行完整、失败 run 的 Task 为 failed
