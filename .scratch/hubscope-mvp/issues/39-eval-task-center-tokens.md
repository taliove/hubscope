# 39 — EvalCenter / TaskCenter token 化

**What to build:** 两页及评估组件(EvalRunList、EvalTrendChart、ScoreMatrix、EvalRunDetailDialog、CaseLibrary)全面 token 化;管理台紧凑密度档(12px 内边距);评分档位色复核与后端口径一致;趋势图断点标注视觉统一;el-tabs 不加 lazy 约定不破。规格:docs/specs/0003-ui-redesign.md 批 4。

**Blocked by:** 35 — 全局壳

**Status:** done

- [ ] 管理台密度不劣化(一屏信息量不减少)
- [ ] 评分绿/黄/红阈值渲染与后端口径一致
- [ ] 无调色板外硬编码色值残留
- [ ] el-tabs 未加 lazy(轮询事件不破)
- [ ] typecheck + build 通过;frontend-checker 全项 PASS
