# 52 — 评估进度网格与实时半成品榜

**What to build:** 按 spec 0004 改造批次进度体验:① Progress Grid——模型 × 能力点状态矩阵(等待中/运行中/已完成/失败),运行中批次的默认视图,状态色复用语义色,批次级汇总在网格上方;② 半成品榜单——网格提供「查看实时分数」入口,运行中不排名次(字典序)、分数带「X/Y 题」Coverage 水印、未跑能力点「进行中」占位且不计入总分;settle 后名次徽章出现 + ElMessage 提示;③ AppHeader 全局入口——有运行中批次时显示「批次运行中 X/Y」,点击跳 /eval。后端:报告 API 透出模型 × 能力点 run 级明细(状态+覆盖率),现为聚合计数,只加字段不动评估语义。轮询沿用既有口径(3s,仅未完成时,settle 即停,卸载清理)。开工前必过 design-owner 评审;落地后回写 ui-guidelines.md(Progress Grid 登记、spec 0002 条件③修订已注)。

**Blocked by:** None — can start immediately(置信标记视觉与 51 协同,但功能独立)

**Status:** done

- [x] 进度 API 返回模型 × 能力点状态与覆盖率明细
- [x] Progress Grid 为运行中默认视图,四态语义色正确
- [x] 半成品榜:不排名次、Coverage 水印、未跑能力点不计入总分
- [x] settle 转场:名次徽章出现 + 完成提示,轮询停止
- [x] AppHeader 全局进度入口(登录态过滤、路由切换重检)
- [x] 三态(加载/空/错误)与轮询清理齐全
- [x] 黑盒测试覆盖 API 部分;前端过 frontend-checker
