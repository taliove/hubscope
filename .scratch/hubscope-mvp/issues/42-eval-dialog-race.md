# 42 — EvalRunDetailDialog 快速切换 runId 的竞态

**What to build:** 连续快速打开两条评估运行详情时,旧 `loadRun` 的 await 可能晚于新请求返回,用过期 detail/error 覆盖新状态。修法方向:loadRun 内捕获调用时的 id,await 返回后比较 `props.runId === id` 再赋值;或用递增 token 作废过期响应。(code-reviewer 于票 41 审查中发现的存量问题。)

**Blocked by:** None — can start immediately

**Status:** done

- [ ] 快速切换两条 run 时弹窗内容始终对应当前 runId
- [ ] 过期响应不覆盖新状态(detail 与 error 两条路径)
- [ ] typecheck + build 通过
