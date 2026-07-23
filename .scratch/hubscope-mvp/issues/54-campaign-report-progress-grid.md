# 54 — 分享报告页接入进度网格

**What to build:** ticket 52 遗留:CampaignReportView(含 /report/{token} 分享页)运行中批次仍显示旧 el-progress 进度卡。按 spec 0004 统一:运行中默认 EvalProgressGrid,可切实时分数(live 榜单),settle 转场与 /eval 同语义;分享页保持只读、隐藏操作类按钮(ADR 0006 不变)。过 design-owner 评审(消费页 16px 档)。

**Blocked by:** 52(组件已登记)

**Status:** done

- [x] CampaignReportView 运行中默认进度网格 + 实时分数切换
- [x] 分享页同语义且只读(无操作按钮)
- [x] settle 转场提示与轮询停止口径与 /eval 一致
- [x] 三态齐全、轮询卸载清理
- [x] shared 未完成批次出剥离 cells(状态+计数,无分数无 samples)
