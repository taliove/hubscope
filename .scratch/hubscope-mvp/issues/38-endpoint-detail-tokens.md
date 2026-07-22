# 38 — EndpointDetail 视觉接入新 token

**What to build:** 详情页全面接入新 token 与字阶;状态区与 Dashboard 卡片同构(StatusBadge 居首,同色同词);ECharts 图表系列色复核调色板,JS 取色从 token 镜像来不手抄;探测记录表密度调整;接入 AppHeader 后页面内返回路径定型(评审决定保留与否)。规格:docs/specs/0003-ui-redesign.md 批 3。

**Blocked by:** 35 — 全局壳

**备注:** 「← 返回总览」链接已在票 35 随散链清理一并删除(先于批 3 评审落地),本票评审时确认不再恢复;返回路径由 AppHeader 承担。

**Status:** done

- [ ] 与 Dashboard 卡片的状态表达一致(同色同词)
- [ ] 图表无调色板外色相,取色走 token 镜像
- [ ] 三态齐全、长文本截断、轮询配对清理
- [ ] typecheck + build 通过;frontend-checker 全项 PASS
