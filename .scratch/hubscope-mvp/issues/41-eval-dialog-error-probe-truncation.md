# 41 — 评估详情弹窗错误态 + 探测记录错误列截断

**What to build:** 两处存量体验缺口补齐(frontend-checker 于批 2-5 审查中发现):① EvalRunDetailDialog 的 `getEvalRun` 请求失败时无 catch,出现 unhandled rejection + 空白弹窗 —— 补弹窗内错误块(错误原因 + 重试入口,实现口径,比 ElMessage 更贴合 §6 三态);② ProbeRecordTable「错误摘要」列长文本无截断/tooltip,会撑高表格行 —— 加 `show-overflow-tooltip`(或等价截断 + hover 全显),符合 ui-guidelines §6 长文本规范。

**Blocked by:** None — can start immediately

**Status:** done

- [ ] EvalRunDetailDialog 加载失败有用户可见错误提示,无 unhandled rejection
- [ ] 错误摘要列截断 + hover 全显,不再撑行
- [ ] typecheck + build 通过
