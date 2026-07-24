---
name: implement-ticket
description: ticket 实现流程(main 放行影响分析之后):TDD at W1 唯一接缝 → 小步验证 → 三层测试 → review 流程 → commit。implementer 阶段 2 与任何 ticket 实现场景使用。
---

# ticket 实现流程

前置条件:影响分析已由 `architect` 产出、main 已明确放行。从一张 ticket 开工到 commit 的完整实现流。

1. **TDD at 唯一接缝(承重墙 W1)** — 测试一律走 HTTP API 层:httptest + stub Hub + 假时钟 + 真 SQLite 临时库;不 mock 内部模块,不断言内部状态。先写黑盒 HTTP 测试跑红 → 最小实现跑绿 → 重构。测试细节纪律(stub Hub 校验请求字段、假时钟禁 sleep、流式故障样本)见 `add-tests` skill。
2. **小步验证** — 定期 `pnpm typecheck`(前端)与跑当前单测试文件;改动相关的最小测试集先于全量。
3. **三层测试** — 当前功能层(新增黑盒测试全绿)+ 关联功能层(触及模块及调用方包回测)+ 核心闭环层(`make test` 全量:后端全部测试 + 前端 typecheck + build)。
4. **review 流程** — 走 `review` skill:范围确认 → test-verifier 三层复核 → code-reviewer 独立双轴审查(CRITICAL/HIGH 修完)。
5. **commit** — 英文 Conventional Commits(`feat|fix|refactor|docs|test|chore|perf|ci: <desc>`);一票一 commit(或票内原子多 commit),单 commit ≤8 文件;测试不绿不许 commit;完成后在 ticket 文件 Status 处标记 done,并做「持续进化」自问(第三次手写的流程 / 第二次违反的约束 / 无人认领的职责 / 未记录的决策——见 AGENTS.md)。

## 铁律(实现期不可违反)

- 测试不绿,不许 commit。`make test` 是硬门禁。
- 不主动 `git push`、打 tag、部署——等用户明确指令。
- 凭证不进 git;凭证/token 用假值。
- 禁 `--no-verify` 及任何门禁绕过。
- 不可变数据优先;函数返回新值,不改入参。
- 单文件超 ~400 行考虑拆分;函数 <50 行。
- 前端改动走 Element Plus + CSS 语义令牌(ui-guidelines.md),完成后过 `frontend-checker` 自查:卡片溢出/横向滚动/长文本截断/加载空态/轮询泄漏。
