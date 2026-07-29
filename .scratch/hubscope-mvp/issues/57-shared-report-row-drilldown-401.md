# 57 — 分享页行下钻 401

**What to build:** ticket 54 评审记录的既有缺陷(非本票引入):分享报告页(/report/{token})settle 后榜单行可点,行下钻打开 ModelTrendDialog 拉取 `/api/campaigns/{id}/trends`——该接口是会话门禁的,匿名分享访客点击后得到 401,弹窗呈错误态。方向:要么分享页行不可点(下钻入口按登录态隐藏),要么趋势接口对持 token 的分享访客开放只读口径(需过 ADR 0006 控制面评审,涉及趋势数据是否属于「单个 Campaign 报告」边界)。

**Blocked by:** None — can start immediately

**Status:** 已迁移至 GitHub issue #10(2026-07-28 全面切换 GitHub Issues;本地票只读存档)

- [ ] 分享页行下钻不再 401(隐藏入口或开放 token 口径,评审择一)
